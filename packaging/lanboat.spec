Name: lanboat
Version: %{app_version}
Release: 1
Summary: Lanboat desktop proxy client
License: GPL-3.0-or-later
Requires: (libX11 or libX11-6), zenity, xdg-utils, (iproute or iproute2), polkit, util-linux
%global debug_package %{nil}
%global __os_install_post %{nil}
%description
Desktop client for proxy connections, DNS and routing.
%install
install -Dm755 %{app_root}/build/bin/lanboat %{buildroot}/usr/bin/lanboat
install -Dm644 %{app_root}/packaging/lanboat.desktop %{buildroot}/usr/share/applications/lanboat.desktop
install -Dm644 %{app_root}/packaging/lanboat.svg %{buildroot}/usr/share/icons/hicolor/scalable/apps/lanboat.svg
mkdir -p %{buildroot}/usr/share/doc/lanboat
cp -r %{app_root}/build/notices/. %{buildroot}/usr/share/doc/lanboat/
%files
/usr/bin/lanboat
/usr/share/applications/lanboat.desktop
/usr/share/icons/hicolor/scalable/apps/lanboat.svg
/usr/share/doc/lanboat/
