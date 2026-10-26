#!/usr/bin/env bash
# The license gate judges a dependency by the license its own package declares.
# That reading stops at the module boundary, and a module can carry compiled
# code built from source under a different license: a `.wasm` is a build output,
# so the SBOM sees the Go module's license and never the license of what the
# wasm was compiled from.
#
# `go-pdfium` is the live case in this tree. The module is Apache-2.0 and the
# 5.7MB `pdfium.wasm` it embeds is built from PDFium, which is BSD-3-Clause over
# a stack that carries its own notices. Both are on the allow list, so nothing
# was admitted that should not have been — but nothing CHECKED either, and the
# next such dependency may not be so lucky.
#
# This check refuses an embedded binary that no one has declared, and a
# declaration whose license the allow list does not carry.
#
# WHAT THE CORPUS IS, and why it is not a directory walk. Walking the module
# cache finds about 8000 of these, nearly all `testdata/` and `examples/`
# fixtures that no build reaches. `go list -json -deps` answers the narrower
# question this check asks: which packages are in the three programs, and which
# files do they embed. That is 5 binaries today.
#
# The npm side is NOT covered here. Detecting a `.wasm` inside a package needs
# an installed tree rather than the lockfile, which is a different shape of
# check; the issue for it is named in docs/reference/supply-chain.md.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# The two inputs are overridable so the test beside this script can plant a
# declaration file with one entry wrong and run THIS code against it. The scan
# is never overridable: a check whose corpus a caller can replace is a check
# that can be turned off from outside.
declared_file="${EMBEDDED_BINARIES_FILE:-${repo_root}/embedded-binaries.txt}"
grant_file="${EMBEDDED_BINARIES_GRANT:-${repo_root}/.grant.yaml}"

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

[[ -f ${declared_file} ]] || fail "no declaration file at embedded-binaries.txt"
[[ -f ${grant_file} ]] || fail "no .grant.yaml to read the allow list from"

# The allow list comes from the file the license gate itself reads, so a license
# admitted here is one that gate admits too.
# `next` after each entry, because the rule below reads $0 and the sub() above
# rewrites it: without it the first license looks like the end of the list.
allowed="$(awk '
    /^allow:/ { inlist = 1; next }
    inlist && /^[[:space:]]*-[[:space:]]/ {
        sub(/^[[:space:]]*-[[:space:]]*/, "")
        sub(/[[:space:]]*#.*$/, "")
        print
        next
    }
    inlist && /^[^[:space:]-]/ { inlist = 0 }
' "${grant_file}")"
[[ -n ${allowed} ]] || fail ".grant.yaml names no allowed licenses; the parse above is wrong"

# Each line: module | file | built from | license of that source.
declared="$(grep -vE '^[[:space:]]*(#|$)' "${declared_file}" || true)"

found="$(cd "${repo_root}/backend" && go list -json -deps ./cmd/... | python3 -c '
import json, os, sys

raw = sys.stdin.read()
decoder = json.JSONDecoder()
at = 0
packages = []
while at < len(raw):
    while at < len(raw) and raw[at] in " \n\t\r":
        at += 1
    if at >= len(raw):
        break
    package, at = decoder.raw_decode(raw, at)
    packages.append(package)

def is_binary(path):
    """A file no reviewer can read as text is one whose provenance needs saying."""
    with open(path, "rb") as handle:
        probe = handle.read(8192)
    if b"\x00" in probe:
        return True
    printable = sum(1 for byte in probe if byte >= 32 or byte in (9, 10, 13))
    return bool(probe) and printable / len(probe) < 0.95

for package in packages:
    for name in package.get("EmbedFiles", []):
        path = os.path.join(package["Dir"], name)
        if os.path.exists(path) and is_binary(path):
            print(package["Module"]["Path"] + "|" + name)
' | sort -u)"

[[ -n ${found} ]] || fail "the scan found no embedded binaries at all, which the tree is known to carry; the walk above is wrong"


# Both of these answer a question rather than read a verdict out of a pipe: with
# pipefail set, a grep that matches nothing fails the pipeline, so the exit
# status of `printf | grep -q` says two different things at once.
license_allowed() {
    local wanted="$1" line
    while IFS= read -r line; do
        [[ ${line} == "${wanted}" ]] && return 0
    done <<<"${allowed}"
    return 1
}

scan_holds() {
    local wanted="$1" line
    while IFS= read -r line; do
        [[ ${line} == "${wanted}" ]] && return 0
    done <<<"${found}"
    return 1
}

status=0
while IFS='|' read -r module file; do
    [[ -n ${module} ]] || continue
    # The fields are compared as copies. Assigning to $1 or $2 would make awk
    # rebuild the record with its output separator and drop the pipes the
    # license below is read with.
    line="$(printf '%s\n' "${declared}" | awk -F'|' -v m="${module}" -v f="${file}" '
        {
            mod = $1; emb = $2
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", mod)
            gsub(/^[[:space:]]+|[[:space:]]+$/, "", emb)
            if (mod == m && emb == f) { print; exit }
        }
    ')"
    if [[ -z ${line} ]]; then
        echo "UNDECLARED: ${module} embeds ${file}" >&2
        echo "  Say what it was built from and that source's license, in embedded-binaries.txt." >&2
        status=1
        continue
    fi
    license="$(printf '%s\n' "${line}" | awk -F'|' '{ lic = $4; gsub(/^[[:space:]]+|[[:space:]]+$/, "", lic); print lic }')"
    if [[ -z ${license} ]]; then
        echo "NO LICENSE: ${module} ${file} is declared without naming its source's license" >&2
        status=1
        continue
    fi
    if ! license_allowed "${license}"; then
        echo "NOT ALLOWED: ${module} ${file} is built from ${license}, which .grant.yaml does not allow" >&2
        status=1
    fi
done <<<"${found}"

# A declaration for something the build no longer carries is removed, so the
# file cannot grow into a list nobody reads.
while IFS='|' read -r module file _rest; do
    [[ -n ${module} ]] || continue
    module="$(printf '%s' "${module}" | awk '{ gsub(/^[[:space:]]+|[[:space:]]+$/, ""); print }')"
    file="$(printf '%s' "${file}" | awk '{ gsub(/^[[:space:]]+|[[:space:]]+$/, ""); print }')"
    if ! scan_holds "${module}|${file}"; then
        echo "STALE: ${module} ${file} is declared but the build no longer embeds it" >&2
        status=1
    fi
done <<<"${declared}"

if ((status != 0)); then
    fail "an embedded binary is undeclared, or its source license is not allowed"
fi

echo "OK: every embedded binary in the three programs names its source and that source's license"
