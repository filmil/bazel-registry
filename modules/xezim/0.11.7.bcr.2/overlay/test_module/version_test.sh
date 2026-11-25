#!/usr/bin/env bash
# Checks that `xezim -V` names the release and the xezim-core rev.
set -euo pipefail
out="$("$1" -V)"
echo "${out}"
grep -qxF "xezim version 0.11.7" <<<"${out}" || { echo "FAIL: version line"; exit 1; }
grep -qxF "tag 0.11.7 (xezim-core ebd78860)" <<<"${out}" || { echo "FAIL: tag line"; exit 1; }
echo PASS
