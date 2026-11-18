#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off CGO_ENABLED=0
go build -trimpath -mod=readonly -buildvcs=false -ldflags '-s -w -buildid=' -o build/bin/lanboat .
