#!/usr/bin/env bash
set -euo pipefail

# Cross-build binaries for Windows/Linux/macOS and create a macOS universal binary if possible.

ROOT_DIR=$(cd "$(dirname "$0")/.."; pwd)
cd "$ROOT_DIR"

mkdir -p dist

VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
COMMIT=${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo none)}
DATE=${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}
LDFLAGS=${LDFLAGS:-"-s -w -X main.version=$VERSION -X main.commit=$COMMIT -X main.date=$DATE"}

PKG=./cmd/obsctl

build() {
  local goos=$1
  local goarch=$2
  local ext=$3
  local out="dist/obsctl_${goos}_${goarch}${ext}"
  echo "[build] $out"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -ldflags "$LDFLAGS" -o "$out" "$PKG"
}

# Windows
build windows amd64 .exe
build windows arm64 .exe

# Linux
build linux amd64 ""
build linux arm64 ""

# macOS (per-arch)
build darwin amd64 ""
build darwin arm64 ""

# macOS universal (if running on macOS with lipo available)
if command -v lipo >/dev/null 2>&1 && [[ "${OSTYPE:-}" == darwin* ]]; then
  echo "[lipo] Creating macOS universal binary..."
  lipo -create \
    -output dist/obsctl_darwin_universal \
    dist/obsctl_darwin_amd64 \
    dist/obsctl_darwin_arm64
  lipo -info dist/obsctl_darwin_universal || true
else
  echo "[lipo] Skipped (not macOS host or lipo not found)"
fi

echo "\nDone. Artifacts:"
ls -la dist

