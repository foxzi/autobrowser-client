#!/usr/bin/env bash
# Run all four variants one after another.
cd "$(dirname "$0")"
for v in "win-nostealth -platform windows" "linux-nostealth -platform linux" \
         "win-stealth -platform windows -stealth" "linux-stealth -platform linux -stealth"; do
  set -- $v; d=$1; shift
  ./run.sh "out-$d" "$@" > "out-$d.log" 2>&1
done
