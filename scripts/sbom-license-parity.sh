#!/usr/bin/env bash
# The three signed SBOMs describe one tree, so they must not name DIFFERENT
# licenses for one package. `sbom-parity` compares their file sets, which is
# why the licenses drifted under it: `sbom-supplement` set them on CycloneDX
# and SPDX 2.2 and left SPDX 3.0 as syft wrote it, so every supplemented
# package disagreed inside a signed document and no gate said so.
#
# Each format states a license its own way:
#
#   CycloneDX   a component's `licenses[]`, as an id, a name or an expression
#   SPDX 2.2    a package's `licenseDeclared` string
#   SPDX 3.0    a `hasDeclaredLicense` relationship pointing at a license
#               ELEMENT, read back through that element's name
#
# Two rules, because the documents are not equally complete and only one of
# those gaps is a defect:
#
#   1. A purl the supplement map covers must carry the mapped license in all
#      three. That is this check's whole reason: a supplement that reaches two
#      documents is the bug it was written for.
#   2. Any other purl may be unstated in one document and named in another —
#      syft resolves Go module licenses for CycloneDX and writes NOASSERTION
#      for the same package in SPDX, on 82 packages here — but where two
#      documents both NAME one, they must name the same one.
#
# An expression is compared without its parentheses and spacing, because
# `MIT OR Apache-2.0` and `(MIT OR Apache-2.0)` are one license written twice.
set -eu

dir="${1:-sboms}"
map="${2:-sbom-schemas/license-supplement.json}"
cdx="$dir/margince.cdx.json"
s22="$dir/margince.spdx221.json"
s30="$dir/margince.spdx300.json"
for f in "$cdx" "$s22" "$s30" "$map"; do
	[ -f "$f" ] || { echo "FAIL: no $f — run 'make sbom' first"; exit 1; }
done

python3 - "$cdx" "$s22" "$s30" "$map" <<'PY'
import json, re, sys

cdx_path, s22_path, s30_path, map_path = sys.argv[1:5]
load = lambda p: json.load(open(p))

UNSTATED = {"", "NOASSERTION", "NONE", "NoAssertionLicense", "NoneLicense"}


def norm(value: str) -> str:
    """One spelling for one license.

    No parentheses, one space, upper case. A choice between licenses is also
    order-free, because the formats do not agree on it: SPDX 2.2 writes
    `(MIT OR CC0-1.0)` where SPDX 3.0's member list reads the other way round.
    Only a single-operator expression is reordered — mixing OR and AND makes
    the order meaningful, and such a spelling is left exactly as it stands.
    """
    flat = re.sub(r"\s+", " ", value.replace("(", " ").replace(")", " ")).strip().upper()
    for operator in (" OR ", " AND "):
        other = " AND " if operator == " OR " else " OR "
        if operator in flat and other not in flat:
            return operator.join(sorted(part.strip() for part in flat.split(operator)))
    return flat


def stated(value: str) -> bool:
    return value not in UNSTATED and norm(value) not in {norm(u) for u in UNSTATED}


def key(purl: str) -> str:
    return re.sub(r"@[^@]*$", "", purl)


cdx = {}
for c in load(cdx_path).get("components", []):
    if not c.get("purl"):
        continue
    names = [l.get("license", {}).get("id") or l.get("license", {}).get("name") or l.get("expression")
             for l in c.get("licenses", [])]
    cdx[c["purl"]] = sorted({n for n in names if n})

s22 = {}
for p in load(s22_path).get("packages", []):
    purls = [r["referenceLocator"] for r in p.get("externalRefs", []) or []
             if r.get("referenceType") == "purl"]
    if purls:
        s22[purls[0]] = [p["licenseDeclared"]] if p.get("licenseDeclared") else []

graph = load(s30_path)["@graph"]
by_id = {e["spdxId"]: e for e in graph if e.get("spdxId")}
SETS = {"expandedlicensing_DisjunctiveLicenseSet": "OR",
        "expandedlicensing_ConjunctiveLicenseSet": "AND"}


def license_name(ref: str, depth: int = 0) -> str:
    """The license an id names, flattening the sets SPDX 3.0 writes.

    A compound license is an element whose members are elements, so a dual
    license reads as an opaque `SPDXRef:DisjunctiveLicenseSet-1` until it is
    walked. The other two formats write it as one expression string.
    """
    element = by_id.get(ref)
    if element is None or depth > 8:
        return ref.rsplit("/", 1)[-1]
    joiner = SETS.get(element.get("type", ""))
    if joiner:
        members = [license_name(m, depth + 1) for m in element.get("expandedlicensing_member", [])]
        return f" {joiner} ".join(sorted(members))
    return element.get("name") or ref.rsplit("/", 1)[-1]
declared: dict[str, list[str]] = {}
for e in graph:
    if e.get("type") == "Relationship" and e.get("relationshipType") == "hasDeclaredLicense":
        declared.setdefault(e["from"], []).extend(
            license_name(t) for t in e.get("to", []))
s30 = {e["software_packageUrl"]: sorted(set(declared.get(e["spdxId"], [])))
       for e in graph
       if e.get("type") == "software_Package" and e.get("software_packageUrl")}

supplement = load(map_path)
docs = {"CycloneDX": cdx, "SPDX 2.2": s22, "SPDX 3.0": s30}
problems: list[str] = []

# Rule 0: the three documents list the same packages. A license comparison
# over the intersection alone would pass a document that simply omits one.
for left, right in [("CycloneDX", "SPDX 2.2"), ("CycloneDX", "SPDX 3.0")]:
    missing = sorted(set(docs[left]) ^ set(docs[right]))
    for purl in missing[:10]:
        where = left if purl in docs[left] else right
        problems.append(f"{purl}: listed by {where} alone")
    if len(missing) > 10:
        problems.append(f"... and {len(missing) - 10} more packages listed by one document alone")

# Rule 1: a supplemented purl carries its mapped license everywhere.
for purl in set(cdx) | set(s22) | set(s30):
    want = supplement.get(key(purl))
    if not want:
        continue
    for name, doc in docs.items():
        got = [v for v in doc.get(purl, []) if stated(v)]
        if [norm(v) for v in got] != [norm(want)]:
            problems.append(
                f"{purl}: the supplement map says {want} and {name} says "
                f"{', '.join(doc.get(purl, [])) or 'nothing'}")

# Rule 2: where two documents both name a license, it is the same license.
pairs = [("CycloneDX", "SPDX 2.2"), ("CycloneDX", "SPDX 3.0"), ("SPDX 2.2", "SPDX 3.0")]
for left, right in pairs:
    a, b = docs[left], docs[right]
    for purl in sorted(set(a) & set(b)):
        av = sorted({norm(v) for v in a[purl] if stated(v)})
        bv = sorted({norm(v) for v in b[purl] if stated(v)})
        if av and bv and av != bv:
            problems.append(
                f"{purl}: {left} says {', '.join(a[purl])} and {right} says {', '.join(b[purl])}")

if problems:
    print("FAIL: the signed SBOMs disagree about a package or its license:")
    for line in problems[:20]:
        print(f"  {line}")
    if len(problems) > 20:
        print(f"  ... and {len(problems) - 20} more")
    raise SystemExit(1)

shared = len(set(cdx) & set(s22) & set(s30))
print(f"OK: three SBOMs name no conflicting license across {shared} shared packages")
PY
