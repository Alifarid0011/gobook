#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist/site/examples
cp -R site/. dist/site/
cp docs/workspace.jpg dist/site/workspace.jpg
cp examples/penguins.gobook.json dist/site/examples/
touch dist/site/.nojekyll
