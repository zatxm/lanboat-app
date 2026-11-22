#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
os=${1:?OS} arch=${2:?architecture} output=${3:?output}
flags="-s -w -buildid="
suffix=
if [[ $os == windows ]]; then suffix=.exe; fi
export GOOS="$os" GOARCH="$arch" CGO_ENABLED=0
directory=$(dirname "$output")
mkdir -p "$directory"
gui_flags="$flags"
if [[ $os == windows ]]; then gui_flags+=" -H=windowsgui"; fi
go build -mod=readonly -trimpath -buildvcs=true -ldflags "$gui_flags" -o "$output" .
go build -mod=readonly -trimpath -buildvcs=true -ldflags "$flags" -o "$directory/lanboat-core$suffix" ./cmd/lanboat-core
go build -mod=readonly -trimpath -buildvcs=true -ldflags "$gui_flags" -o "$directory/lanboat-tun-worker$suffix" ./cmd/lanboat-tun-worker
if [[ $os == windows ]]; then
  for binary in "$output" "$directory/lanboat-core.exe" "$directory/lanboat-tun-worker.exe"; do
    upx -9 -q "$binary"
    upx -t -q "$binary"
  done
fi
