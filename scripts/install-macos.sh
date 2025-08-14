#!/usr/bin/env bash
set -euo pipefail

# Simple installer for obsctl GUI on macOS (avoids Finder quarantine)
# - Downloads latest release asset zip from GitHub
# - Unzips to a temp dir, moves the .app into ~/Applications (by default)
# - Removes quarantine attribute (if any) and opens the app

REPO=${REPO:-"mikucrossing/obsctl"}
ASSET=${ASSET:-"obsctl-gui_darwin_universal_midi_native.zip"}
TARGET_DIR=${TARGET_DIR:-"$HOME/Applications"}

echo "[obsctl] Installing GUI for macOS"
echo "  repo      : $REPO"
echo "  asset     : $ASSET"
echo "  target dir: $TARGET_DIR"

mkdir -p "$TARGET_DIR"

TMP_DIR=$(mktemp -d)
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

ZIP_PATH="$TMP_DIR/app.zip"
URL=${URL:-"https://github.com/${REPO}/releases/latest/download/${ASSET}"}

echo "[obsctl] Downloading: $URL"
curl -fL -o "$ZIP_PATH" "$URL"

UNZ_DIR="$TMP_DIR/unz"
mkdir -p "$UNZ_DIR"
echo "[obsctl] Unzipping"
unzip -q "$ZIP_PATH" -d "$UNZ_DIR"

# Find first .app in extracted contents
APP_SRC=$(find "$UNZ_DIR" -maxdepth 2 -name "*.app" -type d | head -n1)
if [[ -z "${APP_SRC:-}" ]]; then
  echo "[obsctl] Error: .app not found in archive" >&2
  exit 1
fi

APP_NAME=$(basename "$APP_SRC")
APP_DST="$TARGET_DIR/$APP_NAME"

echo "[obsctl] Installing: $APP_NAME -> $TARGET_DIR"
rm -rf "$APP_DST"
mv "$APP_SRC" "$APP_DST"

# Remove quarantine if present (ignore errors)
if command -v xattr >/dev/null 2>&1; then
  xattr -dr com.apple.quarantine "$APP_DST" || true
fi

echo "[obsctl] Installed at: $APP_DST"
echo "[obsctl] Launching..."
open "$APP_DST"

echo "[obsctl] Done"

