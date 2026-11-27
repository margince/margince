# CRA essential requirements and evidence

Margince has source evidence for security controls, with release qualification still open.
The declaration stays a draft until the applicable requirements have evidence for the identified release.
The [technical documentation](technical-documentation.md) owns that release record and its risk assessment.

Legal baseline: [Regulation (EU) 2024/2847, Annex I](https://eur-lex.europa.eu/eli/reg/2024/2847/oj/eng),
English Official Journal text of 20 November 2024, checked on 26 November 2026.
The [Publications Office copy](https://op.europa.eu/o/opportal-service/download-handler?format=PDF&identifier=21b7d4eb-a6e2-11ef-85f0-01aa75ed71a1&language=en&productionSystem=cellar)
places Annex I on pages 68–69. Requirement labels below follow that text.

## Reading the evidence

**Source** means an implementation or policy exists for the stated part of the requirement.
**Open** means the row has a missing decision, control or release report.
Every row still needs release qualification; a test source does not show that the test passed on a release.
No row claims a presumption of conformity from a harmonised standard.

The risk assessment must decide the applicability of each Part I point 2 requirement.
Where it excludes a requirement, it must record the reason and any other risk treatment.
Until that decision is recorded, a conditional requirement stays open.

## Part I product security

| Annex I | Requirement | Source evidence | Status and remaining evidence |
| --- | --- | --- | --- |
| I.1 | Security proportionate to risk | [Architecture](../../explanation/architecture.md), [authorisation](../../explanation/authorization.md) and the [risk assessment](technical-documentation.md) describe the product's boundaries. | Open: release-specific risk review and acceptance under [release evidence](#release-evidence). |
| I.2(a) | No known exploitable vulnerabilities | [CI](../../../.github/workflows/ci.yml) declares vulnerability and secret scans. | Open: retain results and findings disposition for the shipped dependencies and artifacts under [release evidence](#release-evidence). A scan configuration cannot establish the absence of exploitable vulnerabilities. |
| I.2(b) | Secure defaults and reset | [HTTP chassis](../../../backend/internal/platform/httpserver/chassis.go) sets browser security headers. [Header tests](../../../backend/internal/platform/httpserver/secureheaders_test.go) check them. [Key-vault construction](../../../backend/internal/platform/keyvault/local.go) rejects malformed keys. | Source for these defaults. Open: qualify the production configuration and the complete reset procedure under [reset and erasure](#reset-and-erasure). |
| I.2(c) | Security updates and user controls | [Tagged releases](../../../.github/workflows/release-tag.yml) distribute versioned artifacts. [Server upgrade instructions](../../how-to/install-on-a-server.md#upgrade-to-a-new-release) describe manual installation. | Open: [update applicability](#updates-and-support), available-update notices and, where applicable, automatic installation with opt-out and postponement. |
| I.2(d) | Access protection and reporting | [Login](../../../backend/internal/modules/identity/login.go) records failed attempts. [RBAC](../../../backend/internal/platform/auth/rbac.go) and [row visibility](../../../backend/internal/platform/auth/rowvisible.go) enforce access. [Integration tests](../../../backend/internal/compose/integration/authz_integration_test.go) check denials. | Source. Open: release tests and evidence that an operator can detect refused access under [release evidence](#release-evidence). |
| I.2(e) | Confidentiality | [Key vault](../../../backend/internal/platform/keyvault/local.go) encrypts stored secrets with authenticated encryption. [Cryptographic tests](../../../backend/internal/platform/keyvault/local_internal_test.go) check key and reference mismatch refusal. [SMTP submission](../../../backend/internal/platform/mailer/submission.go) requires TLS outside loopback. | Source for these paths. Open: verify TLS and storage protection across databases, objects, backups and external connections under [release evidence](#release-evidence). Secret encryption does not cover every stored record. |
| I.2(f) | Integrity and corruption reporting | [Authenticated secret storage](../../../backend/internal/platform/keyvault/local.go) detects altered ciphertext. [Audit tests](../../../backend/internal/compose/integration/authz_integration_test.go) check mutation attribution. | Source. Open: release evidence for artifact verification, corrupted inputs and operator-visible failure reporting under [release evidence](#release-evidence). |
| I.2(g) | Data minimisation | [Retention engine](../../../backend/internal/modules/privacy/retention.go) applies declared policies. [Erasure integration test](../../../backend/internal/compose/integration/erasure_audit_payloads_integration_test.go) checks copies in audit and outbox records. | Source. Open: assess captured fields, configured retention and external processing against intended use under [release evidence](#release-evidence). |
| I.2(h) | Availability and incident recovery | [HTTP body limits](../../../backend/internal/platform/httpserver/chassis.go), [server deadlines](../../../backend/cmd/api/serve.go) and [shared limiter tests](../../../backend/internal/platform/ratelimit/shared_integration_test.go) address bounded work. [Restore procedure](../../how-to/run-a-restore-drill.md) covers recovery. | Source. Open: keep abuse, dependency-outage and full restore results under [release evidence](#release-evidence). Recording a drill is separate from performing a successful restore. |
| I.2(i) | Limit harm to connected services | [Web pacing](../../../backend/internal/platform/webread/pacer.go) limits requests. [Shared rate-limit registry](../../../backend/internal/platform/ratelimit/registry.go) supports limits across replicas. | Source. Open: qualify retry, cancellation and provider-outage behaviour under [release evidence](#release-evidence). |
| I.2(j) | Limit exposed interfaces | [Architecture](../../explanation/architecture.md), [API contract](../../../backend/api/crm.yaml) and [operator endpoints](../../reference/operator-endpoints.md) identify the interfaces to assess. | Open: reconcile the shipped interface inventory with network exposure and the intended use under [release evidence](#release-evidence). |
| I.2(k) | Limit exploitation impact | [Browser headers](../../../backend/internal/platform/httpserver/chassis.go) restrict scripts and framing. [RBAC](../../../backend/internal/platform/auth/rbac.go) limits actions. [Authorisation tests](../../../backend/internal/compose/integration/authz_integration_test.go) exercise denied access. | Source. Open: assess process privileges and isolation of the final artifacts under [release evidence](#release-evidence). |
| I.2(l) | Security activity records and opt-out | [Request observation](../../../backend/internal/platform/httpserver/observe.go) and [audit reader tests](../../../backend/internal/compose/integration/auditlog_integration_test.go) cover recorded activity. | Source for recording. Open: [logging choice](#logging-choice) and evidence that the selected monitoring controls cover relevant access and modifications. |
| I.2(m) | Permanent removal and secure transfer | [Eraser](../../../backend/internal/modules/privacy/eraser.go) supplies the shared subject-erasure path. [Erasure integration test](../../../backend/internal/compose/integration/erasure_audit_payloads_integration_test.go) checks historical payloads. | Source for subject erasure. Open: [reset and erasure](#reset-and-erasure) for all product data, settings, copies and secure transfer. |

## Part II vulnerability handling

| Annex I | Requirement | Source evidence | Status and remaining evidence |
| --- | --- | --- | --- |
| II.1 | Components, vulnerabilities and SBOM | [Release SBOM workflow](../../../.github/workflows/_release-sbom.yml) generates machine-readable source-tree inventories. [Lockfile census](../../../scripts/sbom-census.sh) checks npm coverage. | Source. Open: [release evidence](#release-evidence) must reconcile the inventory with every shipped artifact, including runtime and operating-system dependencies. |
| II.2 | Prompt fixes and separable updates | [Security policy](../../../SECURITY.md) defines handling, and [support policy](support-period.md) states the latest-release rule. | Policy source. Open: document whether security-only releases are technically feasible and retain remediation records under [updates and support](#updates-and-support). |
| II.3 | Regular security tests and reviews | [CI](../../../.github/workflows/ci.yml) declares security scans and test lanes. [RBAC integration tests](../../../backend/internal/compose/integration/authz_integration_test.go) exercise access controls. | Source. Open: retain actual reports and review findings for the release under [release evidence](#release-evidence). |
| II.4 | Publish fixed-vulnerability information | [Security policy](../../../SECURITY.md) uses public GitHub Security Advisories for disclosure. | Policy source. Open: release records must include affected versions, severity, impact and remediation, with a reason for any delayed publication under [release evidence](#release-evidence). |
| II.5 | Coordinated disclosure policy | [Security policy](../../../SECURITY.md) defines private reporting and coordinated publication. | Policy source. Open: retain evidence that reports follow the policy under [release evidence](#release-evidence). |
| II.6 | Reporting contact and component sharing | [Security policy](../../../SECURITY.md) provides a human contact and GitHub advisory route. | Policy source. Open: keep component-owner notifications and follow-through under [release evidence](#release-evidence). |
| II.7 | Secure and timely update distribution | [Release workflow](../../../.github/workflows/release-tag.yml) creates checksums and a separate signing job. [Server upgrade instructions](../../how-to/install-on-a-server.md#upgrade-to-a-new-release) describe installation. | Source. Open: verify the delivered update end to end and record [automatic-update applicability](#updates-and-support). |
| II.8 | Free prompt updates with advice | [Support policy](support-period.md) commits to free access to the latest release. [Security policy](../../../SECURITY.md) defines notices. | Policy source. Open: verify upgrade access and retain publication and notification records under [updates and support](#updates-and-support). |

## Release evidence

Tracked in [#13592](https://github.com/margince/margince/issues/13592).

The release owner must attach the evidence to the release's technical documentation.
It covers risk acceptance, scan and test reports, artifact identities, dependency inventories, signatures and policy execution.
Reports must identify the revision, artifact digests, test environment, outcome and unresolved findings.
Access-control or cryptographic test sources alone do not qualify the finished product.

Article 14 reporting setup is tracked separately in [#11227](https://github.com/margince/margince/issues/11227).
It stays open independently of the conformity assessment.

## Updates and support

Applicability and expected use: [#13593](https://github.com/margince/margince/issues/13593).
Release mechanisms, notices and retention: [#13592](https://github.com/margince/margince/issues/13592).

The [support policy](support-period.md) governs fixes, free upgrades and the declared period.
The risk assessment must settle automatic-update applicability and the separation of security and feature updates.
Release qualification must also cover support dates, older-release warnings and long-term access to updates and documents.

## Logging choice

Decision owner: Lars, in [#13593](https://github.com/margince/margince/issues/13593).

The manufacturer must decide how the user opt-out in I.2(l) applies to security monitoring.
That decision must account for mandatory domain audit records and name the monitoring controls it governs.
An access log and a mutation audit trail do not by themselves establish an opt-out mechanism.

## Reset and erasure

Release qualification is tracked in [#13592](https://github.com/margince/margince/issues/13592).

Qualify secure reset, final removal and data transfer on disposable demo data through the operator procedure.
Include database records, settings, object versions, exports, logs, replicas, backups and external credentials.
A subject-erasure test covers a smaller scope than permanent removal of the whole product.
