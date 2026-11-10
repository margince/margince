#!/usr/bin/env bash
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
python3 - "$repo_root" "$work" <<'PY'
import json
from pathlib import Path
import subprocess
import sys

source, work = map(Path, sys.argv[1:])
script = source / "scripts/third_party_files.py"
fixture = work / "repo"
fixture.mkdir()
subprocess.run(["git", "init", "-q", str(fixture)], check=True)
grant = fixture / ".grant.yaml"
grant.write_text("allow:\n  - MIT\n  - Apache-2.0\ncopied-file-licenses:\n  - MIT\n")
asset = fixture / "vendor/nested"
asset.mkdir(parents=True)
(asset / "LICENCE.md").write_text("Permission to use this file")
(asset / "mark.svg").write_text("<svg/>")
(asset / "README.md").write_text("provenance")
manifest = fixture / "third-party-files.txt"
record = "vendor/nested/mark.svg | example 1 | MIT | vendor/nested/LICENCE.md\n"
manifest.write_text(record)
(fixture / "license_test.go").write_text("package firstparty")
subprocess.run(["git", "-C", str(fixture), "add", "."], check=True)
count = 0


def run(action, target, expected):
    global count
    result = subprocess.run([sys.executable, str(script), action, str(target), str(manifest)], capture_output=True, text=True)
    if (result.returncode == 0) != expected:
        raise AssertionError(result.stdout + result.stderr)
    count += 1
    return result.stdout + result.stderr


run("check", fixture, True)
(fixture / "lowercase").mkdir()
(fixture / "lowercase/license.txt").write_text("Unrecorded lowercase terms")
subprocess.run(["git", "-C", str(fixture), "add", "."], check=True)
assert "unrecorded licence" in run("check", fixture, False)
(fixture / "lowercase/license.txt").unlink()
subprocess.run(["git", "-C", str(fixture), "add", "-u"], check=True)
manifest.write_text("")
assert "unrecorded licence" in run("check", fixture, False)
manifest.write_text(record)
(asset / "extra.js").write_text("copied code")
subprocess.run(["git", "-C", str(fixture), "add", "."], check=True)
assert "unrecorded copied file" in run("check", fixture, False)
(asset / "extra.js").unlink()
subprocess.run(["git", "-C", str(fixture), "add", "-u"], check=True)
manifest.write_text(record.replace("mark.svg", "missing.svg"))
assert "missing vendor/nested/missing.svg" in run("check", fixture, False)
manifest.write_text(record.replace(" | MIT | ", " | GPL-3.0-only | "))
assert "disallowed licence" in run("check", fixture, False)
# A licence the shipped list allows is still refused for a copied file the
# copied-file list does not name.
manifest.write_text(record.replace(" | MIT | ", " | Apache-2.0 | "))
assert "disallowed licence" in run("check", fixture, False)
# A licence only the copied-file list holds is accepted for no package, so the
# file must be kept out of grant's package judgement by name.
spec_only = "allow:\n  - MIT\ncopied-file-licenses:\n  - MIT\n  - Community-Spec-1.0\n"
grant.write_text(spec_only)
manifest.write_text(record.replace(" | MIT | ", " | Community-Spec-1.0 | "))
assert "no shipped package may carry" in run("check", fixture, False)
grant.write_text(spec_only + "ignore-packages:\n  - vendor/nested/mark.svg # copied, never shipped\n")
run("check", fixture, True)
grant.write_text("allow:\n  - MIT\n  - Apache-2.0\ncopied-file-licenses:\n  - MIT\n")
manifest.write_text(record + record)
assert "duplicate copied path" in run("check", fixture, False)
manifest.write_text(record.replace("vendor/nested/mark.svg", "../outside.svg"))
assert "inside the repository" in run("check", fixture, False)
manifest.write_text(record)
(asset / "LICENCE.md").unlink()
assert "missing vendor/nested/LICENCE.md" in run("check", fixture, False)
(asset / "LICENCE.md").write_text("Permission to use this file")
run("check", fixture, True)

sboms = work / "sboms"
sboms.mkdir()
path = "vendor/nested/mark.svg"
cdx = sboms / "margince.cdx.json"
s22 = sboms / "margince.spdx221.json"
s30 = sboms / "margince.spdx300.json"
cdx.write_text(json.dumps({"components": [{"type": "file", "name": path}]}))
s22.write_text(json.dumps({"files": [{"fileName": path, "licenseConcluded": "NOASSERTION", "licenseInfoInFiles": ["NOASSERTION"]}]}))
s30.write_text(json.dumps({"@graph": [
    {"type": "software_File", "name": path, "spdxId": "urn:file:mark"},
    {"type": "expandedlicensing_ListedLicense", "name": "MIT", "spdxId": "urn:license:mit"},
]}))
run("parity", sboms, False)
run("supplement", sboms, True)
run("parity", sboms, True)
assert json.loads(cdx.read_text())["components"][0]["licenses"] == [{"license": {"id": "MIT"}}]
assert json.loads(s22.read_text())["files"][0]["licenseInfoInFiles"] == ["NOASSERTION"]
first = [item.read_bytes() for item in (cdx, s22, s30)]
run("supplement", sboms, True)
assert first == [item.read_bytes() for item in (cdx, s22, s30)]
doc = json.loads(s30.read_text())
doc["@graph"] = [item for item in doc["@graph"] if item.get("relationshipType") != "hasConcludedLicense"]
s30.write_text(json.dumps(doc))
assert "differs across SBOMs" in run("parity", sboms, False)
# A copied-file licence no package carries gets one shared element.
doc = json.loads(s30.read_text())
doc["@graph"] = [item for item in doc["@graph"] if item.get("type") != "expandedlicensing_ListedLicense"]
s30.write_text(json.dumps(doc))
run("supplement", sboms, True)
run("parity", sboms, True)
minted = [item for item in json.loads(s30.read_text())["@graph"] if item.get("type") == "expandedlicensing_ListedLicense"]
assert [item["name"] for item in minted] == ["MIT"], minted
run("supplement", sboms, True)
assert len([item for item in json.loads(s30.read_text())["@graph"] if item.get("type") == "expandedlicensing_ListedLicense"]) == 1
cdx.write_text(json.dumps({"components": []}))
assert "omitted copied file" in run("supplement", sboms, False)
print(f"OK: third-party provenance and SBOM licence checks pass {count} cases")
PY
