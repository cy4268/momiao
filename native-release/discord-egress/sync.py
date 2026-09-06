"""Adapt the existing fixed-upstream namespace reconciler; never use host networking."""
import json
import os
from pathlib import Path
import subprocess

SERVICE = 'momiao-discord-bridge.service'
NAMESPACE = '/run/chaldea-preview/netns'


def run(*args, check=True):
    return subprocess.run(args, check=check, text=True, capture_output=True, timeout=15).stdout.strip()


try:
    state = json.loads(Path('/run/chaldea-preview/state.json').read_text())
    if state['state'] != 'READY' or not state['namespace_path'].startswith('/var/run/docker/netns/'):
        raise RuntimeError('Native namespace is not ready')
    target = os.stat(NAMESPACE).st_ino
    if target != state['netns_inode'] or target != os.stat(state['namespace_path']).st_ino:
        raise RuntimeError('Native namespace identity changed')
    pid = int(run('systemctl', 'show', SERVICE, '-p', 'MainPID', '--value') or '0')
    if pid and os.stat(f'/proc/{pid}/ns/net').st_ino != target:
        run('systemctl', 'stop', SERVICE)
    run('systemctl', 'start', SERVICE)
    pid = int(run('systemctl', 'show', SERVICE, '-p', 'MainPID', '--value') or '0')
    if not pid or os.stat(f'/proc/{pid}/ns/net').st_ino != target:
        raise RuntimeError('Bridge did not enter the verified namespace')
except Exception:
    run('systemctl', 'stop', SERVICE, check=False)
    raise SystemExit('Native namespace not ready; fixed Discord bridge stopped')
