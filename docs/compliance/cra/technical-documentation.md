# CRA technical documentation

Status: draft conformity dossier for Margince. Source files identify controls; release-specific reports
must prove their results before a declaration is issued. The [requirements matrix](essential-requirements.md)
records the gaps. Gradion Pte. Ltd. owns the assessment and its approval.

Legal baseline checked on 26 November 2026: [Regulation (EU) 2024/2847](https://eur-lex.europa.eu/eli/reg/2024/2847/oj/eng),
OJ L, 20 November 2024, Articles 13, 28, 30–32 and Annexes I–VIII.
The [Official Journal PDF](https://op.europa.eu/o/opportal-service/download-handler?format=PDF&identifier=21b7d4eb-a6e2-11ef-85f0-01aa75ed71a1&language=en&productionSystem=cellar)
is the source text. This dossier claims no harmonised standard, common specification or cybersecurity certificate.

## Product and assessment route

Margince is CRM software for customer records, sales, delivery, communication capture and governed AI assistance.
The intended deployment is on an operator's servers, using a browser, the API and optional MCP clients.
The [architecture](../../explanation/architecture.md) defines the component and trust boundaries.
The [operator security guide](../../how-to/run-margince-securely.md) supplies Annex II information.
Hardware photographs under Annex VII 1(c) do not apply to this software product.

The assessment route is internal control, Module A under Article 32(1)(a) and Annex VIII Part I.
This is the manufacturer's classification assessment: CRM is the core function, outside Annexes III and IV.
Sign-in, access controls and OAuth protect access to Margince; they do not make its core purpose identity
management, privileged access management or a security monitoring system.
Review that conclusion against the technical descriptions in
[Implementing Regulation (EU) 2025/2392](https://eur-lex.europa.eu/eli/reg_impl/2025/2392/oj/eng)
whenever the product's intended purpose or composition changes. An extension or separately marketed security
function can require a different assessment. No notified body or certificate is claimed here.

Public BUSL-licensed source does not by itself establish an exemption for free and open-source software.
Articles 3(21)–(22) use the first supply for commercial distribution or use in the Union, including free supply.
A git tag identifies source; it does not prove the date or circumstances of that supply.
Article 69(2) governs substantial modifications to products placed before 11 December 2027.
Article 14 reporting also applies to those products under Article 69(3).

## Release identity and retained record

Complete this record for each assessed product version. Keep the release's source snapshot with it;
links to a moving branch cannot preserve the evidence.

| Record | Required evidence | State |
| --- | --- | --- |
| Product identity | Release tag, full commit, enabled extensions and dependency lockfiles | Release assessment pending |
| Distributed bytes | Image digests by role and platform, bundle checksums and verified signatures | Release assessment pending |
| Market placement | First Union supply date, distribution route and commercial basis, without customer identities | Manufacturer record pending |
| Modification assessment | Changed purpose, interfaces, data flows and risks; new assessment or reason for reusing evidence | Release assessment pending |
| Support | Expected-use assessment, period, end date and free upgrade compatibility | [Support policy](support-period.md); release record pending |
| Approval | Assessor, reviewer, date, risk acceptance and signed declaration | Not approved |

Article 13(13) requires the technical documentation and declaration for at least ten years after market placement,
or the support period if longer. Article 13(18) requires the user instructions for that duration too.
Keep versioned source documents in this directory's git history. The manufacturer must also keep an independent,
access-controlled release archive of signed declarations, instructions, risk decisions, SBOMs and test reports.
An archive owner, retrieval check and protected retention are tracked in
[#13592](https://github.com/margince/margince/issues/13592); git history and expiring CI artifacts alone do not discharge that duty.
Security-update availability has its own clock in [support-period.md](support-period.md).

## Design, production and vulnerability handling

Annex VII 2(a) is described by the [architecture](../../explanation/architecture.md),
[authorization](../../explanation/authorization.md), [write backbone](../../explanation/write-backbone.md)
and [extension boundaries](../../explanation/extensibility.md).
The [API contract](../../../backend/api/crm.yaml) defines the public HTTP interface.
The assessment includes the composed extension set, connected inputs and optional AI data flows.

For Annex VII 2(b), [SECURITY.md](../../../SECURITY.md) defines intake, coordinated disclosure,
component reporting and remediation. The [Article 14 runbook](README.md) owns statutory reporting.
[Supply-chain controls](../../reference/supply-chain.md) describe dependency inventories and signature generation.

For Annex VII 2(c), [CI](../../../.github/workflows/ci.yml) tests changes and
[the tagged release workflow](../../../.github/workflows/release-tag.yml) builds and signs assets.
Its [SBOM workflow](../../../.github/workflows/_release-sbom.yml) scans the same source revision.
The [operating guide](../../operations/observability.md) covers health and diagnostic signals.
Validation still needs the actual release's build, verification, monitoring and vulnerability-handling review reports.

## Cybersecurity risk assessment

This is the initial risk register under Article 13(2) and Annex VII 3. All listed risks require review for each
assessed composition. Residual risk is unaccepted until the reviewer records a release-specific result.
The matrix assesses the applicability of each Annex I requirement; an open applicability decision is not an exemption.

| Foreseeable event | Consequence | Controls and residual assessment |
| --- | --- | --- |
| Stolen credentials or excessive rights | CRM data disclosure or unauthorised changes | [Authorization](../../explanation/authorization.md); test sign-in, revocation and row scope |
| Hostile mail, files, URLs or model instructions | Execution, network access or agent actions beyond authority | [Capture boundaries](../../explanation/ingress-gate-and-auto-capture.md), [agent controls](../../explanation/agent-surface.md); review enabled connectors and egress |
| Exposed database, root key or unencrypted storage | Disclosure of records and connector credentials | [Secure setup](../../how-to/run-margince-securely.md); operator must evidence network isolation, encryption and key custody |
| Altered build, dependency or update | Code or data compromise | [Supply chain](../../reference/supply-chain.md); verify final artifacts and review vulnerability scan findings |
| Exhaustion or failed recovery | Loss of availability or data | [Restore drill](../../how-to/run-a-restore-drill.md); verify limits and recovery on demo data |
| Incomplete erasure or unsafe transfer | Personal data remains or is disclosed | [Privacy controls](../../handbook/retention-exports-and-deletion.md), operator decommissioning; prove removal of backups and settings too |
| Unsupported release or unsafe upgrade | Known vulnerabilities remain exposed | [Support policy](support-period.md); verify notices, free upgrade path and retained updates |

## Standards, test reports and declaration

The [requirements matrix](essential-requirements.md) records the alternative technical solutions for Annex VII 5.
SBOM interchange formats and signature tools are evidence mechanisms; their use is not a CRA presumption of conformity.
If an assessment later relies on a standard, record its exact edition, applied clauses and available source copy.

For Annex VII 6, preserve the assessed commit's `make check` results, applicable integration and security tests,
dependency findings and their dispositions, signature checks, recovery tests and vulnerability-process review.
Record commands, environment, date, outcome and reviewer. Test source or a green result on another commit is insufficient.
No release test report or conformity approval is asserted by this draft.

Annex VII 7 is the [draft declaration](declaration-of-conformity.md). Sign only after the matrix gaps are resolved
and the release record is approved. Review substantial modifications under Article 3(30), Article 13(12) and
Article 13(14). A security fix that only lowers risk normally is not substantial; changed purpose or affected
Annex I compliance requires reassessment. Keep the reasoning with the release record.

## SBOM scope and access

For Annex VII 8 and Annex I Part II(1), the release publishes the source-tree SBOMs described in
[supply-chain.md](../../reference/supply-chain.md), bound to the release by its signed checksum manifest.
The inventory covers source dependencies across the composed tree. The current tagged workflow distributes
role images and Linux server bundles; it has no desktop-bundle lane.

Source-tree inventories do not establish coverage of OS packages added by the [Dockerfile](../../../Dockerfile).
Final-image dependency coverage and the relationship between each shipped artifact and its SBOM remain to be verified.
Keep the SBOM in the conformity archive and supply it on a reasoned market-surveillance request when necessary.
Public release publication makes it available to operators too; it does not replace the coverage check.
