#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
[[ $(id -u) != 0 ]] || { echo 'Run as your desktop user, without sudo.' >&2; exit 1; }
for lib in libX11.so.6; do
  ldconfig -p 2>/dev/null | grep -F "$lib" >/dev/null || { echo "Missing $lib; install the runtime dependencies in README.md." >&2; exit 1; }
done
prefix=${LANBOAT_INSTALL_PREFIX:-"$HOME/.local"}
target="$prefix/opt/lanboat"
mkdir -p "$target" "$prefix/bin" "$prefix/share/applications" "$prefix/share/icons/hicolor/scalable/apps"
install -m 755 lanboat lanboat-core uninstall.sh "$target/"
for old in sing-box sing-box-vpn libcronet.so naive lanboat-server lanboat-tun lanboat-brook lanboat-lua lanboat-protocols lanboat-sudoku lanboat-hysteria1 lanboat-xray juicity-client lanboat-v2ray mihomo ck-client COPYING.xray COPYING.juicity COPYING.mihomo COPYING.cloak; do
  rm -f "$target/$old"
done
install -m 644 README.md README.en.md THIRD_PARTY.md LICENSE "$target/"
cp -R --remove-destination licenses "$target/"
ln -sfn "$target/lanboat" "$prefix/bin/lanboat"
install -m 644 lanboat.svg "$prefix/share/icons/hicolor/scalable/apps/lanboat.svg"
cat > "$prefix/share/applications/lanboat.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Lanboat
Name[zh_CN]=Lanboat
Name[zh_HK]=Lanboat
Name[zh_TW]=Lanboat
Comment=Lightweight multiprotocol proxy client
Comment[zh_CN]=轻量多协议代理客户端
Comment[zh_HK]=轻量多协议代理客户端
Comment[zh_TW]=轻量多协议代理客户端
Exec="$target/lanboat"
Icon=lanboat
Terminal=false
Categories=Network;
StartupWMClass=lanboat
EOF
for size in 16 32 48 64 128 256; do
 install -D -m 644 "icons/${size}x${size}/lanboat.png" "$prefix/share/icons/hicolor/${size}x${size}/apps/lanboat.png"
done
command -v gtk-update-icon-cache >/dev/null && gtk-update-icon-cache -f -t "$prefix/share/icons/hicolor" >/dev/null 2>&1 || true
command -v update-desktop-database >/dev/null && update-desktop-database "$prefix/share/applications" || true
echo "Installed. Open Lanboat from the application menu, or run: $target/lanboat"
