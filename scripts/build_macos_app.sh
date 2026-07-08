#!/usr/bin/env bash
# Build a double-clickable macOS app: "Job Scorer.app". Opens a native window
# (no Terminal, no browser). Requires Xcode Command Line Tools.
#
#   ./scripts/build_macos_app.sh [output_dir]
set -euo pipefail

cd "$(dirname "$0")/.."
OUT="${1:-dist}"
APP="$OUT/Job Scorer.app"
VERSION="${VERSION:-1.0.0}"

echo "Building Job Scorer.app (version $VERSION)..."
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

CGO_ENABLED=1 go build -tags desktop -trimpath -ldflags "-s -w" \
  -o "$APP/Contents/MacOS/job-scorer" .

# App icon: build AppIcon.icns from favico.png if present.
ICON_KEY=""
if [ -f favico.png ]; then
  echo "Generating app icon from favico.png..."
  ICONSET="$(mktemp -d)/AppIcon.iconset"
  mkdir -p "$ICONSET"
  for size in 16 32 128 256 512; do
    sips -z "$size" "$size" favico.png --out "$ICONSET/icon_${size}x${size}.png" >/dev/null
    sips -z "$((size * 2))" "$((size * 2))" favico.png --out "$ICONSET/icon_${size}x${size}@2x.png" >/dev/null
  done
  iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
  ICON_KEY="  <key>CFBundleIconFile</key><string>AppIcon</string>"
fi

cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>Job Scorer</string>
  <key>CFBundleDisplayName</key><string>Job Scorer</string>
  <key>CFBundleIdentifier</key><string>ai.biie.jobscorer</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleExecutable</key><string>job-scorer</string>
  <key>CFBundlePackageType</key><string>APPL</string>
${ICON_KEY}
  <key>LSMinimumSystemVersion</key><string>10.15</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
PLIST

echo "Built: $APP"
echo "Double-click it in Finder, or run: open \"$APP\""
