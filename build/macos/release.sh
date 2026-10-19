#!/usr/bin/env bash
set -euo pipefail

VERSION="${1:?version is required}"
ARCH="${2:?darwin architecture is required}"
OUTPUT="${3:?output archive is required}"
MACOS_ARCH_FLAGS="${MACOS_ARCH_FLAGS:-}"

case "$ARCH" in
  arm64) EXPECTED_ARCH=arm64 ;;
  amd64) EXPECTED_ARCH=x86_64 ;;
  *) echo "unsupported macOS architecture: $ARCH" >&2; exit 2 ;;
esac

make icons
VERSION="$VERSION" GOOS=darwin GOARCH="$ARCH" CGO_ENABLED=1 MACOS_ARCH_FLAGS="$MACOS_ARCH_FLAGS" make app

test "$(lipo -archs agent-link.app/Contents/MacOS/agent-link)" = "$EXPECTED_ARCH"
test ! -e agent-link.app/Contents/Resources/Assets.car
plutil -extract CFBundleExecutable raw -o - agent-link.app/Contents/Info.plist | grep -Fx agent-link
plutil -extract CFBundleShortVersionString raw -o - agent-link.app/Contents/Info.plist | grep -Fx "$VERSION"

# Apple Developer ID signing and notarization require a paid developer account.
# Ad-hoc signing keeps the bundle structurally valid and lets macOS verify its
# contents; users still need the one-time right-click Open approval.
xattr -cr agent-link.app 2>/dev/null || true
codesign --force --deep --sign - agent-link.app
codesign --verify --deep --strict --verbose=2 agent-link.app

test -z "$(xattr -lr agent-link.app 2>/dev/null | grep -v '^$' || true)"
ditto -c -k --sequesterRsrc --keepParent agent-link.app "$OUTPUT"
