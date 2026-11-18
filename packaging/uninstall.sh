#!/usr/bin/env bash
set -euo pipefail
[[ $(id -u) != 0 ]] || { echo 'Run as your desktop user, without sudo.' >&2; exit 1; }
prefix=${LANBOAT_INSTALL_PREFIX:-"$HOME/.local"}
target=$(realpath -m -- "$prefix/opt/lanboat")
for pid in $(pgrep -u "$(id -u)" -x lanboat || true); do
  executable=$(readlink "/proc/$pid/exe" 2>/dev/null || true)
  case "$executable" in
    "$target/lanboat"|"$target/lanboat (deleted)")
      echo 'Close Lanboat first so it can restore your proxy settings.' >&2; exit 1 ;;
  esac
done
if [[ -L "$prefix/bin/lanboat" && $(readlink "$prefix/bin/lanboat") == "$target/lanboat" ]]; then rm "$prefix/bin/lanboat"; fi
rm -f "$prefix/share/applications/lanboat.desktop" "$prefix/share/icons/hicolor/scalable/apps/lanboat.svg"
for size in 16 32 48 64 128 256; do rm -f "$prefix/share/icons/hicolor/${size}x${size}/apps/lanboat.png"; done
rm -rf -- "$target"
echo 'Uninstalled. Your profiles and logs in ~/.lanboat were preserved.'
