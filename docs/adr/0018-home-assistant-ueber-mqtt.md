# ADR-0018: Home Assistant über MQTT (M2)

- Status: angenommen
- Datum: 2026-09-24

## Kontext

- Der Nutzer möchte StashBert an Home Assistant (HA) anbinden. ADR-0011 hat dafür Outbox und MQTT 5 mit HA-Discovery vorgesehen; `research.md`, Kapitel 10 und 11, enthält den Entwurf.
- Im Heimnetz läuft das Mosquitto-Add-on von HA (`core-mosquitto`, Mosquitto 2.1) auf dem HA-Host. Für StashBert gibt es dort einen eigenen MQTT-Benutzer.
- Eine Nachprüfung am 24.09.2026 gegen HA 2026.9.3 (Quellcode und Doku) ergab Korrekturen am Entwurf: `cmps` ist eine Zuordnung, keine Liste; `origin` ist Pflicht; `object_id` gibt es seit HA 2026.4 nicht mehr (Ersatz `default_entity_id`); ein leeres Payload entfernt das ganze Gerät; die Birth-Nachricht von HA ist nicht retained.
- Anforderung A10 (bestätigt): Die Einkaufsliste, in die geschrieben wird, soll in StashBert auswählbar sein. StashBert soll trotzdem nichts über Bring! wissen.

## Entscheidung

- **Kanal:** MQTT 5 zum vorhandenen Broker. Ist `MQTT_URL` leer, läuft StashBert ohne MQTT und ohne Outbox (Publisher bleibt `events.Nop`).
- **Client:** `github.com/eclipse/paho.golang` v0.23.0 mit `autopaho` (MQTT 5, Wiederverbinden, Last Will, `Publish` mit QoS 1 wartet auf PUBACK und meldet Ablehnungen wie 0x87 als Fehler). Der Client steckt hinter einem kleinen Interface in `internal/mqtt`. Tests nutzen den eingebetteten Broker `github.com/mochi-mqtt/server/v2` v2.7.9 (nur in Tests).
- **Outbox:** Eine Implementierung von `events.Publisher` schreibt jedes Ereignis nach dem Commit in die Tabelle `outbox`. Ein Hintergrundjob publiziert die Einträge der Reihe nach mit QoS 1 und löscht sie nach erfolgreichem PUBACK. Die Fachlogik ändert sich dafür nicht (ADR-0011). Stürzt der Prozess zwischen Commit und Outbox-Eintrag ab, geht ein Ereignis verloren; das Zeitfenster ist klein, und Zusammenfassung sowie Abgleich (`shopping.snapshot`) stellen den richtigen Zustand wieder her.
- **Topics** (Präfix `MQTT_TOPIC_PREFIX`, Standard `stashbert`): Verfügbarkeit, retained Zusammenfassung, Ereignisse, eingehende Befehle und die Discovery (architecture.md, Kapitel 11).
- **HA-Discovery:** eine Gerätemeldung unter `homeassistant/device/<präfix>/config`, retained mit QoS 1, mit `origin`, `default_entity_id` und ohne `object_id`. Vier Zähl-Sensoren, eine Ereignis-Entität für Buchungen und ein Knopf, der die Einkaufsliste neu sendet (Namen auf Englisch, siehe Sprache). Keine Entität pro Produkt. StashBert sendet die Meldung bei jeder eigenen Verbindung und nach der Birth-Nachricht von HA (mit 1 bis 5 s Zufallsverzögerung) erneut. Mit `MQTT_HA_DISCOVERY=false` löscht StashBert die Meldung.
- **Einkaufsliste nach HA (A10):** HA meldet die verfügbaren Listen retained auf `<präfix>/in/targets`. StashBert zeigt sie in der Einkaufsansicht zur Auswahl und speichert die Wahl in `settings`. Jedes Einkaufsereignis trägt die gewählte Liste im Feld `list`. Eine HA-Automation (Blueprint im Repository) schreibt in diese Liste; ist `list` leer, nimmt sie ihre eigene Standardliste. StashBert kennt dabei nur undurchsichtige Kennungen.
- **Abgleich statt Übergänge:** Die Einkaufsereignisse tragen den aktuellen Zustand eines Eintrags (`on_list`, `quantity`, `unit`), nicht nur die Änderung. Die Automation legt an, ändert oder entfernt danach; doppelte Einträge verhindert sie mit `todo.get_items`. Auch Vormerken und Entfernen der Vormerkung lösen `shopping.changed` aus.
- **Sprache (Nutzervorgabe vom 24.09.2026, verbindlich):** Topics und Nachrichten sind immer Englisch: Topic-Namen, JSON-Schlüssel, feste Werte und die Namen und Modellangaben in der Discovery. Mengen gehen als Zahl mit Einheit (`quantity`, `unit`) hinaus, nicht als deutscher Text. Deutsche Texte entstehen erst in HA (Blueprint), in der Oberfläche von StashBert oder stammen aus Nutzerdaten wie Produktnamen.
- **HA-spezifisch** ist in StashBert nur das Discovery-Payload. Alles andere sind allgemeine MQTT-Nachrichten, die auch Node-RED oder eigene Skripte lesen können. StashBert speichert keine Zugangsdaten von HA oder Bring!.
- **Sicherheit:** Das MQTT-Passwort kommt nur aus der Umgebung und wird nie geloggt. Die Betriebsanleitung empfiehlt eine ACL für den Benutzer `stashbert`.

## Konsequenzen

- Neue Abhängigkeiten: `paho.golang` (Laufzeit) und `mochi-mqtt/server/v2` (nur Tests), eingetragen in AGENTS.md.
- Neue Migrationen (`outbox`), neue Endpunkte (`GET /summary`, Abgleich, MQTT-Status und Zielliste) und neue Konfiguration (architecture.md 9.2).
- Das Discovery-Payload muss bei HA-Releases geprüft werden (HA gibt Nutzeroptionen nur 6 Monate Übergangsfrist).
- `paho.golang` ist vor 1.0; bekannte Reconnect-Fehler sind nur auf master behoben. Die Outbox macht die Zustellung trotzdem verlässlich; ein Update auf v0.24.0 folgt, sobald es erscheint.
- `mochi-mqtt` wird kaum noch gepflegt; es dient nur als Test-Broker.

## Alternativen

- **REST-Sensor und Webhooks statt MQTT:** Der Broker läuft schon; MQTT ist lose gekoppelt und bringt Discovery ohne YAML.
- **Eigene HA-Integration (HACS):** eigene Python-Codebasis mit hoher Änderungsrate bei HA; nur bei echtem Bedarf, z. B. einer `todo`-Entität direkt aus StashBert.
- **`paho.mqtt.golang`:** nur MQTT 3.1.1, meldet abgelehnte Publishes nicht.
- **Outbox in derselben Transaktion:** exakter, verlangt aber Änderungen an allen schreibenden Abläufen der Fachlogik.
- **Zielliste nur im Blueprint wählen:** einfacher, widerspricht aber A10. Der Blueprint behält eine Standardliste als Rückfall.
