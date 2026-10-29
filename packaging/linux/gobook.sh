#!/usr/bin/env bash
set -euo pipefail
if ! command -v go >/dev/null 2>&1; then
  echo 'Go compiler not found. Install Go 1.25 or newer from https://go.dev/dl/ and add go to PATH.' >&2
  exit 1
fi
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
exec "$here/gobook" -open -addr 127.0.0.1:0 "$@"
