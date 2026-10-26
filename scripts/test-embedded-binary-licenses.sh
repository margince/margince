#!/usr/bin/env bash
# `embedded-binary-licenses` passes on the committed tree, which says nothing
# about whether it would refuse anything. These cases plant a declaration file
# with one thing wrong and run the real script against the real scan, so the
# comparison is exercised where it can be wrong: the match between a found
# binary and its entry, and the license that entry names.
#
# The scan itself is not planted. Every case reads the live build, so a case
# that stopped finding anything would fail on the script's own empty-scan
# refusal rather than passing quietly.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "${work}"' EXIT

failures=0
real="${root}/embedded-binaries.txt"

# expect runs the script against a planted declaration file and checks both the
# exit status and the word the operator is meant to read.
expect() {
    local name="$1" declared="$2" want_status="$3" want_text="${4:-}"
    local out status=0
    out="$(EMBEDDED_BINARIES_FILE="${declared}" bash "${root}/scripts/embedded-binary-licenses.sh" 2>&1)" || status=$?
    if [[ ${status} -ne ${want_status} ]]; then
        echo "FAIL ${name}: exit ${status}, want ${want_status}"
        printf '%s\n' "${out}" | sed 's/^/    /'
        failures=$((failures + 1))
        return
    fi
    if [[ -n ${want_text} && ${out} != *"${want_text}"* ]]; then
        echo "FAIL ${name}: the output does not say ${want_text}"
        printf '%s\n' "${out}" | sed 's/^/    /'
        failures=$((failures + 1))
        return
    fi
    echo "ok   ${name}"
}

# The committed file is the passing case, and proves the scan and the
# declarations agree as they stand.
expect "the committed declarations pass" "${real}" 0 "OK:"

# An embedded binary nobody declared.
grep -v 'pdfium.wasm' "${real}" > "${work}/missing.txt"
expect "an undeclared binary is refused" "${work}/missing.txt" 1 "UNDECLARED"

# A declaration naming a license the allow list does not carry.
sed 's#| Apache-2.0#| GPL-3.0-only#' "${real}" > "${work}/forbidden.txt"
expect "a license outside the allow list is refused" "${work}/forbidden.txt" 1 "NOT ALLOWED"

# A declaration whose license field is empty.
sed 's#| Apache-2.0##' "${real}" > "${work}/blank.txt"
expect "a declaration with no license is refused" "${work}/blank.txt" 1 "NO LICENSE"

# An entry for something the build does not carry, which would otherwise sit in
# the file forever and read as covering something.
{ cat "${real}"; echo "example.com/gone | ghost.wasm | a dependency that was removed | MIT"; } > "${work}/stale.txt"
expect "an entry the build no longer carries is refused" "${work}/stale.txt" 1 "STALE"

# A license list the parse cannot read must stop the check rather than admit
# everything: an empty allow list makes every license unknown.
: > "${work}/empty.yaml"
out=""
status=0
out="$(EMBEDDED_BINARIES_GRANT="${work}/empty.yaml" bash "${root}/scripts/embedded-binary-licenses.sh" 2>&1)" || status=$?
if [[ ${status} -eq 0 ]]; then
    echo "FAIL an unreadable allow list passed"
    failures=$((failures + 1))
else
    echo "ok   an unreadable allow list stops the check"
fi

if ((failures != 0)); then
    echo "FAIL: ${failures} case(s) failed"
    exit 1
fi
echo "OK: embedded-binary-licenses refuses each way it is meant to"
