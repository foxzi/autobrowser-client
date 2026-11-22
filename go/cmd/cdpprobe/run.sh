#!/usr/bin/env bash
# Run go-cdp inside the parsers-autobrowser image. Usage: ./run.sh <outdir> [go-cdp flags]
set -euo pipefail
cd "$(dirname "$0")"
out=$1; shift
mkdir -p "$out"
docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -v "$PWD/go-cdp:/go-cdp:ro" -v "$PWD/$out:/out" \
  --entrypoint /go-cdp parsers-autobrowser:latest -out /out "$@"
