#!/usr/bin/env bash
# Builds the Windows x64 installer (NSIS) with the Claude bridge inside.
# Runs in Git Bash on Windows, or on macOS/Linux with makensis installed
# (the Windows build cross-compiles).
#
#   scripts/package-windows.sh [version] [out-dir]
#
# The version defaults to wails.json info.productVersion. With
# WINDOWS_SIGN_CERT (a .pfx file) and WINDOWS_SIGN_PASSWORD set, the app
# and the installer are signed with signtool.
set -euo pipefail
cd "$(dirname "$0")/.."
version=${1:-$(node -p "require('./wails.json').info.productVersion")}
out=${2:-dist}

sign() {
  [ -n "${WINDOWS_SIGN_CERT:-}" ] || return 0
  signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 \
    /f "$WINDOWS_SIGN_CERT" /p "$WINDOWS_SIGN_PASSWORD" "$1"
}

ldflags="-X main.appVersion=$version"
wails build -clean -platform windows/amd64 -ldflags "$ldflags"
node scripts/stage-runner.mjs build/bin/claude-runner --os win32 --cpu x64
sign build/bin/agent-office.exe
# Second pass only wraps the existing build into the installer: -s skips
# the frontend, and project.nsi picks up build/bin/claude-runner.
wails build -s -nsis -platform windows/amd64 -ldflags "$ldflags"
sign build/bin/agent-office.exe

mkdir -p "$out"
setup="$out/AgentOffice-$version-windows-x64-setup.exe"
mv build/bin/agent-office-amd64-installer.exe "$setup"
sign "$setup"
echo "$setup"
