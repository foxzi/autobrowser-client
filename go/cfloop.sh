#!/usr/bin/env bash
# Repeat the Cloudflare check: 3 runs per variant, fresh browser and fingerprint each time.
cd "$(dirname "$0")"
for i in 1 2 3; do
  for v in "win -platform windows" "linux -platform linux" \
           "win-stealth -platform windows -stealth" "linux-stealth -platform linux -stealth"; do
    set -- $v; d=$1; shift
    ./run.sh "out-cf-$d-$i" -url https://www.criptonoticias.com/ "$@" > "out-cf-$d-$i.log" 2>&1
  done
done
