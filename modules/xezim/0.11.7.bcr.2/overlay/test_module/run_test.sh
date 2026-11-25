#!/usr/bin/env bash
# Runs xezim and checks what it did.
#   run_test.sh <xezim> <want> <expected output line> [xezim args...]
# want: "waves" (exit 0 and an FST written), "pass" (exit 0), or "fatal"
# (exit 1, from $fatal; a crash or exec failure exits otherwise).
set -euo pipefail
xezim="$1" want="$2" expect="$3"
shift 3
out="${TEST_TMPDIR}/out.txt"
fst="${TEST_TMPDIR}/waves.fst"

rc=0
"${xezim}" --no-cache --fst "${fst}" "$@" >"${out}" 2>&1 || rc=$?
cat "${out}"

case "${want}" in
  waves | pass) [[ ${rc} -eq 0 ]] || { echo "FAIL: exit ${rc}, want 0"; exit 1; } ;;
  fatal) [[ ${rc} -eq 1 ]] || { echo "FAIL: exit ${rc}, want 1"; exit 1; } ;;
  *) echo "FAIL: unknown want '${want}'"; exit 1 ;;
esac
grep -qxF -- "${expect}" "${out}" || { echo "FAIL: no line '${expect}'"; exit 1; }
if [[ "${want}" == waves ]]; then
  [[ -s "${fst}" ]] || { echo "FAIL: no FST written"; exit 1; }
fi
echo PASS
