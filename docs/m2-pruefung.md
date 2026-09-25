# M2-Prüfung: Home Assistant über MQTT

Stand 24.09.2026. Prüfung von Phase 2a (ADR-0018, architecture.md Kapitel 11) gegen den Broker im Heimnetz (Mosquitto-Add-on auf `homeassistant.local`) und das Home Assistant des Haushalts (2026.9.3). Geprüft hat der Lead mit Kopien der lokalen Datenbank und einer eigenen Client-ID (`stashbert-dev`).

## 1. Vom Lead geprüft

| Bereich | Ergebnis |
|---|---|
| Verbindung (B34) | `stashbert/status` ist nach dem Start `online` und nach dem Beenden retained `offline`; das Passwort steht nicht im Log |
| Ereignisse (B35) | drei Entnahmen ergeben drei `stock.consumed` und ein `shopping.changed` mit `quantity: 1`, `unit: "crate"`; alles in Englisch |
| Vormerken und Abgleich (B36, B36a) | `POST /shopping-list/snapshot` und `stashbert/in/snapshot` ergeben je einen `shopping.snapshot`; Einlagern eines vorgemerkten Produkts entfernt es aus der To-do-Liste |
| Zusammenfassung (B37, B38) | die vier Sensoren zeigen die richtigen Zahlen, die Einkaufsliste steht im Attribut `items`; „Passt so" auf der Produktseite senkt „Products to review" sofort |
| Discovery (B38) | HA legt das Gerät „StashBert" (Modell „Pantry inventory", Version aus dem Build) mit vier Sensoren, der Ereignis-Entität „Stock change" und dem Knopf „Resend shopping list" an; ohne StashBert sind die Entitäten „Nicht verfügbar" |
| Knopf in HA | „Resend shopping list" löst den Abgleich aus; die Einträge landen in der Testliste |
| Zielliste (B39) | das Angebot von HA kommt an; Wählen füllt die gewählte Liste, Aufheben räumt sie ab, eine nicht angebotene Liste gibt 422 |
| Oberfläche (F30) | Abschnitt „Home Assistant" in der Einkaufsansicht mit „Verbunden", Auswahl und „Liste neu senden" (mit echten Daten gegen den Broker) |
| Blueprints (H01) | beide Blueprints in HA gespeichert; Hinzufügen, Menge ändern, Kästen, vorgemerkt ohne Menge, Entfernen, Abgleich, abgehakten Eintrag wieder öffnen ohne Duplikat und Rückfall auf die Standardliste geprüft; kein Fehler im HA-Log |
| Bring! | über die Bring!-Liste „Albert Hijn": Eintrag „StashBert Test" mit Spezifikation „2 Stück" angelegt, auf „1 Kasten" geändert und wieder entfernt; die übrigen Einträge blieben unberührt |

Nicht gegen das echte System ausgelöst: die Birth-Nachricht von HA (`homeassistant/status` = `online`). Sie würde alle MQTT-Geräte im Haus zum Neusenden bringen; das Verhalten von StashBert darauf ist mit dem Test-Broker geprüft (B38).

## 2. Was in Home Assistant eingerichtet wurde

- Blueprints unter `blueprints/automation/stashbert/`: „StashBert: Einkaufsliste in eine To-do-Liste übertragen" und „StashBert: Einkaufslisten anbieten".
- Automation „StashBert: Einkaufsliste übertragen": Standardliste ist die Testliste „StashBert Test" (Local To-do). Sie gilt nur, solange in StashBert keine Liste gewählt ist.
- Automation „StashBert: Einkaufslisten anbieten": bietet StashBert die fünf Bring!-Listen an (Albert Hijn, Baumarkt, Ferienhaus, Gemeinsam, IKEA).
- Testliste „StashBert Test" (Local To-do, leer).
- Gerät „StashBert" (per Discovery; zeigt „Nicht verfügbar", solange keine Instanz mit MQTT läuft).

Nichts davon schreibt in eine Bring!-Liste, solange in StashBert keine Bring!-Liste gewählt ist.

## 3. Zum Ausprobieren (Stand des Entwicklungsrechners)

Der Server auf Port 8080 läuft mit MQTT (Client-ID `stashbert-dev`); die Vorschau auf 8083 und 8084 nutzt ihn.

1. In StashBert in der Vorrat-Ansicht auf das Zahnrad tippen und unter „Home Assistant" eine Bring!-Liste wählen, z. B. „Gemeinsam". Die vorgemerkten Produkte erscheinen dort.
2. Ein Produkt einscannen, das auf der Liste steht: der Eintrag verschwindet aus Bring!.
3. Etwas entnehmen, bis Soll unterschritten ist: der Eintrag erscheint mit Menge als Spezifikation.
4. In HA unter Geräte „StashBert" ansehen; die Entitäten lassen sich auf Deutsch umbenennen.

## 4. Für den Betrieb im LXC

Das Proxmox-Skript (docs/betrieb.md, Abschnitt 2) fragt die MQTT-Angaben beim Einrichten ab. Von Hand in `/etc/stashbert/stashbert.env`:

```bash
MQTT_URL=mqtt://192.168.1.10:1883
MQTT_USERNAME=stashbert
MQTT_PASSWORD=...
```

Die Client-ID bleibt beim Standard `stashbert`. Solange die Entwicklungsinstanz (`stashbert-dev`) auch läuft, senden beide an dieselben Topics; die Entwicklungsinstanz also vorher beenden oder ohne MQTT starten. Alles Weitere steht in `docs/home-assistant.md`.

## 5. Offene Nutzerprüfungen

- [ ] In HA erscheint das Gerät „StashBert" mit vier Sensoren, der Ereignis-Entität und dem Knopf (H02, Kriterium 2).
- [ ] Eine Buchung ändert die Sensoren; ein Artikel erscheint über den Blueprint in der gewählten Bring!-Liste und verschwindet nach dem Einlagern (H02, Kriterium 3).
- [ ] Standardliste der Automation „StashBert: Einkaufsliste übertragen" nach Wunsch setzen (oder die Testliste behalten) und die Testliste bei Bedarf löschen.
- [ ] Das für die Entwicklung angelegte HA-Token löschen (Profil → Sicherheit), wenn es nicht mehr gebraucht wird.

## 6. Bekannte Grenzen

- Wird ein Produkt umbenannt, bleibt der alte Name in der To-do-Liste stehen.
- Ob das Mosquitto-Add-on eine ACL für HA-Benutzer anwendet, ist nicht bestätigt (docs/home-assistant.md, 9).
- `paho.golang` ist vor 1.0; die Outbox fängt Verbindungsabbrüche ab.
