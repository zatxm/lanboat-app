#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
os=${1:?OS} arch=${2:?architecture} output=${3:?output}
version=${LANBOAT_VERSION:-1.0.0}
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
flags="-s -w -buildid= -X github.com/zatxm/lanboat-app/internal/appmeta.Version=$version"
if [[ $os == windows ]]; then flags+=" -H=windowsgui"; fi
mkdir -p "$(dirname "$output")"
GOWORK=off GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -mod=readonly -trimpath -buildvcs=false -ldflags "$flags" -o "$output" .
if [[ $os != darwin ]]; then
  upx -9 -q "$output"
  upx -t -q "$output"
fi
