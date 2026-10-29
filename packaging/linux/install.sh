#!/usr/bin/env bash
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
go_bin=$(command -v go) || { echo 'Install Go 1.25+ from https://go.dev/dl/ first.' >&2; exit 1; }
go_dir=$(dirname -- "$go_bin")
data_home=${XDG_DATA_HOME:-$HOME/.local/share}
[[ "$data_home" == /* && "$data_home" != *$'\n'* && "$data_home" != *$'\r'* ]] || { echo 'XDG_DATA_HOME must be an absolute single-line path.' >&2; exit 1; }
app="$data_home/gobook"
mkdir -p "$app" "$data_home/applications"
chmod 700 "$app"
install -m 755 "$here/gobook" "$app/gobook.new"
mv -f -- "$app/gobook.new" "$app/gobook"
cp -- "$here/icon.svg" "$app/icon.svg"
{
  printf '#!/bin/bash\numask 077\nexport PATH=%q:"$PATH"\n' "$go_dir"
  printf 'exec %q -desktop "$@" >> %q 2>&1\n' "$app/gobook" "$app/host.log"
} > "$app/run-gobook"
chmod 755 "$app/run-gobook"
# Desktop Entry strings and Exec arguments have separate escaping layers.
quoted=$(printf '%s' "$app/run-gobook" | sed 's/\\/\\\\\\\\/g; s/"/\\\\"/g; s/`/\\\\`/g; s/\$/\\\\$/g; s/%/%%/g')
icon=$(printf '%s' "$app/icon.svg" | sed 's/\\/\\\\/g')
cat > "$data_home/applications/io.github.Alifarid0011.GoBook.desktop" <<ENTRY
[Desktop Entry]
Type=Application
Name=GoBook Local
Comment=Go notebooks with an automatically managed local host
Exec="$quoted"
Icon=$icon
Terminal=false
Categories=Development;
StartupNotify=false
ENTRY
if command -v update-desktop-database >/dev/null 2>&1; then update-desktop-database "$data_home/applications"; fi
printf 'Installed GoBook Local. Open it from your application menu.\nHost log: %s/host.log\n' "$app"
