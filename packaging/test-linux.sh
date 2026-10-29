#!/usr/bin/env bash
set -euo pipefail
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-$root}/gobook-launcher-test.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
mkdir -p "$tmp/package with spaces" "$tmp/bin" "$tmp/empty"
cp "$root/packaging/linux/gobook.sh" "$tmp/package with spaces/"
cat > "$tmp/package with spaces/gobook" <<'STUB'
#!/bin/bash
printf '%s\n' "$@" > "$GOBOOK_TEST_ARGS"
STUB
chmod +x "$tmp/package with spaces/gobook"
ln -s "$(command -v go)" "$tmp/bin/go"
ln -s "$(command -v dirname)" "$tmp/bin/dirname"
export GOBOOK_TEST_ARGS="$tmp/args"
PATH="$tmp/bin" /bin/bash "$tmp/package with spaces/gobook.sh" -chartframe 'a path with spaces'
printf '%s\n' -open -addr 127.0.0.1:0 -chartframe 'a path with spaces' > "$tmp/expected"
cmp "$tmp/expected" "$tmp/args"
if PATH="$tmp/empty" /bin/bash "$tmp/package with spaces/gobook.sh" > "$tmp/missing" 2>&1; then
  echo 'Launcher unexpectedly accepted a missing Go compiler' >&2
  exit 1
fi
grep -q 'Go compiler.*https://go.dev/dl/' "$tmp/missing"
cp "$root/packaging/linux/install.sh" "$root/packaging/linux/icon.svg" "$tmp/package with spaces/"
XDG_DATA_HOME="$tmp/app data" bash "$tmp/package with spaces/install.sh"
XDG_DATA_HOME="$tmp/app data" bash "$tmp/package with spaces/install.sh"
GOBOOK_TEST_ARGS="$tmp/installed-args" "$tmp/app data/gobook/run-gobook" -open=false
printf '%s\n' -desktop -open=false > "$tmp/expected-installed"
cmp "$tmp/expected-installed" "$tmp/installed-args"
grep -q '^Terminal=false$' "$tmp/app data/applications/io.github.Alifarid0011.GoBook.desktop"
grep -q '^Name=GoBook Local$' "$tmp/app data/applications/io.github.Alifarid0011.GoBook.desktop"
if command -v desktop-file-validate >/dev/null 2>&1; then desktop-file-validate "$tmp/app data/applications/io.github.Alifarid0011.GoBook.desktop"; fi
echo 'Linux launcher and installation checks passed'
