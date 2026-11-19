#!/bin/bash
# Runs the verilated ml_phy_tb. The testbench always ends with $finish, so
# its exit code does not tell pass from fail. Its last line does:
#   ml_phy_tb W=4 skew=00010302: 300 packets checked of 300, 0 errors -> PASS
set -euo pipefail
out="$("$1" 2>&1)" || { printf '%s\n' "$out"; echo "ml_phy_tb exited with an error"; exit 1; }
printf '%s\n' "$out"
if ! grep -q -- '-> PASS$' <<<"$out"; then
  echo "ml_phy_tb did not report PASS"
  exit 1
fi
