#!/usr/bin/env bash
# Checks that xezim was built with the module's clang against its sysroot,
# not with the host's compiler and C library, and that the module's clang
# did not leak into the consumer's own C builds.
#   hermetic_test.sh <xezim, target config> <xezim, exec config> <consumer .so>
set -euo pipefail

check_xezim() {
  local bin
  bin="$(readlink -f "$1")"
  echo "== $1"
  grep -aq "clang version 23.1.5" "${bin}" \
    || { echo "FAIL: no 'clang version 23.1.5' in the binary"; exit 1; }

  # Compiler idents land in the .comment section. The only GCC there may be
  # the sysroot's own startup files, from Debian bullseye's GCC 10.2.1.
  local gcc
  gcc="$(grep -aoE 'GCC: \([^)]*\) [0-9.]+' "${bin}" | sort -u | grep -vF 'GCC: (Debian 10.2.1-6) 10.2.1' || true)"
  [[ -z "${gcc}" ]] || { echo "FAIL: a host GCC built part of xezim: ${gcc}"; exit 1; }

  # The sysroot has glibc 2.31. A newer symbol version means the host's C
  # library was linked in.
  local newest
  newest="$(grep -aoE 'GLIBC_2\.[0-9]+(\.[0-9]+)?' "${bin}" | sort -uV | tail -1)"
  echo "newest glibc symbol version: ${newest}"
  [[ "$(printf '%s\nGLIBC_2.31\n' "${newest}" | sort -V | tail -1)" == GLIBC_2.31 ]] \
    || { echo "FAIL: ${newest} is newer than the sysroot's glibc 2.31"; exit 1; }
}

check_xezim "$1"
check_xezim "$2"

# The consumer's library must come from the consumer's own C toolchain.
if grep -aq "clang version 23.1.5" "$(readlink -f "$3")"; then
  echo "FAIL: $3 was built with xezim's clang; its toolchain leaked"
  exit 1
fi
echo PASS
