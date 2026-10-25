#!/usr/bin/env bash
# Prove scripts/sbom-license-parity.sh fails on each way the three signed SBOMs
# can disagree, and passes on each way they can differ harmlessly.
#
# The drift it exists to catch was invisible for as long as `sbom-parity`
# compared file sets alone, so the cases below are the forms of the bug rather
# than one example of it: a package in one document only, a mapped purl whose
# declared license is wrong, and a mapped purl whose CONCLUDED license is wrong
# while its declared license is right.
#
# The harmless differences matter as much. syft states a license in one document
# and NOASSERTION in another for the same package, and the formats disagree on
# the spelling and the operand order of an expression. A check that failed on
# those would be turned off within a week.
#
# Fixtures are planted documents, and the script under test is the real one.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
failures=0

# plant writes the three documents of one scenario from a compact description:
# each argument is `purl|cyclonedx|spdx22-declared|spdx22-concluded|spdx30-declared|spdx30-concluded`,
# and an empty field means the document states no license there. A `-` for a
# whole document leaves the package out of it.
plant() {
	local dir="$1"
	shift
	mkdir -p "$dir"
	python3 - "$dir" "$@" <<'PY'
import json, sys

dir, rows = sys.argv[1], sys.argv[2:]
cdx, s22, graph, licenses = [], [], [], {}


def element(name):
    """One license element per name, so two packages under one license share it."""
    if name not in licenses:
        licenses[name] = f"SPDXRef:ListedLicense-{len(licenses)}"
        graph.append({"type": "expandedlicensing_ListedLicense",
                      "spdxId": licenses[name], "name": name})
    return licenses[name]


for index, row in enumerate(rows):
    purl, in_cdx, declared, concluded, s30_declared, s30_concluded = row.split("|")
    if in_cdx != "-":
        component = {"purl": purl, "type": "library"}
        if in_cdx:
            component["licenses"] = [{"license": {"id": in_cdx}}]
        cdx.append(component)
    if declared != "-":
        package = {"name": purl, "externalRefs": [
            {"referenceType": "purl", "referenceCategory": "PACKAGE-MANAGER",
             "referenceLocator": purl}]}
        if declared:
            package["licenseDeclared"] = declared
        if concluded:
            package["licenseConcluded"] = concluded
        s22.append(package)
    if s30_declared == "-":
        continue
    spdx_id = f"SPDXRef:Package-{index}"
    graph.append({"type": "software_Package", "spdxId": spdx_id, "software_packageUrl": purl})
    for kind, value in (("hasDeclaredLicense", s30_declared), ("hasConcludedLicense", s30_concluded)):
        target = element(value) if value else (
            "https://spdx.org/rdf/3.0.1/terms/ExpandedLicensing/NoAssertionLicense")
        graph.append({"type": "Relationship", "relationshipType": kind,
                      "from": spdx_id, "to": [target]})

json.dump({"components": cdx}, open(f"{dir}/margince.cdx.json", "w"))
json.dump({"packages": s22}, open(f"{dir}/margince.spdx221.json", "w"))
json.dump({"@graph": graph}, open(f"{dir}/margince.spdx300.json", "w"))
PY
}

# case_is runs the real script over one planted scenario and holds its verdict.
case_is() {
	local name="$1" want="$2" carries="$3" map="$4"
	shift 4
	local dir="$work/$(printf '%s' "$name" | tr -c 'a-zA-Z0-9' '-')"
	plant "$dir" "$@"
	local status=0
	bash "$root/scripts/sbom-license-parity.sh" "$dir" "$map" >"$dir/out" 2>&1 || status=$?
	local text
	text="$(cat "$dir/out")"
	if [[ "$want" == pass && $status -ne 0 ]] || [[ "$want" == fail && $status -eq 0 ]]; then
		echo "FAIL: $name — exit $status, want $want"
	elif [[ "$text" != *"$carries"* ]]; then
		echo "FAIL: $name — output does not carry '$carries'"
	else
		echo "ok: $name"
		return
	fi
	sed 's/^/    /' "$dir/out"
	failures=$((failures + 1))
}

echo '{"pkg:pypi/ply": "BSD-3-Clause"}' >"$work/map.json"
echo '{}' >"$work/empty.json"

mapped="pkg:pypi/ply@3.11|BSD-3-Clause|BSD-3-Clause|BSD-3-Clause|BSD-3-Clause|BSD-3-Clause"

case_is "three agreeing documents pass" pass "name no conflicting license" "$work/map.json" \
	"$mapped"

case_is "a package in one document alone fails" fail "listed by CycloneDX alone" \
	"$work/empty.json" "pkg:npm/only-here@1.0.0|MIT|-|-|-|-"

case_is "a mapped purl with the wrong declared license fails" fail \
	"the supplement map says BSD-3-Clause" "$work/map.json" \
	"pkg:pypi/ply@3.11|BSD-3-Clause|BSD-3-Clause|BSD-3-Clause|MIT|BSD-3-Clause"

case_is "a mapped purl with no concluded license fails" fail \
	"concluded says" "$work/map.json" \
	"pkg:pypi/ply@3.11|BSD-3-Clause|BSD-3-Clause|BSD-3-Clause|BSD-3-Clause|"

case_is "one document stating nothing is not a conflict" pass "name no conflicting license" \
	"$work/empty.json" "pkg:golang/example.com/x@v1.0.0|MIT|NOASSERTION|NOASSERTION|MIT|MIT"

case_is "a reordered choice is one license" pass "name no conflicting license" \
	"$work/empty.json" \
	"pkg:npm/type-fest@4.41.0|MIT OR CC0-1.0|(CC0-1.0 OR MIT)|(CC0-1.0 OR MIT)||"

case_is "a regrouped mixed expression is not one license" fail "and SPDX 2.2 says" \
	"$work/empty.json" \
	"pkg:npm/mixed@1.0.0|(MIT OR Apache-2.0) AND BSD-3-Clause|MIT OR (Apache-2.0 AND BSD-3-Clause)|||"

if [[ $failures -ne 0 ]]; then
	echo "FAIL: test-sbom-license-parity — $failures case(s) failed"
	exit 1
fi
echo "OK: sbom-license-parity fails on every way the three SBOMs can disagree"
