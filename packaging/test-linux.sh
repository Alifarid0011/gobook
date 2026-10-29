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
echo 'Linux launcher checks passed'
