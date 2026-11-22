#!/usr/bin/env python3
"""Exercise deb hooks without root, then optionally inspect a built deb."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile

root = Path(__file__).resolve().parent.parent
if os.getuid() == 0:
    raise SystemExit('Run this check as your ordinary desktop user.')

with tempfile.TemporaryDirectory(prefix='lanboat-deb-user-') as temporary:
    work = Path(temporary)
    payload = work/'payload'
    payload.mkdir()
    for name in ('install.sh', 'uninstall.sh', 'lanboat.svg'):
        shutil.copy2(root/'packaging'/name, payload/name)
    shutil.copytree(root/'packaging/icons', payload/'icons')
    (payload/'licenses').mkdir()
    (payload/'licenses/fixture.txt').write_text('license version 1\n')
    (payload/'licenses/fixture.txt').chmod(0o444)
    binaries = ('lanboat', 'lanboat-core')
    for name in (*binaries, 'COPYING.fixture', 'README.md', 'README.zh-CN.md', 'THIRD_PARTY.md', 'LICENSE'):
        (payload/name).write_text('fixture version 1\n')
    desktop_home = work/'Desktop User'
    data = desktop_home/'.lanboat/config.json'
    data.parent.mkdir(parents=True)
    data.write_text('preserve this profile')
    state = work/'state'
    commands = work/'commands'
    commands.mkdir()
    stubs = {
        'getent': '''#!/bin/sh
[ "$1" = passwd ] || exit 2
case "$2" in
  desktop) printf 'desktop:x:1000:1000::%s:/bin/sh\n' "$TEST_DESKTOP_HOME" ;;
  baduid) printf 'baduid:x:0:0::%s:/bin/sh\n' "$TEST_DESKTOP_HOME" ;;
  badhome) printf 'badhome:x:1000:1000::/:/bin/sh\n' ;;
  *) exit 2 ;;
esac
''',
        'runuser': '''#!/bin/sh
[ "$1" = -u ] && [ "$2" = desktop ] && [ "$3" = -- ] || exit 99
shift 3
exec "$@"
''',
        'pgrep': '#!/bin/sh\nexit 1\n',
        'gtk-update-icon-cache': '#!/bin/sh\nexit 0\n',
        'update-desktop-database': '#!/bin/sh\nexit 0\n',
    }
    for name, content in stubs.items():
        path = commands/name
        path.write_text(content)
        path.chmod(0o755)
    hooks = {}
    for name in ('postinst', 'prerm', 'postrm'):
        path = work/name
        # Redirect only filesystem roots; execute the real hook and user installers.
        path.write_text((root/'packaging/deb'/name).read_text()
                        .replace('/var/lib/lanboat', str(state))
                        .replace('/usr/share/lanboat', str(payload)))
        hooks[name] = path
    env = dict(os.environ, PATH=str(commands)+os.pathsep+os.environ['PATH'],
               SUDO_USER='desktop', TEST_DESKTOP_HOME=str(desktop_home))

    def run(name, action, environment=env, success=True):
        result = subprocess.run(['sh', str(hooks[name]), action], env=environment,
                                text=True, capture_output=True)
        assert (result.returncode == 0) == success, result.stdout+result.stderr

    run('postinst', 'configure')
    prefix = desktop_home/'.local'
    target = prefix/'opt/lanboat'
    assert (state/'install-user').read_text().strip() == 'desktop'
    for name in binaries:
        assert (target/name).read_bytes() == (payload/name).read_bytes()
        assert (target/name).stat().st_uid == os.getuid()
    assert (prefix/'bin/lanboat').resolve() == target/'lanboat'
    for name in ('README.md', 'README.zh-CN.md'):
        assert (target/name).read_bytes() == (payload/name).read_bytes()
    menu = prefix/'share/applications/lanboat.desktop'
    assert f'Exec="{target}/lanboat"' in menu.read_text()
    assert (prefix/'share/icons/hicolor/256x256/apps/lanboat.png').is_file()
    assert data.read_text() == 'preserve this profile'
    (payload/'lanboat').write_text('fixture version 2\n')
    (payload/'licenses/fixture.txt').chmod(0o644)
    (payload/'licenses/fixture.txt').write_text('license version 2\n')
    (payload/'licenses/fixture.txt').chmod(0o444)
    run('prerm', 'upgrade')
    run('postrm', 'upgrade')
    assert (target/'lanboat').is_file() and (state/'install-user').is_file()
    run('postinst', 'configure', dict(env, SUDO_USER='another-admin'))
    assert (target/'lanboat').read_text() == 'fixture version 2\n'
    assert (target/'licenses/fixture.txt').read_text() == 'license version 2\n'
    run('postinst', 'configure', {key: value for key, value in env.items() if key != 'SUDO_USER'})
    run('prerm', 'remove')
    assert not target.exists() and not (prefix/'bin/lanboat').is_symlink()
    assert not menu.exists()
    assert data.read_text() == 'preserve this profile'
    run('postrm', 'remove')
    assert not state.exists()
    for user in ('', 'root', 'missing-account', 'baduid', 'badhome'):
        run('postinst', 'configure', dict(env, SUDO_USER=user), success=False)
        assert not state.exists()
    run('postrm', 'purge')

    if len(sys.argv) > 1:
        package = Path(sys.argv[1]).resolve()
        process = subprocess.Popen(['dpkg-deb', '--fsys-tarfile', str(package)], stdout=subprocess.PIPE)
        with tarfile.open(fileobj=process.stdout, mode='r|') as archive:
            members = {member.name.removeprefix('./'): member for member in archive if member.isfile() or member.issym()}
        names = members.keys()
        helper = members['usr/libexec/lanboat-tun-worker']
        assert helper.isfile() and helper.uid == 0 and helper.mode == 0o755
        process.stdout.close()
        assert process.wait() == 0
        for name in ('lanboat', 'lanboat-core', 'README.md', 'README.zh-CN.md'):
            assert f'usr/share/lanboat/{name}' in names
        assert not any(Path(name).name in ('sing-box','sing-box-vpn','lanboat-tun','libcronet.so') for name in names)
        assert all(name.startswith('usr/share/lanboat/') or name == 'usr/libexec/lanboat-tun-worker' for name in names)
        control = work/'control'
        subprocess.run(['dpkg-deb', '--control', str(package), str(control)], check=True)
        for name in hooks:
            assert (control/name).read_bytes() == (root/'packaging/deb'/name).read_bytes()
            assert (control/name).stat().st_mode & 0o111
        dependencies = subprocess.check_output(['dpkg-deb', '--field', str(package), 'Depends'], text=True)
        assert 'policykit-1' not in dependencies
        assert all(name in dependencies for name in ('pkexec', 'polkitd', 'util-linux'))

print('PASS: user installation, upgrade account retention, removal, profile preservation and invalid-account checks.')
if len(sys.argv) > 1:
    print('PASS: deb layout, executable hooks and runtime dependencies.')
