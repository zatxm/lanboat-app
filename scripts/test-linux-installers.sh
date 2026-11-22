#!/usr/bin/env bash
# Run each installer with its native package manager in a disposable container.
set -euo pipefail
cd "$(dirname "$0")/.."
arch=${1:?architecture required}
version=${2:?version required}
app_version=${3:-$version}
case "$arch" in
  amd64) machine=x86_64; arch_image=archlinux:base ;;
  arm64) machine=aarch64; arch_image=ghcr.io/menci/archlinuxarm:base ;;
  *) exit 2 ;;
esac
test "$(uname -m)" = "$machine"

failed=()
for target in ubuntu fedora opensuse arch; do
  case "$target" in
    ubuntu) image=ubuntu:24.04 ;;
    fedora) image=fedora:44 ;;
    opensuse) image=opensuse/tumbleweed:latest ;;
    arch) image=$arch_image ;;
  esac
  echo "Testing $target/$arch installers"
  if docker run --rm -i --platform "linux/$arch" --mount "type=bind,source=$PWD/dist,target=/dist,readonly" \
    "$image" bash -euo pipefail -s -- "$target" "$arch" "$machine" "$version" "$app_version" <<'CONTAINER'
target=$1 arch=$2 machine=$3 version=$4 app_version=$5
test "$(uname -m)" = "$machine"
case "$target" in
  ubuntu)
    useradd -m lanboat
    export DEBIAN_FRONTEND=noninteractive SUDO_USER=lanboat
    apt-get update -qq
    apt-get install -y --no-install-recommends "/dist/lanboat_${version}_$arch.deb" xvfb xauth x11-utils
    test -x /usr/libexec/lanboat-tun-worker
    user_home=/home/lanboat
    app="$user_home/.local/bin/lanboat"
    test -x "$user_home/.local/opt/lanboat/lanboat-core"
    test "$(runuser -u lanboat -- "$app" --version)" = "$app_version"
    test -f "$user_home/.local/share/applications/lanboat.desktop"
    runuser -u lanboat -- xvfb-run -a bash -euc '
      export LANBOAT_HOME="$HOME/.lanboat"
      "$1" >"$HOME/launch.log" 2>&1 &
      pid=$!
      trap "kill $pid 2>/dev/null || true; wait $pid || true" EXIT
      for attempt in {1..100}; do
        if xwininfo -root -tree | grep -F "(\"lanboat\" \"lanboat\")" >/dev/null; then exit 0; fi
        kill -0 "$pid" 2>/dev/null || break
        sleep 0.2
      done
      cat "$HOME/launch.log" >&2
      exit 1
    ' _ "$app"
    touch "$user_home/.lanboat/release-test"
    apt-get purge -y lanboat
    test ! -e "$app"
    test -f "$user_home/.lanboat/release-test"

    work=$(mktemp -d)
    chmod 755 "$work"
    name="lanboat-$version-linux-$arch"
    tar -xzf "/dist/$name.tar.gz" -C "$work"
    prefix="$user_home/portable"
    for attempt in 1 2; do
      runuser -u lanboat -- env LANBOAT_INSTALL_PREFIX="$prefix" "$work/$name/install.sh"
    done
    test "$(runuser -u lanboat -- "$prefix/bin/lanboat" --version)" = "$app_version"
    runuser -u lanboat -- env LANBOAT_INSTALL_PREFIX="$prefix" "$work/$name/uninstall.sh"
    test ! -e "$prefix/bin/lanboat"
    test -f "$user_home/.lanboat/release-test"
    exit 0
    ;;
  fedora)
    dnf -y --setopt=install_weak_deps=False install "/dist/lanboat-$version-1.$machine.rpm"
    ;;
  opensuse)
    zypper --non-interactive --gpg-auto-import-keys refresh
    zypper --non-interactive install --allow-unsigned-rpm --no-recommends "/dist/lanboat-$version-1.$machine.rpm"
    ;;
  arch)
    pacman_options=(--noconfirm)
    # The ARM runner's Docker sandbox rejects the nested Landlock filesystem sandbox.
    if [[ $arch == arm64 ]]; then
      pacman_options+=(--disable-sandbox-filesystem)
    fi
    pacman -Syu "${pacman_options[@]}"
    pacman -U "${pacman_options[@]}" "/dist/lanboat-$version-1-$machine.pkg.tar.zst"
    ;;
esac
test "$(lanboat --version)" = "$app_version"
test -x /usr/libexec/lanboat-tun-worker
test -x /usr/libexec/lanboat-core
test -f /usr/share/applications/lanboat.desktop
for dependency in ip pkexec zenity xdg-open; do
  command -v "$dependency" || { echo "$target/$arch: missing installed command $dependency" >&2; exit 1; }
done
case "$target" in
  arch) pacman -R --noconfirm lanboat ;;
  *) rpm -e lanboat ;;
esac
test ! -e /usr/bin/lanboat
CONTAINER
  then
    echo "$target/$arch installer tests passed"
  else
    failed+=("$target/$arch")
  fi
done
if (( ${#failed[@]} > 0 )); then
  printf 'Installer tests failed: %s\n' "${failed[*]}" >&2
  exit 1
fi
