#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version=1.4.5
checksum=eede1fb2b8e9c2e581e77082e15252145855c79aad30070ee3b24aabe2f926f1
archive=".cache/fltk-$version-source.tar.gz"
source_dir=".cache/fltk-$version"
mkdir -p .cache tools/fltk/lib tools/fltk/include
if [[ ! -f "$archive" ]]; then
  curl --fail --location --retry 2 "https://github.com/fltk/fltk/releases/download/release-$version/fltk-$version-source.tar.gz" -o "$archive"
fi
printf '%s  %s\n' "$checksum" "$archive" | sha256sum -c -
[[ -f "$source_dir/configure" ]] || tar -xzf "$archive" -C .cache
if [[ ! -f "$source_dir/lib/libfltk.a" ]]; then
  (
    cd "$source_dir"
    # The application uses only 2D widgets. Xft retains antialiased UTF-8 text.
    CPPFLAGS="${CPPFLAGS:-} $(pkg-config --cflags x11 xft)" \
    LDFLAGS="${LDFLAGS:-} $(pkg-config --libs-only-L x11 xft)" \
      ./configure --disable-gl --disable-wayland --disable-pango --disable-cairo \
      --disable-usecairo --disable-forms --disable-test --disable-fluid \
      --disable-svg --disable-print --enable-threads --with-optim='-O2 -fPIC' \
      > ../fltk-configure.log 2>&1
    make -C src -j"${LANBOAT_BUILD_JOBS:-4}" ../lib/libfltk.a > ../fltk-build.log 2>&1
  )
fi
cp -R "$source_dir/FL" tools/fltk/include/
install -m 644 "$source_dir/lib/libfltk.a" tools/fltk/lib/
install -m 644 "$source_dir/COPYING" tools/fltk/COPYING.FLTK
printf 'Built FLTK %s, X11/Xft, no OpenGL or GTK.\n' "$version"
