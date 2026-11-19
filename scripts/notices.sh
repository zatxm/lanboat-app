#!/usr/bin/env bash
set -euo pipefail
dir=build/notices
mkdir -p "$dir/modules"
cp README.md README.zh-CN.md LICENSE THIRD_PARTY.md packaging/COPYING.* internal/routing/assets/sources.json "$dir/"
modules=$(GOWORK=off go list -deps -f '{{with .Module}}{{.Path}} {{.Dir}}{{end}}' . | sort -u)
while read -r name path; do
  [[ -n "$path" ]] || continue
  for file in LICENSE LICENSE.txt LICENSE.md COPYING; do
    if [[ -f "$path/$file" ]]; then install -m 644 "$path/$file" "$dir/modules/${name//\//_}.txt"; break; fi
  done
done <<< "$modules"
