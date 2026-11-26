# Run Margince securely

Run Margince behind HTTPS on infrastructure you control. Its API, worker, browser app and enabled
extensions handle customer records, messages and credentials. The operator controls host security,
database access, storage encryption, backups and the external services it connects.

These instructions address Annex II of [Regulation (EU) 2024/2847](https://eur-lex.europa.eu/eli/reg/2024/2847/oj/eng),
OJ L, 20 November 2024, checked on 26 November 2026.
The [conformity declaration](../compliance/cra/declaration-of-conformity.md) is an unsigned draft.

## Identify the product and manufacturer

The manufacturer is Gradion Pte. Ltd., 77 High Street, #09-11 High Street Plaza, Singapore 179433.
The public address is in [Gradion's imprint](https://gradion.com/en/imprint).
Use [security@gradion.com](mailto:security@gradion.com) as the single security contact, or use a
[private GitHub advisory](https://github.com/margince/margince/security/advisories/new).
[SECURITY.md](../../SECURITY.md) states how to report, receive updates and coordinate disclosure.

Record the release and source revision before requesting help. Server bundles carry `VERSION` and
`REVISION`; the programs answer `--version`. Container releases identify their role images by digest.
The [server installation guide](install-on-a-server.md) and [deployment guide](deploy-margince.md)
show how to verify those identities and signatures.

## Set up and operate

1. Check [system requirements](../operations/system-requirements.md), then follow the
   [container deployment](deploy-margince.md) or [server installation](install-on-a-server.md) instructions.
   Keep the database, Redis and object storage off the public network. Use HTTPS at the reverse proxy.
2. Use the deployment guide's separate database roles, fresh bootstrap password, protected root key
   and proxy settings. Keep development mode off. Encrypt data volumes and backups through your infrastructure.
3. Give each person only the access they need. Review service accounts, agent passports and connectors.
   A compromised administrator, database owner or host can expose the whole installation.
4. Read the [AI data-egress reference](../reference/ai-egress.md) before enabling model providers.
   Untrusted messages, files and websites can contain hostile instructions. Keep approvals and access limits in place.
5. Set up [monitoring](../operations/observability.md), [backups](back-up-an-installation.md)
   and [restore drills](run-a-restore-drill.md). Keep the root key apart from the data it protects.

A change to extensions, permissions, proxies, network access or model providers changes the security boundary.
Review it against the [extension contract](../explanation/extensibility.md),
[API contract](../../backend/api/crm.yaml) and the guides above.
Integrators must assess their complete product and retain its configuration, dependency and test evidence.
The [technical dossier](../compliance/cra/technical-documentation.md) gives the component and risk references.

## Receive and apply security updates

Subscribe to this repository's [security advisories](https://github.com/margince/margince/security/advisories)
and [releases](https://github.com/margince/margince/releases). Read the advisory's affected versions,
mitigation and fixed version. Contact the security address if an upgrade is blocked.

Security fixes ship in the latest release. Upgrades are free, subject to the no-added-cost conditions in
[the support policy](../compliance/cra/support-period.md). Each assessed version declares at least five years;
release-specific support end dates are still an open item there. Older releases receive no backported fixes
and can contain vulnerabilities fixed in the latest release. A general support contract is separate from security updates.

Verify the release's signatures and checksums, back up, and use the documented
[container rollout](deploy-margince.md#roll-out-in-this-order) or
[server upgrade](install-on-a-server.md#upgrade-to-a-new-release). Check readiness before resuming work.
Database migrations run on API start; restoring only old binaries does not undo a migration.

The supplied server units and Compose deployment require operator-driven upgrades.
Margince supplies no automatic security-update setting or postpone control to turn off.
Automatic-update applicability under Annex I remains an open conformity decision in the
[requirements matrix](../compliance/cra/essential-requirements.md).
Retrieve the release's machine-readable SBOM from its release assets; formats and verification are in
[supply-chain.md](../reference/supply-chain.md).

## Decommission and remove data

1. Export required records through the [documented export routes](../handbook/retention-exports-and-deletion.md).
   Encrypt the handover and restrict access. Apply legal holds and retention duties before destroying copies.
2. Stop the API and worker. Revoke connector credentials, sessions, passports, API credentials and provider grants
   through their issuing systems. Remove DNS and proxy routes so the retired software cannot be reached.
3. Use your infrastructure's secure deletion procedure for the database, object store, Redis, files, logs,
   configuration and root keys. Include replicas, snapshots, backups and exported copies when their retention permits.
   Check storage-provider retention and key-destruction rules; deleting a container leaves persistent storage behind.
4. Record what was removed, which copies remain under retention, who controls them and when they must be destroyed.
   Verify that no process, credential or recoverable data copy remains outside that record.

Archiving a record does not destroy it. Contact erasure also leaves specified material for review;
the [privacy guide](../handbook/retention-exports-and-deletion.md) states those limits.
Margince has no single whole-installation reset or secure-wipe command. The release assessment must verify
the complete removal procedure on demo data, including settings and retained copies.
