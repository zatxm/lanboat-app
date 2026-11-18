#!/usr/bin/env python3
"""Measure every app descendant, including the proxy core, using /proc."""
import argparse
import json
import time
from pathlib import Path


def snapshot(root):
    parents = {}
    for path in Path('/proc').iterdir():
        if not path.name.isdigit():
            continue
        try:
            fields = (path / 'stat').read_text().rsplit(')', 1)[1].split()
            parents[int(path.name)] = int(fields[1])
        except (OSError, ValueError, IndexError):
            continue
    processes = []
    for pid in parents:
        ancestor = pid
        seen = set()
        while ancestor != root and ancestor > 1 and ancestor not in seen:
            seen.add(ancestor)
            ancestor = parents.get(ancestor, 0)
        if ancestor != root:
            continue
        try:
            stats = {}
            for line in Path(f'/proc/{pid}/smaps_rollup').read_text().splitlines()[1:]:
                key, value = line.split(':', 1)
                stats[key] = int(value.split()[0]) * 1024
            processes.append({'pid': pid, 'name': Path(f'/proc/{pid}/comm').read_text().strip(),
                              'rss': stats['Rss'], 'pss': stats['Pss']})
        except (OSError, ValueError, KeyError):
            continue
    return {'rss': sum(p['rss'] for p in processes), 'pss': sum(p['pss'] for p in processes), 'processes': processes}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('pid', type=int)
    parser.add_argument('--seconds', type=int, default=30)
    parser.add_argument('--output')
    args = parser.parse_args()
    if not Path(f'/proc/{args.pid}').exists() or args.seconds < 1:
        parser.error('PID must exist and seconds must be positive')
    samples = []
    for i in range(args.seconds):
        samples.append(snapshot(args.pid))
        if i + 1 < args.seconds:
            time.sleep(1)
    report = {'pid': args.pid, 'samples': len(samples), 'peak_rss': max(s['rss'] for s in samples),
              'peak_pss': max(s['pss'] for s in samples), 'target_bytes': 200_000_000,
              'pss_within_target': max(s['pss'] for s in samples) < 200_000_000,
              'rss_within_target': max(s['rss'] for s in samples) < 200_000_000, 'last': samples[-1]}
    result = json.dumps(report, ensure_ascii=False, indent=2)
    print(result)
    if args.output:
        Path(args.output).write_text(result + '\n')
