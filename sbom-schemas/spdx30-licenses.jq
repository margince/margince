# The SPDX 3.0 half of `make sbom-supplement`.
#
# syft writes a package's license as two relationships rather than a field, so
# the one-line filters that set CycloneDX's `licenses` and SPDX 2.2's
# `licenseConcluded`/`licenseDeclared` have nothing to assign here. Without
# this the three signed documents disagreed on every supplemented package:
# CycloneDX said BSD-3-Clause for pkg:pypi/ply while SPDX 3.0 said
# LicenseRef-BSD declared and NoAssertion concluded.
#
# A license is an ELEMENT here, named by its SPDX id, and a relationship points
# at it. So this reuses the element the document already carries for that name
# rather than writing a second one: a listed license minted per package is how
# two packages under one license stop comparing equal.
#
# A mapped license the document has no element for is an error rather than a
# silent pass, because a supplement that quietly sets nothing is the state this
# filter exists to end.
$m[0] as $map
| ([."@graph"[] | select(.type == "expandedlicensing_ListedLicense")]
   | map({key: .name, value: .spdxId}) | from_entries) as $listed
| ([."@graph"[] | select(.type == "software_Package")
    | ((.software_packageUrl // "") | sub("@[^@]*$"; "")) as $k
    | select($map[$k] != null)
    | {key: .spdxId, value: $map[$k]}] | from_entries) as $want
| ([$want[] | select($listed[.] == null)] | unique) as $unlisted
| if ($unlisted | length) > 0
  then error("sbom-supplement: no SPDX 3.0 license element for " + ($unlisted | join(", ")))
  else . end
| ."@graph" |= map(
    if .type == "Relationship" and (.relationshipType | test("License"))
       and ($want[.from] != null)
    then .to = [$listed[$want[.from]]]
    else . end)
