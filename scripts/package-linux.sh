#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off
version=${LANBOAT_VERSION:-1.0.0}
if [[ ${LANBOAT_PACKAGE_PREBUILT:-0} == 1 ]]; then
  arch=${ARCH:?missing target architecture}
else
  arch=$(go env GOARCH)
  bash scripts/build-release.sh linux "$arch" build/bin/lanboat
  bash scripts/notices.sh
fi
[[ -x build/bin/lanboat ]] || { echo 'Missing build/bin/lanboat' >&2; exit 1; }
mkdir -p dist
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
name="lanboat-$version-linux-$arch"
mkdir -p "$tmp/$name"
install -m 755 build/bin/lanboat packaging/install.sh packaging/uninstall.sh "$tmp/$name/"
install -m 644 packaging/lanboat.svg README.md README.zh-CN.md THIRD_PARTY.md LICENSE "$tmp/$name/"
cp -R packaging/icons "$tmp/$name/icons"
mkdir -p "$tmp/$name/licenses"
install -m 644 packaging/COPYING.* "$tmp/$name/licenses/"
install -m 644 internal/routing/assets/sources.json "$tmp/$name/licenses/routing-data-sources.json"
cp -R build/notices/modules "$tmp/$name/licenses/"
tar -czf "dist/$name.tar.gz" -C "$tmp" "$name"
if command -v dpkg-deb >/dev/null; then
  root="$tmp/deb"
  mkdir -p "$root/DEBIAN" "$root/usr/share/lanboat"
  cp -R "$tmp/$name/." "$root/usr/share/lanboat/"
  install -m 755 packaging/deb/postinst packaging/deb/prerm packaging/deb/postrm "$root/DEBIAN/"
  cat > "$root/DEBIAN/control" <<EOF
Package: lanboat
Version: $version
Architecture: $arch
Maintainer: zatxm
Section: net
Priority: optional
Depends: libx11-6, zenity, iproute2, pkexec, polkitd, util-linux
Recommends: fonts-noto-cjk
Description: Lanboat lightweight multiprotocol desktop client
 Native Fego desktop client installed in the invoking desktop user's .local.
EOF
  dpkg-deb --root-owner-group --build "$root" "dist/lanboat_${version}_${arch}.deb"
fi
(cd dist; sha256sum "$name.tar.gz" > "$name.sha256"; if [[ -f "lanboat_${version}_${arch}.deb" ]]; then sha256sum "lanboat_${version}_${arch}.deb" >> "$name.sha256"; fi)
(sed "s#  #  dist/#" "dist/$name.sha256" > dist/SHA256SUMS)
echo "Packages are in dist/ ($arch)."
