#!/usr/bin/env bash
# Fetch latest linux/amd64 gobotlite release and replace the local binary.
set -euo pipefail

REPO="lepinkainen/gobotlite"
DEST="${1:-./gobotlite}"   # target path, default ./gobotlite

json=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest")

tag=$(jq -r '.tag_name' <<<"$json")
url=$(jq -r '.assets[] | select(.name | endswith("linux_amd64.tar.gz")) | .browser_download_url' <<<"$json")

[ -n "$url" ] || { echo "no linux_amd64 asset found" >&2; exit 1; }

echo "=== gobotlite $tag ==="
jq -r '.body' <<<"$json"
echo "======================"
echo "downloading $url"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

curl -fsSL "$url" | tar xz -C "$tmp" gobotlite
install -m755 "$tmp/gobotlite" "$DEST"   # atomic overwrite, safe while running
echo "installed $tag -> $DEST"
