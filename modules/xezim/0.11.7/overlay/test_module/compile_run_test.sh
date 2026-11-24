#!/usr/bin/env bash
# Compiles a design to an artifact, then runs the artifact as a second step.
# Lets a benchmark time elaboration and simulation separately.
set -euo pipefail
xezim="$1" src="$2" expect="$3"
art="${TEST_TMPDIR}/design.xzc"
"${xezim}" --no-cache --compile -o "${art}" "${src}"
[[ -s "${art}" ]] || { echo "FAIL: no artifact"; exit 1; }
out="$("${xezim}" --no-cache "${art}")"
echo "${out}"
grep -qxF -- "${expect}" <<<"${out}" || { echo "FAIL: no line '${expect}'"; exit 1; }
echo PASS
