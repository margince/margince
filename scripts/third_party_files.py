#!/usr/bin/env python3
# SPDX-License-Identifier: BUSL-1.1
# SPDX-FileCopyrightText: 2026 Gradion

import json
from pathlib import Path, PurePosixPath
import re
import subprocess
import sys
from typing import NamedTuple


class CopiedFile(NamedTuple):
    path: str
    source: str
    license: str
    text: str


def records(manifest):
    found = []
    seen = set()
    for number, line in enumerate(Path(manifest).read_text().splitlines(), 1):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        fields = [field.strip() for field in line.split("|")]
        if len(fields) != 4 or not all(fields):
            raise ValueError(f"{manifest}:{number}: expected four nonempty fields")
        record = CopiedFile(*fields)
        for path in (record.path, record.text):
            if PurePosixPath(path).is_absolute() or ".." in PurePosixPath(path).parts:
                raise ValueError(f"{manifest}:{number}: path must stay inside the repository")
        if record.path in seen:
            raise ValueError(f"{manifest}:{number}: duplicate copied path {record.path}")
        seen.add(record.path)
        found.append(record)
    return found


def grant_list(grant, key):
    entries = []
    inside = False
    for line in grant.read_text().splitlines():
        if line == key + ":":
            inside = True
        elif inside and line and not line[0].isspace() and not line.startswith("#"):
            break
        elif inside and (match := re.match(r"\s+-\s+([^\s#]+)", line)):
            entries.append(match[1])
    return entries


# A copied file answers to copied-file-licenses, never to the shipped `allow`.
# A licence only the copied list holds is accepted for no package, so grant
# must skip that file through ignore-packages.
def check_licences(grant, copied):
    shipped = set(grant_list(grant, "allow"))
    accepted = set(grant_list(grant, "copied-file-licenses"))
    skipped = set(grant_list(grant, "ignore-packages"))
    for record in copied:
        if record.license not in accepted:
            raise ValueError(f"third-party files: {record.path} has disallowed licence {record.license}")
        if record.license not in shipped and record.path not in skipped:
            raise ValueError(f"third-party files: {record.path} is under {record.license}, "
                             "which no shipped package may carry; name it in ignore-packages")


def check(root, manifest):
    root = Path(root)
    copied = records(manifest)
    check_licences(root / ".grant.yaml", copied)
    paths = {record.path for record in copied}
    texts = {record.text for record in copied}
    for record in copied:
        for path in (record.path, record.text):
            if not (root / path).is_file():
                raise ValueError(f"third-party files: missing {path}")
    tracked = subprocess.check_output(["git", "ls-files", "-z"], cwd=root).decode().split("\0")
    legal = [path for path in tracked if path and path != "LICENSE" and
             (re.match(r"^(?:LICEN[CS]E|NOTICE|COPYING)", Path(path).name) or
              re.fullmatch(r"(?:licen[cs]e|notice|copying)(?:\.(?:txt|md|rst))?", Path(path).name, re.I))]
    for license_path in legal:
        if license_path not in texts:
            raise ValueError(f"third-party files: unrecorded licence text {license_path}")
        folder = str(PurePosixPath(license_path).parent) + "/"
        for path in tracked:
            if path.startswith(folder) and path not in paths and path not in texts and Path(path).name != "README.md":
                raise ValueError(f"third-party files: unrecorded copied file {path}")
    print(f"OK: {len(copied)} copied third-party files have provenance and allowed licences")


def documents(directory):
    paths = [Path(directory) / f"margince.{suffix}.json" for suffix in ("cdx", "spdx221", "spdx300")]
    return paths, [json.loads(path.read_text()) for path in paths]


def file_components(docs):
    cdx, s22, s30 = docs
    return [
        {item["name"]: item for item in cdx.get("components", []) if item.get("type") == "file"},
        {item["fileName"]: item for item in s22.get("files", [])},
        {item["name"]: item for item in s30["@graph"] if item.get("type") == "software_File"},
    ]


def license_relationship(graph, file, license_id, kind):
    matches = [item for item in graph if item.get("type") == "Relationship" and
               item.get("from") == file["spdxId"] and item.get("relationshipType") == kind]
    if matches:
        for relation in matches:
            relation["to"] = [license_id]
        return
    relation = {"type": "Relationship", "spdxId": file["spdxId"] + "/" + kind,
                "relationshipType": kind, "from": file["spdxId"], "to": [license_id]}
    if "creationInfo" in file:
        relation["creationInfo"] = file["creationInfo"]
    graph.append(relation)


# File conclusions are path-based; package declarations remain purl-based in Makefile.
# Existing in-file scanner evidence must survive supplementation.
def supplement(directory, manifest):
    paths, docs = documents(directory)
    components = file_components(docs)
    graph = docs[2]["@graph"]
    licenses = {item.get("name"): item["spdxId"] for item in graph
                if item.get("type") == "expandedlicensing_ListedLicense"}
    for record in records(manifest):
        for entries in components:
            if record.path not in entries:
                raise ValueError(f"SBOM omitted copied file {record.path}")
        if record.license not in licenses:
            raise ValueError(f"SPDX 3.0 has no listed licence element for {record.license}")
        components[0][record.path]["licenses"] = [{"license": {"id": record.license}}]
        components[1][record.path]["licenseConcluded"] = record.license
        components[1][record.path]["licenseComments"] = "Concluded from " + record.text + "; source: " + record.source
        for kind in ("hasConcludedLicense", "hasDeclaredLicense"):
            license_relationship(graph, components[2][record.path], licenses[record.license], kind)
    for path, doc in zip(paths, docs):
        path.write_text(json.dumps(doc, indent=2) + "\n")
    print("OK: copied-file licences supplemented in all three SBOM formats")


def parity(directory, manifest):
    _, docs = documents(directory)
    cdx, s22, s30 = file_components(docs)
    graph = docs[2]["@graph"]
    licenses = {item["spdxId"]: item.get("name") for item in graph if item.get("spdxId")}
    for record in records(manifest):
        if any(record.path not in entries for entries in (cdx, s22, s30)):
            raise ValueError(f"SBOM omitted copied file {record.path}")
        cdx_licenses = [item.get("license", {}).get("id") for item in cdx[record.path].get("licenses", [])]
        spdx22 = s22[record.path]
        file_id = s30[record.path]["spdxId"]
        spdx30 = {kind: [licenses.get(target) for item in graph
                        if item.get("type") == "Relationship" and item.get("from") == file_id and
                        item.get("relationshipType") == kind for target in item.get("to", [])]
                  for kind in ("hasConcludedLicense", "hasDeclaredLicense")}
        if (cdx_licenses != [record.license] or spdx22.get("licenseConcluded") != record.license or
                any(values != [record.license] for values in spdx30.values())):
            raise ValueError(f"copied-file licence differs across SBOMs: {record.path}")
    print("OK: copied-file licences agree in all three SBOMs")


def main():
    if len(sys.argv) != 4 or sys.argv[1] not in ("check", "supplement", "parity"):
        raise ValueError("usage: third_party_files.py check|supplement|parity ROOT_OR_SBOMS MANIFEST")
    actions = {"check": check, "supplement": supplement, "parity": parity}
    actions[sys.argv[1]](sys.argv[2], sys.argv[3])


if __name__ == "__main__":
    try:
        main()
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        sys.exit(1)
