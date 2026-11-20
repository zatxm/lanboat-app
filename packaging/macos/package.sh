#!/usr/bin/env bash
set -euo pipefail
payload=build/macos/payload
app="$payload/Lanboat.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" dist
install -m755 build/bin/Lanboat "$app/Contents/MacOS/Lanboat"
install -m644 packaging/macos/Lanboat.icns "$app/Contents/Resources/"
cp -R build/notices "$app/Contents/Resources/"
cat >"$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>org.lanboat.desktop</string>
<key>CFBundleName</key><string>Lanboat</string>
<key>CFBundleIconFile</key><string>Lanboat.icns</string>
<key>CFBundleExecutable</key><string>Lanboat</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>$LANBOAT_VERSION</string>
<key>CFBundleShortVersionString</key><string>$LANBOAT_VERSION</string>
</dict></plist>
PLIST
codesign --force --sign - "$app"
codesign --verify --strict "$app"
# Keep Installer from updating a copy of the app outside /Applications.
components=build/macos/components.plist
pkgbuild --analyze --root "$payload" "$components"
/usr/libexec/PlistBuddy -c 'Set :0:BundleIsRelocatable false' "$components"
pkgbuild --root "$payload" --component-plist "$components" \
  --install-location /Applications --identifier org.lanboat.desktop \
  --version "$LANBOAT_VERSION" "dist/Lanboat-$LANBOAT_VERSION-macos-$1.pkg"
