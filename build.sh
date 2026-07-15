#!/usr/bin/env bash
# Cross-compiles GramGrabber (CLI + Studio) for macOS, Windows and Linux.
# Usage: ./build.sh            -> builds everything into dist/
set -euo pipefail
cd "$(dirname "$0")"

VERSION=$(git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS="-s -w"
DIST=dist

rm -rf "$DIST"
mkdir -p "$DIST"

build() {
  local goos=$1 goarch=$2 label=$3
  local ext=""
  [ "$goos" = "windows" ] && ext=".exe"

  echo "→ $label"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
    go build -trimpath -ldflags "$LDFLAGS" \
    -o "$DIST/gram-grabber_${label}${ext}" .
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch \
    go build -trimpath -ldflags "$LDFLAGS" \
    -o "$DIST/gram-grabber-studio_${label}${ext}" ./cmd/studio
}

build darwin  arm64 mac-apple-silicon
build darwin  amd64 mac-intel
build windows amd64 windows
build linux   amd64 linux

echo
echo "Binaries in $DIST/ (version $VERSION):"
ls -lh "$DIST"
