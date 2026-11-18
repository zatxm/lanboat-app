#!/usr/bin/env bash
# Use installed development libraries, or an explicitly prepared local sysroot.
if ! pkg-config --exists x11 xft fontconfig freetype2 xext xrender xcursor xfixes xinerama && [[ -z ${PKG_CONFIG_SYSROOT_DIR:-} && -z ${PKG_CONFIG_LIBDIR:-} ]]; then
  sysroot="$PWD/.cache/build-sysroot"
  multiarch=$(gcc -dumpmachine)
  if [[ -d "$sysroot/usr/lib/$multiarch/pkgconfig" ]]; then
    export PKG_CONFIG_SYSROOT_DIR="$sysroot"
    export PKG_CONFIG_LIBDIR="$sysroot/usr/lib/$multiarch/pkgconfig:$sysroot/usr/share/pkgconfig"
  fi
fi
pkg-config --exists x11 xft fontconfig freetype2 xext xrender xcursor xfixes xinerama || { echo 'Missing FLTK comparison build dependencies. Check pkg-config for X11, Xft, Fontconfig, FreeType, Xext, Xrender, Xcursor, Xfixes and Xinerama.' >&2; return 1; }
