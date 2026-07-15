#!/usr/bin/env bash
# Packages GramGrabber Studio as a macOS drag-to-Applications DMG:
# universal binary (lipo) → .app bundle with icon → compressed DMG.
# Run ./build.sh first (it produces the per-arch binaries in dist/).
set -euo pipefail
cd "$(dirname "$0")"

VERSION=$(git describe --tags --always 2>/dev/null || echo dev)
DIST=dist
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

ARM="$DIST/gram-grabber-studio_mac-apple-silicon"
INTEL="$DIST/gram-grabber-studio_mac-intel"
[ -f "$ARM" ] && [ -f "$INTEL" ] || { echo "Run ./build.sh first."; exit 1; }

echo "→ universal binary (arm64 + x86_64)"
lipo -create "$ARM" "$INTEL" -output "$WORK/gram-grabber-studio"

echo "→ icon"
cat > "$WORK/icon.html" <<'EOF'
<!DOCTYPE html><html><head><style>
  body{margin:0;width:1024px;height:1024px;background:transparent;display:grid;place-items:center}
  .tile{width:832px;height:832px;border-radius:186px;display:grid;place-items:center;
    background:linear-gradient(160deg,#0d1226 0%,#0a0d1c 55%,#131c42 100%);
    box-shadow:inset 0 6px 30px rgba(120,150,255,.14)}
  svg{width:520px;height:520px}
</style></head><body>
  <div class="tile"><svg viewBox="0 0 32 32"><path d="M29 4 3 14.4l7.6 3.2L26 7 13.2 19.5l.5 7.5 4.4-5 6.3 2.7L29 4Z" fill="#8fd8f2"/></svg></div>
</body></html>
EOF
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
  --headless=new --disable-gpu --default-background-color=00000000 \
  --window-size=1024,1024 --screenshot="$WORK/icon-1024.png" \
  "file://$WORK/icon.html" 2>/dev/null

ICONSET="$WORK/AppIcon.iconset"
mkdir -p "$ICONSET"
for size in 16 32 128 256 512; do
  sips -z $size $size "$WORK/icon-1024.png" --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
  dbl=$((size * 2))
  sips -z $dbl $dbl "$WORK/icon-1024.png" --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$WORK/AppIcon.icns"

echo "→ app bundle"
APP="$WORK/GramGrabber Studio.app"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$WORK/gram-grabber-studio" "$APP/Contents/MacOS/GramGrabber Studio"
cp "$WORK/AppIcon.icns" "$APP/Contents/Resources/"
cat > "$APP/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>GramGrabber Studio</string>
  <key>CFBundleDisplayName</key><string>GramGrabber Studio</string>
  <key>CFBundleIdentifier</key><string>dev.gramgrabber.studio</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>GramGrabber Studio</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
codesign --force --deep --sign - "$APP" 2>/dev/null || true

echo "→ DMG"
STAGE="$WORK/dmg"
mkdir -p "$STAGE"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
hdiutil create -volname "GramGrabber Studio" -srcfolder "$STAGE" \
  -ov -format UDZO "$DIST/GramGrabber-Studio-mac.dmg" >/dev/null

echo
echo "Done: $DIST/GramGrabber-Studio-mac.dmg"
ls -lh "$DIST/GramGrabber-Studio-mac.dmg"
