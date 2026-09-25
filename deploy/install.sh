#!/bin/sh
# Installiert oder aktualisiert StashBert als systemd-Dienst im Debian-LXC
# (ADR-0014, ADR-0019, architecture.md 9). Aufruf als root im Container:
#
#   sh install.sh [pfad-zum-binary]
#
# Ohne Argument nimmt das Skript das Binary stashbert neben sich, wie im
# Release-Archiv. stashbert.service, stashbert.env.example, stashbert-update
# und stashbert-restore liegen ebenfalls neben diesem Skript.
# Ein erneuter Aufruf ersetzt Binary, Skripte und Unit und startet den Dienst
# neu; die Env-Datei bleibt unverändert.
set -eu

bin_dir=/usr/local/bin
bin=$bin_dir/stashbert
unit=/etc/systemd/system/stashbert.service
conf_dir=/etc/stashbert
env_file=$conf_dir/stashbert.env

die() {
	echo "Fehler: $*" >&2
	exit 1
}

[ "$#" -le 1 ] || die "Aufruf: sh install.sh [pfad-zum-binary]"

[ "$(id -u)" -eq 0 ] || die "install.sh muss als root laufen."
command -v systemctl >/dev/null 2>&1 ||
	die "systemctl nicht gefunden. install.sh braucht einen Container mit systemd."

script_dir=$(cd "$(dirname "$0")" && pwd)
src=${1:-$script_dir/stashbert}
for f in stashbert.service stashbert.env.example stashbert-update stashbert-restore; do
	[ -f "$script_dir/$f" ] || die "$f fehlt neben install.sh (in $script_dir)."
done
[ -f "$src" ] || die "Binary nicht gefunden: $src"

# Binary und Skripte kommen zuerst als temporäre Datei neben das Ziel und
# ersetzen die alte Datei erst danach per mv. So sieht ein laufender Dienst nie
# ein halb geschriebenes Binary, und ein laufendes stashbert-update, das dieses
# Skript aufgerufen hat, liest sein altes Skript unverändert zu Ende.
tmp=$bin_dir/.stashbert.new
trap 'rm -f "$tmp" "$bin_dir/.stashbert-update.new" "$bin_dir/.stashbert-restore.new"' EXIT
trap 'exit 1' HUP INT TERM
install -m 0755 "$src" "$tmp"
version=$("$tmp" -version) ||
	die "Das Binary $src läuft nicht (-version schlug fehl). Erwartet wird stashbert aus dem Release-Archiv stashbert_<version>_linux_amd64.tar.gz."

if id -u stashbert >/dev/null 2>&1; then
	echo "Systembenutzer stashbert ist vorhanden."
else
	useradd --system --user-group --home-dir /var/lib/stashbert --shell /usr/sbin/nologin stashbert
	echo "Systembenutzer stashbert angelegt."
fi

mv -f "$tmp" "$bin"
echo "Binary installiert: $bin ($version)"

for f in stashbert-update stashbert-restore; do
	install -m 0755 "$script_dir/$f" "$bin_dir/.$f.new"
	mv -f "$bin_dir/.$f.new" "$bin_dir/$f"
	echo "Skript installiert: $bin_dir/$f"
done

install -m 0644 "$script_dir/stashbert.service" "$unit"
echo "Unit installiert: $unit"

mkdir -p "$conf_dir"
chown root:stashbert "$conf_dir"
chmod 0750 "$conf_dir"
if [ -e "$env_file" ]; then
	echo "Konfiguration bleibt unverändert: $env_file"
else
	install -m 0640 -o root -g stashbert "$script_dir/stashbert.env.example" "$env_file"
	echo "Konfiguration neu angelegt: $env_file"
	echo "Bitte dort OFF_CONTACT setzen (sonst keine Abfragen bei Open Food Facts) und danach: systemctl restart stashbert"
fi

systemctl daemon-reload
if systemctl is-enabled --quiet stashbert; then
	systemctl restart stashbert
	echo "Dienst stashbert neu gestartet."
else
	systemctl enable --now stashbert
	echo "Dienst stashbert aktiviert und gestartet."
fi

installed=$("$bin" -version) || die "$bin -version schlug fehl."
echo "Installierte Version: $installed"
# Kurz warten, damit ein Dienst, der gleich nach dem Start abstürzt, nicht
# noch als laufend gemeldet wird.
sleep 3
systemctl --no-pager --lines=5 status stashbert ||
	die "Der Dienst läuft nicht. Details: journalctl -u stashbert"
