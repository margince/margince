<!-- prose:plain -->
# Pinned inputs for SBOM checks

`make sbom-validate` checks each generated SBOM against its format. The validators, and the files in
this folder:

| Format | Validator | Asset here |
| --- | --- | --- |
| CycloneDX | first-party `cyclonedx-cli` (bundles its spec schema) | none |
| SPDX 2.2.1 | `pyspdxtools` (spdx/tools-python), structural **and** semantic | `spdx-tools-requirements.txt` |
| SPDX 3.0.1 | generic JSON-schema validation | `spdx-3.0.1.schema.json` |

`license-supplement.json` is the curated map from purl to SPDX license that `make sbom-supplement`
applies. It covers packages syft cannot find a license for by itself: GitHub Actions, and PyPI
packages whose metadata says only "BSD". Keys are purls without a version, so every pinned action
version matches.

`pyspdxtools` is the standard validator for SPDX 2. It runs in a Python image pinned by digest, with
the set of packages pinned by hash in `spdx-tools-requirements.txt`. So it is as repeatable as the
SBOM images pinned by digest. The steps to build that file again are in its header. It fails on a
document that is not valid, so the `Makefile` recipe gates on that, and shows its report on failure.

## Why SPDX 3.0 is checked by JSON schema only

The semantic layer of SPDX 3.0 is SHACL, and the SPDX project points at
[`spdx3-validate`](https://github.com/JPEWdev/spdx3-validate) /
[`pyshacl`](https://github.com/spdx/spdx-3-model/blob/develop/serialization/jsonld/validation.md)
for it. Neither can serve as a gate here. Run against the SPDX 3.0 output of syft 1.50,
`spdx3-validate` reports about 1100 `sh:ClassConstraintComponent` violations. Every `Element`
reference fails the class check, because the model loads without the OWL inference that makes
`software_File` a subclass of `Element`. It also takes about 4 minutes, and fetches the model from
`spdx.org` on every run.

A gate on it would fail `make sbom` on a document we do not own (the syft writer) and cannot patch
(contract-first). So SPDX 3.0 gets a structural check by JSON schema, the layer the SPDX 3 model doc
calls the structural check. It passes on the output of syft, and it is offline and fast. Look at it
again when both the SPDX 3.0 writer of syft and the SHACL tools are mature.

## The copied schema

`spdx-3.0.1.schema.json` is copied into the tree, not fetched at build time, for the same reason the
validator images are pinned by digest. A host for the schema that moves, or a payload someone changed,
must not change what the gate accepts. To update it, replace the file from its source and note the
change in the commit.

| File | Source |
| --- | --- |
| `spdx-3.0.1.schema.json` | https://spdx.org/schema/3.0.1/spdx-json-schema.json |

These are inputs for the checks, not shipped product. Still, `.syft.yaml` includes this committed
folder, so the SBOM covers the whole committed tree.
