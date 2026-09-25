#!/usr/bin/env bash
# Legt auf einem Proxmox-Host einen unprivilegierten Debian-13-Container mit
# StashBert an (ADR-0019, ADR-0014). Aufruf als root auf dem Proxmox-Host:
#
#   bash -c "$(curl -fsSL https://github.com/schmitz-chris/stashbert/releases/latest/download/proxmox.sh)"
#
# Argumente folgen nach einem Platzhalter für $0, etwa: bash -c "$(curl ...)" _ --dry-run
# Die Antworten kommen von der Standardeingabe (beim Aufruf oben das
# Terminal). Alle Eingaben werden geprüft, bevor etwas angelegt wird. Die
# ausführliche Ausgabe von pveam, apt und stashbert-update landet in einem
# Protokoll auf dem Host. STASHBERT_RELEASES_URL ersetzt die Adresse der
# Releases (etwa für Tests) und wird an stashbert-update weitergegeben.
set -Eeuo pipefail

BASE=${STASHBERT_RELEASES_URL:-https://github.com/schmitz-chris/stashbert/releases}
ENV_FILE=/etc/stashbert/stashbert.env
# Gilt für alle Konsolen des Containers (tty1, tty2), nicht nur für die erste.
GETTY_DIR=/etc/systemd/system/container-getty@.service.d
OCTET='(25[0-5]|2[0-4][0-9]|1[0-9][0-9]|[1-9]?[0-9])'
IPV4_RE="^$OCTET(\\.$OCTET){3}\$"
CIDR_RE="^$OCTET(\\.$OCTET){3}/([1-9]|[12][0-9]|3[0-2])\$"
MQTT_RE='^mqtts?://[^/?#@]+:[0-9]{1,5}$'
APT=(env DEBIAN_FRONTEND=noninteractive apt-get -y -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold)

DRY=0 CREATED=0 DIED=0 STEP="Prüfung der Umgebung" LOG=/dev/null
TMP='' CTID='' ANSWER='' GW='' OFF='' MQTT_URL='' MQTT_USER='' MQTT_PASS='' BACKUP='' BACKUP_FILE=''

# Ersetzt im Container die Zeilen KEY=... der Env-Datei; fehlt eine Zeile,
# wird sie angehängt, leere Werte bleiben unberührt. Die Werte kommen über
# die Umgebung (env im Aufruf von pct exec), nie über diesen Text.
ENV_EDIT=$(
	cat <<'EOF'
set -e
f=/etc/stashbert/stashbert.env
awk 'BEGIN { n = split("OFF_CONTACT MQTT_URL MQTT_USERNAME MQTT_PASSWORD", k, " ") }
{ for (i = 1; i <= n; i++) if (ENVIRON[k[i]] != "" && index($0, k[i] "=") == 1) { $0 = k[i] "=" ENVIRON[k[i]]; seen[k[i]] = 1 }; print }
END { for (i = 1; i <= n; i++) if (ENVIRON[k[i]] != "" && !(k[i] in seen)) print k[i] "=" ENVIRON[k[i]] }' "$f" >"$f.neu"
cat "$f.neu" >"$f"
rm -f "$f.neu"
EOF
)

usage() {
	cat <<'EOF'
Aufruf: proxmox.sh [--dry-run] [--help]

Legt auf dem Proxmox-Host (als root) einen unprivilegierten Debian-13-Container
mit StashBert an. Die Fragen kommen auf der Standardeingabe; Enter übernimmt
die Vorgabe in eckigen Klammern.

  --dry-run  fragt wie sonst, zeigt die ändernden Befehle aber nur an
  --help     zeigt diese Hilfe

  bash -c "$(curl -fsSL https://github.com/schmitz-chris/stashbert/releases/latest/download/proxmox.sh)" _ --dry-run
EOF
}

die() {
	DIED=1
	echo "Fehler: $*" >&2
	exit 1
}

on_exit() {
	local rc=$?
	[ -z "$TMP" ] || rm -rf "$TMP"
	((rc != 0)) || return 0
	((DIED)) || echo "Fehler: Schritt \"$STEP\" ist fehlgeschlagen." >&2
	((CREATED == 0)) ||
		echo "Der Container $CTID bleibt zur Fehlersuche bestehen. Entfernen mit: pct stop $CTID; pct destroy $CTID" >&2
	if [ "$LOG" != /dev/null ]; then
		echo "Letzte Zeilen des Protokolls $LOG:" >&2
		tail -n 8 "$LOG" | sed 's/^/  /' >&2
	fi
}
trap on_exit EXIT
trap 'exit 130' INT TERM HUP

step() { STEP=$1 && echo "- $1"; }

# run <befehl...>: führt einen ändernden Befehl aus, die Ausgabe geht ins
# Protokoll; mit --dry-run wird er nur angezeigt. SHOW ersetzt die Anzeige,
# damit ein Passwort weder im Terminal noch im Protokoll steht.
run() {
	local shown=${SHOW:-}
	[ -n "$shown" ] || { shown=$(printf '%q ' "$@") && shown=${shown% } && shown=${shown//\\,/,}; }
	if ((DRY)); then
		echo "    $shown"
		return 0
	fi
	echo "+ $shown" >>"$LOG"
	"$@" </dev/null >>"$LOG" 2>&1
}
ct() { pct exec "$CTID" -- "$@" </dev/null; }
ct_run() { run pct exec "$CTID" -- "$@"; }

# ask <frage> [<vorgabe>]: liest eine Zeile nach ANSWER.
ask() {
	printf '%s: ' "$1${2:+ [$2]}"
	read -r ANSWER || die "Keine Eingabe mehr. Abbruch, nichts geändert."
	[ -t 0 ] || echo "$ANSWER"
	[ -n "$ANSWER" ] || ANSWER=${2:-}
}

# ask_valid <frage> <vorgabe> <prüfung> <variable>: fragt, bis die Prüfung
# passt (sie meldet selbst, was nicht passt), und setzt die Variable.
ask_valid() {
	while :; do
		ask "$1" "$2"
		"$3" "$ANSWER" && printf -v "$4" '%s' "$ANSWER" && return 0
	done
}

confirm() {
	while :; do
		ask "$1 (j/n)" "$2"
		case ${ANSWER,,} in
		j | ja | y | yes) return 0 ;;
		n | nein | no) return 1 ;;
		esac
		echo "  Bitte j oder n eingeben."
	done
}

# list_storages <inhalt>: aktive Speicher, die diesen Inhalt aufnehmen.
list_storages() {
	pvesm status --content "$1" | awk 'NR > 1 && $3 == "active" { print $1 }'
}

check_id() {
	[[ $1 =~ ^[1-9][0-9]{2,8}$ ]] || { echo "  Die ID ist eine Zahl von 100 bis 999999999."; return 1; }
	pvesh get /cluster/nextid --vmid "$1" >/dev/null 2>&1 || { echo "  Die ID $1 ist schon vergeben."; return 1; }
}
check_name() {
	[[ $1 =~ ^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ ]] ||
		{ echo "  Der Name besteht aus Buchstaben, Ziffern und Bindestrichen."; return 1; }
}
check_store() {
	grep -qxF -- "$1" <<<"$ROOT_STORES" ||
		{ echo "  Speicher $1 gibt es nicht oder er nimmt keine Container auf. Möglich: ${ROOT_STORES//$'\n'/, }"; return 1; }
}
check_bridge() {
	ip link show dev "$1" >/dev/null 2>&1 || { echo "  Die Bridge $1 gibt es auf diesem Host nicht."; return 1; }
}
check_net() {
	[ "$1" = dhcp ] || [[ $1 =~ $CIDR_RE ]] ||
		{ echo "  Bitte dhcp oder eine IPv4-Adresse mit Präfix eingeben, z. B. 192.168.1.50/24."; return 1; }
}
check_gw() {
	[[ $1 =~ $IPV4_RE ]] || { echo "  Bitte eine IPv4-Adresse eingeben, z. B. 192.168.1.1."; return 1; }
}
# Die Werte landen in der Env-Datei, die systemd liest; dort haben diese
# Zeichen eine eigene Bedeutung.
check_value() {
	case $1 in
	*[[:space:]\"\'\\\$\`]*)
		echo "  Anführungszeichen, \\, \$, \` und Leerzeichen gehen hier nicht. Den Wert bitte später in $ENV_FILE eintragen."
		return 1
		;;
	esac
}
check_mqtt() {
	[ -n "$1" ] || return 0
	check_value "$1" || return 1
	if ! [[ $1 =~ $MQTT_RE ]] || ((10#${1##*:} < 1 || 10#${1##*:} > 65535)); then
		echo "  Erwartet wird mqtt://host:port oder mqtts://host:port."
		return 1
	fi
}
# check_backup <datei oder adresse>: lädt eine Adresse herunter und prüft, dass
# das Archiv eine stashbert.db enthält. Setzt BACKUP_FILE.
check_backup() {
	local src=$1 url list
	BACKUP_FILE=''
	[ -n "$src" ] || return 0
	if [[ $src =~ ^https?:// ]]; then
		url=${src%/}
		[[ $url == */api/v1/backup ]] || url=$url/api/v1/backup
		echo "  Lade $url"
		curl -fsSL --connect-timeout 10 -o "$TMP/backup.tar.gz" "$url" || { echo "  Download fehlgeschlagen."; return 1; }
		src=$TMP/backup.tar.gz
	elif [ ! -f "$src" ] || [ ! -r "$src" ]; then
		echo "  Die Datei $src gibt es nicht oder sie ist nicht lesbar."
		return 1
	fi
	list=$(tar -tzf "$src" 2>/dev/null) || { echo "  Das ist kein tar.gz-Archiv."; return 1; }
	grep -qxE '(\./)?stashbert\.db' <<<"$list" ||
		{ echo "  Das Archiv enthält keine stashbert.db. Erwartet wird ein Backup aus GET /api/v1/backup."; return 1; }
	BACKUP_FILE=$src
}

for arg in "$@"; do
	case $arg in
	--dry-run) DRY=1 ;;
	-h | --help) usage && exit 0 ;;
	*) die "Unbekanntes Argument: $arg (Hilfe: --help)" ;;
	esac
done

[ "$(id -u)" = 0 ] || die "Das Skript muss als root auf dem Proxmox-Host laufen."
for c in pct pveam pvesm pvesh; do
	command -v "$c" >/dev/null 2>&1 || die "$c nicht gefunden. Das Skript gehört auf den Proxmox-Host."
done
umask 077
TMP=$(mktemp -d)

ROOT_STORES=$(list_storages rootdir)
TMPL_STORES=$(list_storages vztmpl)
[ -n "$ROOT_STORES" ] || die "Kein aktiver Speicher für Container (pvesm status --content rootdir)."
[ -n "$TMPL_STORES" ] || die "Kein aktiver Speicher für Vorlagen (pvesm status --content vztmpl)."
DEF_STORE=$(grep -xF local-lvm <<<"$ROOT_STORES" || head -n 1 <<<"$ROOT_STORES")
DEF_ID=$(pvesh get /cluster/nextid)
[[ $DEF_ID =~ ^[0-9]+$ ]] || die "pvesh get /cluster/nextid lieferte keine ID: $DEF_ID"

STEP=Fragen
echo "StashBert in einem neuen Container einrichten. Enter übernimmt die Vorgabe in [ ]."
((DRY == 0)) || echo "Probelauf (--dry-run): Es wird nichts geändert."
if confirm "Standardeinstellungen verwenden (ID $DEF_ID, Name stashbert, Speicher $DEF_STORE, Bridge vmbr0, DHCP)?" j; then
	CTID=$DEF_ID NAME=stashbert STORE=$DEF_STORE BRIDGE=vmbr0 NET=dhcp
	check_bridge vmbr0 || die "Ohne vmbr0 bitte die Standardeinstellungen ablehnen und eine Bridge angeben."
else
	ask_valid "Container-ID" "$DEF_ID" check_id CTID
	ask_valid "Name" stashbert check_name NAME
	ask_valid "Speicher für den Container" "$DEF_STORE" check_store STORE
	ask_valid "Bridge" vmbr0 check_bridge BRIDGE
	ask_valid "IP-Adresse: dhcp oder mit Präfix, z. B. 192.168.1.50/24" dhcp check_net NET
	if [ "$NET" != dhcp ]; then
		ask_valid "Gateway, z. B. 192.168.1.1" "" check_gw GW
	fi
fi
ask_valid "Kontakt für Open Food Facts, z. B. eine E-Mail-Adresse (leer: keine Abfragen)" "" check_value OFF
ask_valid "MQTT-Broker, z. B. mqtt://192.168.1.10:1883 (leer: ohne MQTT)" "" check_mqtt MQTT_URL
if [ -n "$MQTT_URL" ]; then
	ask_valid "MQTT-Benutzer (leer: keiner)" "" check_value MQTT_USER
	while :; do
		printf '%s: ' "MQTT-Passwort (leer: keines)"
		IFS= read -r -s MQTT_PASS || die "Keine Eingabe mehr. Abbruch, nichts geändert."
		echo
		check_value "$MQTT_PASS" && break
	done
fi
ask_valid "Backup übernehmen: Pfad zu einem Backup auf diesem Host oder Adresse einer laufenden Instanz, z. B. http://192.168.1.20:8080 (leer: keins)" "" check_backup BACKUP

net="name=eth0,bridge=$BRIDGE,ip=$NET${GW:+,gw=$GW}"
cat <<EOF

Zusammenfassung:
  Container:        ID $CTID, Name $NAME, Debian 13, unprivilegiert, nesting=1, startet mit dem Host
  Ressourcen:       1 Kern, 512 MB RAM, 512 MB Swap, 8 GB auf $STORE
  Netz:             $BRIDGE, ${NET/dhcp/DHCP}${GW:+, Gateway $GW}
  Open Food Facts:  ${OFF:-keine Abfragen}
  MQTT:             ${MQTT_URL:-ohne}${MQTT_USER:+, Benutzer $MQTT_USER}${MQTT_PASS:+, mit Passwort}
  Backup:           ${BACKUP:-keins}
EOF
confirm "Container so anlegen?" j || { echo "Abgebrochen, nichts geändert."; exit 0; }
echo
if ((DRY == 0)); then
	LOG=$(mktemp "${TMPDIR:-/tmp}/stashbert-proxmox-XXXXXX")
	echo "# proxmox.sh, $(date -u +%Y-%m-%dT%H:%M:%SZ), Container $CTID" >"$LOG"
	echo "Protokoll: $LOG"
fi

# Die neueste Vorlage aus der Liste von pveam oder, falls schon geladen, von
# einem Speicher für Vorlagen.
step "Vorlage für Debian 13 suchen"
run pveam update || echo "  Hinweis: pveam update ist fehlgeschlagen, es gilt die vorhandene Liste."
have=''
for s in $TMPL_STORES; do
	have+=$(pveam list "$s" | awk '$1 ~ /:vztmpl\/debian-13-standard_.*_amd64\.tar\./ { print $1 }')$'\n'
done
avail=$(pveam available --section system | awk '$2 ~ /^debian-13-standard_.*_amd64\.tar\./ { print $2 }')
TEMPLATE=$({ echo "$avail"; awk -F/ 'NF { print $NF }' <<<"$have"; } | awk NF | sort -V | tail -n 1)
[ -n "$TEMPLATE" ] || die "Keine Vorlage debian-13-standard für amd64 gefunden (pveam available --section system)."
VOLID=$(awk -F/ -v t="$TEMPLATE" '$NF == t { print; exit }' <<<"$have")
if [ -z "$VOLID" ]; then
	tstore=$(grep -xF local <<<"$TMPL_STORES" || head -n 1 <<<"$TMPL_STORES")
	step "Vorlage $TEMPLATE nach $tstore laden"
	run pveam download "$tstore" "$TEMPLATE"
	VOLID=$tstore:vztmpl/$TEMPLATE
fi

step "Container $CTID anlegen ($VOLID)"
run pct create "$CTID" "$VOLID" --hostname "$NAME" --unprivileged 1 --features nesting=1 \
	--cores 1 --memory 512 --swap 512 --rootfs "$STORE:8" --net0 "$net" --onboot 1 --tags stashbert
((DRY)) || CREATED=1

step "Container starten und auf das Netz warten (höchstens 60 s)"
run pct start "$CTID"
IP=''
if ((DRY == 0)); then
	end=$((SECONDS + 60))
	while [ -z "$IP" ] && ((SECONDS < end)); do
		sleep 2
		IP=$(ct hostname -I 2>>"$LOG" | awk '{ for (i = 1; i <= NF; i++) if ($i ~ /^[0-9.]+$/) { print $i; exit } }') || IP=''
	done
	[ -n "$IP" ] || die "Der Container hat nach 60 s keine IPv4-Adresse. Bridge, DHCP bzw. Adresse und Gateway prüfen."
	echo "  Adresse: $IP"
	# Ohne eigene Angabe übernimmt Proxmox die DNS-Server des Hosts. Einer, den
	# nur der Host erreicht (127.0.0.1, Tailscale 100.100.100.100), lässt apt
	# und den Download von StashBert scheitern.
	until ct getent hosts deb.debian.org >/dev/null 2>&1; do
		if ((SECONDS >= end)); then
			dns=$(ct sed -n 's/^nameserver[[:space:]]*//p' /etc/resolv.conf | paste -sd ' ' -) || dns=''
			die "Der Container kann keine Namen auflösen (DNS-Server im Container: ${dns:-keiner}). Proxmox übernimmt die DNS-Server des Hosts; einer, den nur der Host erreicht (z. B. 127.0.0.1 oder 100.100.100.100 von Tailscale), geht im Container nicht. Abhilfe: pct set $CTID --nameserver <IP des Routers>, dann pct reboot $CTID und prüfen mit pct exec $CTID -- getent hosts deb.debian.org."
		fi
		sleep 2
	done
fi

step "Pakete aktualisieren, curl installieren"
ct_run "${APT[@]}" update --error-on=any
ct_run "${APT[@]}" upgrade
ct_run "${APT[@]}" install curl ca-certificates

step "StashBert installieren (stashbert-update)"
ct_run curl -fsSL -o /root/stashbert-update "$BASE/latest/download/stashbert-update"
ct_run env "STASHBERT_RELEASES_URL=$BASE" sh /root/stashbert-update
ct_run rm -f /root/stashbert-update
((DRY)) || echo "  Version: $(ct /usr/local/bin/stashbert -version)"

if [ -n "$OFF$MQTT_URL" ]; then
	step "Einstellungen in $ENV_FILE eintragen"
	SHOW="pct exec $CTID -- env OFF_CONTACT=$OFF MQTT_URL=$MQTT_URL MQTT_USERNAME=$MQTT_USER MQTT_PASSWORD=${MQTT_PASS:+***} sh -c <Zeilen ersetzen>" \
		ct_run env "OFF_CONTACT=$OFF" "MQTT_URL=$MQTT_URL" "MQTT_USERNAME=$MQTT_USER" "MQTT_PASSWORD=$MQTT_PASS" sh -c "$ENV_EDIT"
fi

if [ -n "$BACKUP_FILE" ]; then
	step "Backup einspielen (stashbert-restore)"
	run pct push "$CTID" "$BACKUP_FILE" /root/stashbert-backup.tar.gz
	ct_run /usr/local/bin/stashbert-restore /root/stashbert-backup.tar.gz
	ct_run rm -f /root/stashbert-backup.tar.gz
fi

step "Dienst stashbert neu starten"
ct_run systemctl restart stashbert
if ((DRY == 0)); then
	sleep 3
	ct systemctl is-active --quiet stashbert || die "Der Dienst stashbert läuft nicht. Details im Container: journalctl -u stashbert"
fi

# /usr/bin/update und die automatische Anmeldung wie bei den community-scripts,
# hier aber für alle Konsolen: Die Konsole in Proxmox nimmt die erste freie.
step "Befehl update und automatische Anmeldung auf der Konsole einrichten"
cat >"$TMP/update" <<'EOF'
#!/bin/sh
# Aktualisiert StashBert auf das neueste Release (ADR-0019).
exec /usr/local/bin/stashbert-update "$@"
EOF
cat >"$TMP/override.conf" <<'EOF'
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin root --noclear --keep-baud tty%I 115200,38400,9600 $TERM
EOF
run pct push "$CTID" "$TMP/update" /usr/bin/update --perms 0755
ct_run mkdir -p "$GETTY_DIR"
run pct push "$CTID" "$TMP/override.conf" "$GETTY_DIR/autologin.conf" --perms 0644
ct_run systemctl daemon-reload
ct_run systemctl restart 'container-getty@*.service'
if ((DRY == 0)); then
	ct test -x /usr/bin/update || die "/usr/bin/update fehlt im Container."
	ct systemctl cat container-getty@1.service | grep -q -- '--autologin root' ||
		die "Die automatische Anmeldung greift nicht (systemctl cat container-getty@1.service)."
fi

if ((DRY)); then
	echo
	echo "Probelauf beendet, nichts geändert."
	exit 0
fi
PORT=$(ct sed -n 's/^PORT=//p' "$ENV_FILE")
MAC=$(pct config "$CTID" | sed -n 's/^net0:.*hwaddr=\([0-9A-Fa-f:]*\).*/\1/p')
cat <<EOF

StashBert läuft im Container $CTID.

  Adresse:      http://$IP:${PORT:-8080}
  MAC-Adresse:  ${MAC:-unbekannt} (für eine feste Zuordnung per DHCP im Router)
  Update:       update in der Konsole des Containers, auf dem Host: pct exec $CTID -- update
  Kamera:       Das iPhone braucht HTTPS über den eigenen Reverse Proxy, siehe
                docs/betrieb.md, Abschnitt "Reverse Proxy":
                https://github.com/schmitz-chris/stashbert/blob/main/docs/betrieb.md
  Protokoll:    $LOG
EOF
