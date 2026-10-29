#!/usr/bin/env bash
set -euo pipefail
version=${1:?usage: build-linux.sh VERSION ARCH}
arch=${2:?usage: build-linux.sh VERSION ARCH}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][A-Za-z0-9.-]+)?$ ]] || { echo 'Invalid release version' >&2; exit 1; }
[[ "$arch" == amd64 || "$arch" == arm64 ]] || { echo 'Unsupported architecture' >&2; exit 1; }
root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
name="gobook-${version}-linux-${arch}"
stage="dist/$name"
mkdir -p "$stage"
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o "$stage/gobook" .
cp packaging/linux/* "$stage/"
cp docs/desktop.md "$stage/README.md"
cp LICENSE "$stage/"
install -m 644 third_party/yaegi-LICENSE.txt "$stage/THIRD_PARTY_NOTICES.txt"
if [[ -n "${GOBOOK_GOPLS_BINARY:-}" ]]; then
  install -m 755 "$GOBOOK_GOPLS_BINARY" "$stage/gopls"
  "$GOBOOK_GOPLS_BINARY" licenses >> "$stage/THIRD_PARTY_NOTICES.txt"
fi
chmod +x "$stage/gobook" "$stage/gobook.sh" "$stage/install.sh"
tar -C dist -czf "dist/$name.tar.gz" "$name"
(cd dist && sha256sum "$name.tar.gz" > "$name.tar.gz.sha256")
