#!/usr/bin/env bash
# Builds a .deb for Ubuntu 24.04 LTS (x64) with the Claude bridge inside.
# Run on Linux with libgtk-3-dev and libwebkit2gtk-4.1-dev installed.
#
#   scripts/package-linux.sh [version] [out-dir]
#
# Layout: /usr/lib/agent-office/{agent-office,claude-runner/,diagram-design/},
# /usr/bin/agent-office (link), a desktop entry and an icon.
set -euo pipefail
cd "$(dirname "$0")/.."
version=${1:-$(node -p "require('./wails.json').info.productVersion")}
out=${2:-dist}

wails build -clean -platform linux/amd64 -tags webkit2_41 -ldflags "-X main.appVersion=$version"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
root="$work/agent-office"
install -Dm755 build/bin/agent-office "$root/usr/lib/agent-office/agent-office"
node scripts/stage-runner.mjs "$root/usr/lib/agent-office/claude-runner" --os linux --cpu x64
# The diagram-design skill given to diagram steps.
cp -R third_party/diagram-design "$root/usr/lib/agent-office/diagram-design"
mkdir -p "$root/usr/bin"
ln -s ../lib/agent-office/agent-office "$root/usr/bin/agent-office"
install -Dm644 build/linux/agent-office.desktop "$root/usr/share/applications/agent-office.desktop"
install -Dm644 build/appicon.png "$root/usr/share/pixmaps/agent-office.png"

# Debian versions may not contain '-' unless they have a revision.
debver=${version//-/\~}
mkdir -p "$root/DEBIAN"
cat > "$root/DEBIAN/control" <<CONTROL
Package: agent-office
Version: $debver
Architecture: amd64
Maintainer: J <jwj4016@gmail.com>
Installed-Size: $(du -sk "$root/usr" | cut -f1)
Depends: libgtk-3-0t64 | libgtk-3-0, libwebkit2gtk-4.1-0
Recommends: nodejs (>= 18), git
Suggests: gnome-keyring
Section: devel
Priority: optional
Homepage: https://github.com/jwj4016/agent-office
Description: AI와 사람이 함께 일하는 사무실
 Agent Office runs workflows where AI assistants (Claude, Codex) and
 people work together, with approvals and reviews by the person.
CONTROL

mkdir -p "$out"
deb="$out/AgentOffice-$version-linux-amd64.deb"
dpkg-deb --build --root-owner-group "$root" "$deb"
echo "$deb"
