#!/usr/bin/env bash
# Checks that xezim was built with the module's clang against its sysroot,
# not with the host's compiler and C library.
#   hermetic_test.sh <xezim>
set -euo pipefail
bin="$(readlink -f "$1")"

grep -aq "clang version 23.1.5" "${bin}" \
  || { echo "FAIL: no 'clang version 23.1.5' in the binary"; exit 1; }

# The sysroot has glibc 2.31. A newer symbol version means the host's C
# library was linked in.
newest="$(grep -aoE 'GLIBC_2\.[0-9]+(\.[0-9]+)?' "${bin}" | sort -uV | tail -1)"
echo "newest glibc symbol version: ${newest}"
[[ "$(printf '%s\nGLIBC_2.31\n' "${newest}" | sort -V | tail -1)" == GLIBC_2.31 ]] \
  || { echo "FAIL: ${newest} is newer than the sysroot's glibc 2.31"; exit 1; }
echo PASS
