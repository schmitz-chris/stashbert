# StashBert mit Home Assistant verbinden

Diese Anleitung beschreibt, wie StashBert über MQTT an Home Assistant (HA) angebunden wird und wie die Einkaufsliste in eine To-do-Liste wie Bring! kommt. Die Entscheidungen stehen in ADR-0018, die technischen Details in `architecture.md`, Kapitel 11.

## 1. Überblick

- StashBert verbindet sich mit dem MQTT-Broker (bei HA meist das Mosquitto-Add-on) und meldet sich per Discovery als Gerät „StashBert" an.
- Das Gerät bringt diese Entitäten mit (Namen auf Englisch, in HA umbenennbar):

  | Entität | Bedeutung |
  |---|---|
  | `sensor.stashbert_shopping` („Shopping list") | Anzahl der Einträge auf der Einkaufsliste; die Einträge stehen im Attribut `items` |
  | `sensor.stashbert_empty` („Empty products") | Produkte mit Bestand 0 |
  | `sensor.stashbert_review` („Products to review") | Produkte, deren Angaben geprüft werden sollten |
  | `sensor.stashbert_products` („Products") | alle Produkte |
  | `event.stashbert_stock` („Stock change") | jede Buchung (`stock.added`, `stock.consumed`, `stock.adjusted`) |
  | `button.stashbert_resend` („Resend shopping list") | sendet die ganze Einkaufsliste erneut |

- Jede Änderung der Einkaufsliste geht als MQTT-Nachricht hinaus. Die Automation aus dem Blueprint „StashBert: Einkaufsliste in eine To-do-Liste übertragen" schreibt sie in eine To-do-Liste und legt Einträge an, ändert ihre Menge oder entfernt sie.
- Alle Topics und Nachrichten sind Englisch. Deutsche Texte wie „3 Stück" oder „1 Kasten" entstehen erst im Blueprint.

## 2. Voraussetzungen

- HA mit der MQTT-Integration und einem Broker, der MQTT 5 spricht (Mosquitto-Add-on ab Version 6).
- Ein eigener MQTT-Benutzer für StashBert, z. B. `stashbert`. Beim Mosquitto-Add-on geht das über einen HA-Benutzer oder in der Add-on-Konfiguration unter `logins`.
- Der Broker muss aus dem Container von StashBert erreichbar sein (Port 1883, mit TLS 8883).

## 3. StashBert konfigurieren

In `/etc/stashbert/stashbert.env` (siehe `deploy/stashbert.env.example`):

```bash
MQTT_URL=mqtt://192.168.1.10:1883
MQTT_USERNAME=stashbert
MQTT_PASSWORD=geheim
```

Danach neu starten:

```bash
systemctl restart stashbert
```

Die übrigen Variablen haben passende Standardwerte:

| Variable | Standard | Bedeutung |
|---|---|---|
| `MQTT_CLIENT_ID` | `stashbert` | Client-ID; zwei laufende Instanzen (z. B. Test und Betrieb) brauchen verschiedene IDs |
| `MQTT_TOPIC_PREFIX` | `stashbert` | Präfix aller Topics und der Entitäts-IDs |
| `MQTT_HA_DISCOVERY` | `true` | `false` meldet das Gerät in HA ab |
| `MQTT_HA_PREFIX` | `homeassistant` | Discovery-Präfix von HA |

Im Log von StashBert (`journalctl -u stashbert`) steht nach dem Start `mqtt connected`.

## 4. In HA prüfen

Unter **Einstellungen → Geräte & Dienste → MQTT → Geräte** erscheint „StashBert" mit den Entitäten aus Abschnitt 1. Ist StashBert gestoppt, zeigen sie „Nicht verfügbar".

## 5. Blueprints installieren

Die Blueprints liegen im Repository unter `deploy/homeassistant/`:

- `stashbert_einkauf_todo.yaml`: „StashBert: Einkaufsliste in eine To-do-Liste übertragen"
- `stashbert_listen_melden.yaml`: „StashBert: Einkaufslisten anbieten"

**Installieren:** Die beiden Dateien nach `/config/blueprints/automation/stashbert/` in HA kopieren (z. B. mit dem Add-on „File editor" oder „Samba share"), danach unter **Entwicklerwerkzeuge → YAML** „Automationen" neu laden. Ist das Repository öffentlich, geht auch **Einstellungen → Automationen & Szenen → Blueprints → Blueprint importieren** mit der URL der Datei.

**Automationen anlegen** (Einstellungen → Automationen & Szenen → Blueprints → Blueprint anklicken):

1. „StashBert: Einkaufslisten anbieten": Integration `bring` (Standard). Die Automation meldet StashBert beim Start von HA, stündlich und wenn StashBert online geht, welche Bring!-Listen es gibt.
2. „StashBert: Einkaufsliste in eine To-do-Liste übertragen": als **Standardliste** die Liste wählen, die gelten soll, solange in StashBert keine gewählt ist. Die Einheiten („Stück", „Kasten", „Kästen") lassen sich anpassen.

## 6. Liste in StashBert wählen

In StashBert in der Vorrat-Ansicht oben rechts auf das Zahnrad tippen und in den Einstellungen im Abschnitt „Home Assistant" die Liste wählen. Beim Wechsel räumt StashBert die alte Liste ab und füllt die neue. „Liste neu senden" (oder der Knopf „Resend shopping list" in HA) gleicht die ganze Liste ab, z. B. nachdem die Automation eine Weile aus war.

## 7. Wie der Abgleich arbeitet

- StashBert sendet bei jeder Änderung den aktuellen Zustand eines Eintrags: steht er auf der Liste, wie viel fehlt (`quantity`, `unit`: `piece` oder `crate`) und in welche Liste er gehört.
- Die Automation sucht den Eintrag über den Produktnamen in der To-do-Liste:
  - nicht mehr auf der Liste: entfernen, falls vorhanden;
  - schon vorhanden (auch abgehakt): Menge als Beschreibung setzen und wieder öffnen;
  - sonst: neu anlegen.
- Einträge, die StashBert nicht kennt (von Hand angelegt), bleiben unberührt.
- In Bring! steht die Menge unter dem Artikelnamen (Feld „Spezifikation").
- Wird ein Produkt in StashBert umbenannt, bleibt der alte Name in der To-do-Liste stehen; ihn einmal von Hand löschen.

## 8. Eigene Automationen

Die Nachrichten eignen sich auch für eigene Automationen, z. B. eine Benachrichtigung, wenn ein Produkt leer ist:

```yaml
triggers:
  - trigger: mqtt
    topic: stashbert/events/product.empty
actions:
  - action: notify.mobile_app_iphone
    data:
      message: "{{ trigger.payload_json.data.name }} ist leer."
```

Alle Topics und Felder stehen in `architecture.md`, 11.2 und 11.3.

## 9. Zugriff begrenzen (ACL, optional)

Beim Mosquitto-Add-on lässt sich der Benutzer `stashbert` auf seine Topics beschränken. Dafür in der Add-on-Konfiguration `customize: {active: true, folder: mosquitto}` setzen und in `/share/mosquitto/acl.conf` eintragen:

```
user stashbert
topic write stashbert/status
topic write stashbert/state/#
topic write stashbert/events/#
topic write homeassistant/device/stashbert/config
topic read stashbert/in/#
topic read homeassistant/status
```

Die Benutzer von HA und der Add-ons brauchen weiterhin vollen Zugriff. Ob das Add-on die ACL-Datei für HA-Benutzer tatsächlich anwendet, ist nicht bestätigt; nach dem Einrichten prüfen, dass StashBert weiter `mqtt connected` meldet und keine Warnungen `deliver outbox` im Log stehen.

## 10. Fehlersuche

Alle Nachrichten von StashBert mitlesen (Paket `mosquitto-clients`):

```bash
mosquitto_sub -h 192.168.1.10 -u stashbert -P geheim -t 'stashbert/#' -v
```

| Beobachtung | Ursache und Abhilfe |
|---|---|
| Kein Gerät „StashBert" in HA | `MQTT_URL` leer, falscher Benutzer oder `MQTT_HA_DISCOVERY=false`; Log von StashBert prüfen |
| Entitäten „Nicht verfügbar" | StashBert läuft nicht oder hat keine Verbindung (`stashbert/status` ist `offline`) |
| Nichts kommt in Bring! an | Automation aus, keine Liste in StashBert und keine Standardliste gewählt; in HA unter der Automation die Ablaufverfolgung ansehen |
| Liste in StashBert nicht auswählbar | Automation „Einkaufslisten anbieten" fehlt oder hat noch nicht ausgelöst; sie einmal von Hand ausführen |

## 11. Entfernen

- `MQTT_HA_DISCOVERY=false` setzen und StashBert neu starten: HA entfernt das Gerät.
- Wird das Gerät nur in HA gelöscht, meldet StashBert es beim nächsten Start wieder an.
- Ohne `MQTT_URL` sendet StashBert gar nichts mehr.
- Die Automationen und Blueprints in HA von Hand löschen.
