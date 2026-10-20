#!/usr/bin/env bash
# Builds "Agent Office.app" with the Claude bridge inside and wraps it in
# a dmg. Run on macOS:
#
#   scripts/package-macos.sh <arm64|amd64> [version] [out-dir]
#
# The version defaults to wails.json info.productVersion.
#
# Without signing credentials the app is ad-hoc signed (Gatekeeper asks
# the person to allow it once). With MACOS_SIGN_IDENTITY set it is signed
# with that Developer ID, and with MACOS_NOTARY_PROFILE (a notarytool
# keychain profile) the dmg is notarized and stapled.
set -euo pipefail
arch=${1:?arch: arm64 or amd64}
cd "$(dirname "$0")/.."
version=${2:-$(node -p "require('./wails.json').info.productVersion")}
out=${3:-dist}
case "$arch" in
arm64) cpu=arm64 ;;
amd64) cpu=x64 ;;
*) echo "unknown arch $arch" >&2; exit 2 ;;
esac

# The bundled Claude binary needs macOS 13 (its LC_BUILD_VERSION minos).
export MACOSX_DEPLOYMENT_TARGET=13.0
export CGO_CFLAGS="-mmacosx-version-min=13.0" CGO_LDFLAGS="-mmacosx-version-min=13.0"
wails build -clean -platform "darwin/$arch" -ldflags "-X main.appVersion=$version"
built=$(find build/bin -maxdepth 1 -name '*.app' | head -1)
app="build/bin/Agent Office.app"
[ "$built" = "$app" ] || mv "$built" "$app"
node scripts/stage-runner.mjs "$app/Contents/Resources/claude-runner" --os darwin --cpu "$cpu"
# The diagram-design skill given to diagram steps (third_party/diagram-design).
ditto third_party/diagram-design "$app/Contents/Resources/diagram-design"

if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
  codesign --force --options runtime --timestamp --entitlements build/darwin/entitlements.plist \
    --sign "$MACOS_SIGN_IDENTITY" "$app/Contents/MacOS/"*
  codesign --force --options runtime --timestamp --entitlements build/darwin/entitlements.plist \
    --sign "$MACOS_SIGN_IDENTITY" "$app"
else
  codesign --force --sign - "$app"
fi
codesign --verify --strict "$app"

mkdir -p "$out"
dmg="$out/AgentOffice-$version-macos-$arch.dmg"
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT
cp -R "$app" "$stage/"
ln -s /Applications "$stage/Applications"
rm -f "$dmg"
hdiutil create -quiet -volname "Agent Office" -srcfolder "$stage" -fs HFS+ -format UDZO "$dmg"

if [ -n "${MACOS_SIGN_IDENTITY:-}" ]; then
  codesign --force --timestamp --sign "$MACOS_SIGN_IDENTITY" "$dmg"
  if [ -n "${MACOS_NOTARY_PROFILE:-}" ]; then
    xcrun notarytool submit "$dmg" --keychain-profile "$MACOS_NOTARY_PROFILE" --wait
    xcrun stapler staple "$dmg"
  fi
fi
echo "$dmg"
