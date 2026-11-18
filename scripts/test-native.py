#!/usr/bin/env python3
"""Run FLTK regressions on an isolated X11 display."""
import os
from pathlib import Path
import select
import shutil
import subprocess
import sys

root = Path(__file__).resolve().parents[1]
cache = root / '.cache'
cache.mkdir(exist_ok=True)
binary = root / (sys.argv[1] if len(sys.argv) > 1 else '.cache/native-ui.test')
if len(sys.argv) == 1:
    subprocess.run(['bash', '-ec', 'source scripts/build-env.sh; go test -tags fltk -c -o "$1" ./internal/native',
                    'native-test', str(binary)], cwd=root, check=True)

env = dict(os.environ, WAYLAND_DISPLAY='lanboat-disabled', LANBOAT_FLTK_TEST='1')
xvfb = shutil.which('Xvfb')
if xvfb is None:
    sysroot = cache / 'build-sysroot'
    xvfb = str(sysroot / 'usr/bin/Xvfb')
    multiarch = subprocess.check_output(['gcc', '-dumpmachine'], text=True).strip()
    env['LD_LIBRARY_PATH'] = str(sysroot / 'usr/lib' / multiarch) + ':' + env.get('LD_LIBRARY_PATH', '')

args = sys.argv[2:] or ['-test.v', '-test.run', 'TestNativeWindow|TestLatePoll']
r, w = os.pipe()
with (cache / 'ui-xvfb.log').open('w') as log:
    try:
        server = subprocess.Popen([xvfb, '-displayfd', str(w), '-screen', '0', '1600x1000x24',
                                   '-nolisten', 'tcp', '-ac'], pass_fds=(w,), env=env, stdout=log, stderr=subprocess.STDOUT)
    finally:
        os.close(w)
    try:
        with os.fdopen(r) as stream:
            if not select.select([stream], [], [], 8)[0]:
                raise RuntimeError('Xvfb did not start; see .cache/ui-xvfb.log')
            display = stream.readline().strip()
        if not display:
            raise RuntimeError('Xvfb failed; see .cache/ui-xvfb.log')
        env['DISPLAY'] = ':' + display
        pattern = '.'
        for i, arg in enumerate(args):
            if arg == '-test.run':
                pattern = args[i + 1]
            elif arg.startswith('-test.run='):
                pattern = arg.split('=', 1)[1]
        if any(arg.startswith('-test.bench') for arg in args):
            subprocess.run([str(binary), *args], cwd=root / 'internal/native', env=env, check=True, timeout=45)
        else:
            names = subprocess.check_output([str(binary), '-test.list', pattern],
                                            cwd=root / 'internal/native', env=env, text=True).splitlines()
            # FLTK owns global locks and its first UI thread; isolate each GUI test in a process.
            for name in names:
                if name.startswith('Test'):
                    subprocess.run([str(binary), *args, '-test.run=^' + name + '$'],
                                   cwd=root / 'internal/native', env=env, check=True, timeout=45)
    finally:
        server.terminate()
        server.wait(5)
