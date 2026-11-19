#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
os=${1:?OS} arch=${2:?architecture} output=${3:?output}
flags="-s -w -buildid="
if [[ $os == windows ]]; then flags+=" -H=windowsgui"; fi
mkdir -p "$(dirname "$output")"
GOWORK=off GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=true -ldflags "$flags" -o "$output" .
if [[ $os != darwin ]]; then
  upx -9 -q "$output"
  upx -t -q "$output"
fi
