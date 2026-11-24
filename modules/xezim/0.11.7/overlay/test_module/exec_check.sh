#!/usr/bin/env bash
# Checks the output of a genrule that ran xezim as a build tool.
set -euo pipefail
cat "$1"
grep -qxF "count=10" "$1" || { echo "FAIL: no line 'count=10'"; exit 1; }
echo PASS
