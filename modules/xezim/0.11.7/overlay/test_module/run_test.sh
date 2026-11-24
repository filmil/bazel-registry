#!/usr/bin/env bash
# Runs xezim on one SystemVerilog file and checks what it did.
#   run_test.sh <xezim> <source.sv> <want: pass|fail> [expected output line]
set -euo pipefail
xezim="$1" src="$2" want="$3" expect="${4:-}"
out="${TEST_TMPDIR}/out.txt"
fst="${TEST_TMPDIR}/waves.fst"

rc=0
"${xezim}" --no-cache --fst "${fst}" "${src}" >"${out}" 2>&1 || rc=$?
cat "${out}"

if [[ "${want}" == pass ]]; then
  [[ ${rc} -eq 0 ]] || { echo "FAIL: exit ${rc}, want 0"; exit 1; }
  grep -qxF -- "${expect}" "${out}" || { echo "FAIL: no line '${expect}'"; exit 1; }
  [[ -s "${fst}" ]] || { echo "FAIL: no FST written"; exit 1; }
else
  [[ ${rc} -ne 0 ]] || { echo "FAIL: exit 0, want nonzero"; exit 1; }
fi
echo PASS
