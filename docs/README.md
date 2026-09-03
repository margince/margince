# Margince documentation

**Using the product rather than changing it?** The end-to-end walkthroughs for a
rep or a delivery lead live in [`user-guide/`](../user-guide/README.md). This tree
is mostly for the person changing the code — with one deliberate exception:
`how-to/` carries product how-tos (working your pipeline, setting up projects,
partner programs) beside the engineering ones, because a how-to is a thing you
return to for one answer whichever audience you are in.

Documentation for building and operating **Margince** — a governed, multi-tenant CRM (a Go `/v1` API
backend; the Vite/React web UI ships separately). The docs follow the [Diátaxis](https://diataxis.fr/) split: **tutorials** to learn,
**how-to** guides for tasks, **reference** for lookup, **explanation** for the *why*, plus
**[principles](principles/README.md)** — the handful of statements about this codebase's shape that
settle a class of arguments before they start.

**Designing anything a person can see?** [`DESIGN.md`](../DESIGN.md) at the
repository root is the visual language: the look every new surface is designed
against, and the research behind it. The plan for landing it, one PR per
step, is [how-to/adopt-the-design.md](how-to/adopt-the-design.md) with its
two continuations for the record pages and the remaining surfaces. The
component catalog stays in
[`frontend/src/design-system/README.md`](../frontend/src/design-system/README.md).

**New to the backend? Start with [tutorials/getting-started.md](tutorials/getting-started.md), then
[explanation/backend-onboarding.md](explanation/backend-onboarding.md)** — the orientation hub that
maps the codebase and links everything below.

## Map

### Handbook — how to USE the product

The only tree here written for the person using Margince rather than building it:
no code, no API, just the app.

- [handbook/README.md](handbook/README.md) — eleven pages covering records, the
  pipeline, capture, what the AI does and does not do, approvals, documents,
  retention, seats and settings.

### Principles — how this codebase decides things

- [principles/README.md](principles/README.md) — the index. Each page carries the statement, the method for checking the tree still holds it, and what it explicitly does not ask for.
- [one-source-of-truth.md](principles/one-source-of-truth.md) — one place decides each topic, and module boundaries decide where that place may live. Carries the six-probe duplication scan.
- [the-record-is-the-code.md](principles/the-record-is-the-code.md) — what outranks what when two sources disagree.
- [every-mutation-leaves-a-trace.md](principles/every-mutation-leaves-a-trace.md) — domain row, audit row and event commit together.
- [legibility-is-the-product.md](principles/legibility-is-the-product.md) — why the craft bar is a gate rather than taste.
- [derive-the-obligation.md](principles/derive-the-obligation.md) — how to write a fitness function that actually holds.
- [nothing-here-is-private.md](principles/nothing-here-is-private.md) — the public reader, and why a working exploit takes the private path.

### Tutorials — learn by doing
- [getting-started.md](tutorials/getting-started.md) — clone → running instance with a bootstrapped workspace.

### How-to — accomplish a task
- [add-an-endpoint.md](how-to/add-an-endpoint.md) — add or change an API operation (contract → gen → handler).
- [add-a-module.md](how-to/add-a-module.md) — add a new capability (module) or a cross-module edge, wired into compose.
- [add-a-job.md](how-to/add-a-job.md) — declare a background job kind in the job contract, then write and register its worker.
- [add-an-rbac-object.md](how-to/add-an-rbac-object.md) — add a new RBAC object across the policy, the contract enum, the backfill migration, and the published matrix.
- [create-a-workflow.md](how-to/create-a-workflow.md) — scaffold and wire a new automation starter workflow into the closed catalog.
- [apply-migrations.md](how-to/apply-migrations.md) — write and apply a database migration.
- [mint-a-passport.md](how-to/mint-a-passport.md) — issue an agent passport token.
- [connect-an-mcp-client.md](how-to/connect-an-mcp-client.md) — connect a client to the governed MCP tool surface.
- [run-the-frontend.md](how-to/run-the-frontend.md) — run the SPA in dev.
- [connect-a-mailbox.md](how-to/connect-a-mailbox.md) — connect a mailbox for capture: Gmail OAuth (standing sync + backfill), IMAP app-password, Microsoft Graph OAuth, or Google Calendar — all standing connections.
- [enrich-with-a-local-llm.md](how-to/enrich-with-a-local-llm.md) — point the AI lanes at a local Ollama and enrich a company with no cloud key.
- [read-what-a-company-runs.md](how-to/read-what-a-company-runs.md) — the technical lookup: what DNS, certificate logs and a homepage fingerprint write onto a company, what triggers it, and the one setting that turns it on.
- [check-a-vat-number.md](how-to/check-a-vat-number.md) — ask the EU register whether a company's stated VAT ID is real, read the receipt a tax authority accepts, and the one setting that turns it on.
- [connect-telegram.md](how-to/connect-telegram.md) — bind a workspace-level Telegram bot for pull ingress and governed replies.
- [import-your-linkedin-network.md](how-to/import-your-linkedin-network.md) — import your own `Connections.csv` as graph substrate, and read the reach it buys.
- [import-a-company-spreadsheet.md](how-to/import-a-company-spreadsheet.md) — bring a CSV of companies in: the column mapping, what the preview counts, and how a row names the company it corrects.
- [connect-a-hubspot-overlay.md](how-to/connect-a-hubspot-overlay.md) — connect a workspace to a HubSpot portal in overlay (read + continuous sync) mode.
- [flip-an-overlay-to-native.md](how-to/flip-an-overlay-to-native.md) — the one-way overlay→native cutover: preflight, seal, the typed confirmation, and what recovery actually means.
- [connect-a-cloud-model-provider.md](how-to/connect-a-cloud-model-provider.md) — bind the AI lanes to a BYOK cloud key (Anthropic / OpenAI / Gemini / any OpenAI-compatible vendor).
- [certify-an-ai-model.md](how-to/certify-an-ai-model.md) — certify a model against a task's fixture corpus and benchmark a candidate swap (`make e2e-ai`).
- [add-an-ai-task.md](how-to/add-an-ai-task.md) — add a new AI task or invocation site: declare it in the contract, wire the lane, register the site, certify it.
- [write-a-certification-case.md](how-to/write-a-certification-case.md) — bind a site to the production request builder and validator that certify it: the test-first loop, the case interface, the three site kinds, scenario and rubric authoring, scope.
- [register-a-webhook.md](how-to/register-a-webhook.md) — register an HTTPS endpoint for Standard-Webhooks-signed, retried outbound delivery of contract-generated event payloads (curl or Settings → Integrations), and verify/inspect/replay a delivery.
- [add-an-extension.md](how-to/add-an-extension.md) — ship a stable-tier extension unit (a jurisdiction pack) under `extensions/`, composed and verified.
- [work-your-pipeline.md](how-to/work-your-pipeline.md) — sell with Margince: move a deal, close it (and what winning one requires), stalled deals, saved views, bulk actions, and how to read the pipeline numbers; no code.
- [set-up-a-partner-program.md](how-to/set-up-a-partner-program.md) — the partner reference: what every field and every value means, and how to work the pipeline.
- [set-up-projects.md](how-to/set-up-projects.md) — who can create, edit, archive and share a project; key conventions; when to create one (deal creation vs close-won); the fixed phase and stakeholder vocabularies; no code.
- [run-a-project.md](how-to/run-a-project.md) — the project page section by section, phase moves, and every rule by which an email finds its project — including what filing does to retention; no code.
- [build-the-desktop-app.md](how-to/build-the-desktop-app.md) — build the self-contained folder that runs the whole stack with no Docker, on macOS (`make desktop`) or Windows (`make desktop-win`), then run, configure and update an installation.

### Reference — look it up
- [modules.md](reference/modules.md) — the modules: what each owns, its tables, its HTTP surface.
- [meeting-brief.md](reference/meeting-brief.md) — the pre-meeting brief and its preparation plan: the three invariants, what a caller is and is not shown, how a year of history becomes five moments, and why the plan and the sections have separate writers.
- [agent-tools.md](reference/agent-tools.md) — the governed tool catalog: every registered tool, its tier, the passport scope it spends, egress, and overlay-mode behaviour.
- [mcp-info.md](reference/mcp-info.md) — the served MCP surface exactly as a client receives it, with `mcp-info.json` beside it as the same surface byte for byte. Generated from the running registry, never hand-edited; the generator fails the build when the committed copy and the served surface disagree. The largest page here by an order of magnitude — a lookup table, not something to read through.
- [agent-tool-budget.md](reference/agent-tool-budget.md) — what each agent's tool menu costs in prompt tokens, agent by agent, against the published ceiling. Generated with its `.json` sibling, never hand-edited.
- [ai-certification.md](reference/ai-certification.md) — what the AI certification lane covers: every shipped invocation site, the scenarios it is scored against with a link to each case, and a table per (provider, model, env) binding. Generated from the corpus, the records and the invocation-site census, never hand-edited; a stale record says which scenario moved under it.
- [supply-chain.md](reference/supply-chain.md) — the source-tree SBOMs, the license gate, keyless signing, and the pinned toolchain.

Several reference pages are **generated** and say so in their own first lines —
`mcp-info`, `agent-tool-budget`, `ai-certification`, `rbac-matrix`, `performance-budgets` and the
`perfbench/` records. Do not hand-edit them, and do not try to shorten them: their
length is a function of the surface they tabulate, which is why
`backend/gates/docspagelength_test.go` reads that marker and exempts them from the page
budget rather than keeping its own list of which pages are generated.

- [rbac-matrix.md](reference/rbac-matrix.md) — what each seeded role may do to each kind of record. Generated from the seeded policy, never hand-edited.
- [performance-budgets.md](reference/performance-budgets.md) — every performance budget this product publishes, the last measurement taken for each, and the machine it was taken on. Generated by the `make bench-*` targets, never hand-edited. A RECORD rather than a gate, unlike the matrix above: a latency cannot be checked for drift, so what is held instead is that a budget nobody has measured says so rather than being left out.
- [platform-toolkit.md](reference/platform-toolkit.md) — the reusable `platform/*` + `shared/*` utilities.
- [gate-patterns.md](reference/gate-patterns.md) — the eight shapes a fitness gate comes in, how strong each can be, and how each one silently passes. Read it before writing a gate: [principles/derive-the-obligation.md](principles/derive-the-obligation.md) is the method, this is the menu.
- [gate-inventory.md](reference/gate-inventory.md) — every gate in `backend/`, grouped by shape, with what it holds. Generated from the `//gate:kind` line each gate declares in its own file, never hand-edited.
- [configuration.md](reference/configuration.md) — every binary flag and environment variable.
- [openrouter.md](reference/openrouter.md) — the broker's upstream selection: why a model id served by 21 hosts makes latency and answer quality a per-request lottery, the `routing:` default this product ships against that (reliability over price, the inverse of the broker's own), which preferences are hard filters and which only reorder, and the 2026-09-02 measurements behind each choice — including the one that was 17× faster and would have cost a fifth of the certification score.
- [make-targets.md](reference/make-targets.md) — every `make` target.
- [license-release-rule.md](reference/license-release-rule.md) — the BUSL Change-Date release-stamping rule. (The per-file SPDX license *header* rule is described in [backend-onboarding.md](explanation/backend-onboarding.md) and `AGENTS.md`.)

### Explanation — understand the why

**Start here — the shape of the system**

- [backend-onboarding.md](explanation/backend-onboarding.md) — **the contributor hub**: system overview, the map, what's generated vs hand-written, the store shape, the gates.
- [architecture.md](explanation/architecture.md) — the module DAG, the spine shapes, tenancy-as-structure.
- [contract-first.md](explanation/contract-first.md) — how code is generated from `crm.yaml`.
- [authorization.md](explanation/authorization.md) — why the auth check lives at the store entry point; the structural backstop; what a passport is.
- [rbac-roles-and-teams.md](explanation/rbac-roles-and-teams.md) — the role matrix, row scope (own/team/all), teams, role assignment, and per-record sharing — the data model the auth gate reads.

**The platform spine — how a change is written**

- [write-backbone.md](explanation/write-backbone.md) — storekit, `audit_log`, the outbox, and who consumes the events.
- [composition-layer.md](explanation/composition-layer.md) — how `internal/compose/` boots and where every cross-module edge is wired.
- [job-fleet.md](explanation/job-fleet.md) — the job contract: declaration before code, dispatchers vs workspace workers, why args name rows, and the failure vocabulary.
- [custom-fields.md](explanation/custom-fields.md) — the one runtime `ALTER TABLE` chokepoint: the closed type/object sets, the privilege boundary, and the `fieldcatalog` seam.

**Capture, messaging and privacy**

- [capture-connectors.md](explanation/capture-connectors.md) — the governed **ingress** surface: the connector seam (Gmail / IMAP / Graph / Calendar / Telegram), the one Sink that owns every write, the grant-time scope gate, the ingestion modes (bounded backfill, continuous sync, Gmail push, Telegram long poll), the OAuth connect/callback flow, vault-sealed credentials, and the connect UI.
- [ingress-gate-and-auto-capture.md](explanation/ingress-gate-and-auto-capture.md) — what happens to a message after a connector fetches it, the same for every source: the ingress gate's checks and its two results, the single capture transaction (internal-only check, raw payload, activity write, tier ladder T0–T4), and the verdict engine that decides whether an unknown sender becomes a contact. Says which steps are plain code and which use AI (with the task name), what a dropped message does and does not store, which limits are configurable, and what the per-member **Capture activity** tab shows.
- [mail-history-import.md](explanation/mail-history-import.md) — the bounded backward scan a fresh mailbox is offered: the scope count that reads ids and no bodies, the consent estimate (measured units × measured per-unit cost, `observed` vs `heuristic`, and why an unpriceable estimate hides rather than shows `$0`), the resumable page loop and its failure ladder, the yields a run measures about itself, and the sweeps that spend after the progress bar fills.
- [outbound-messaging.md](explanation/outbound-messaging.md) — the egress twin of capture: the staging row, the transmit-time gates, receipt-before-bookkeeping, and the channel reply.
- [outbound-webhooks.md](explanation/outbound-webhooks.md) — the governed egress surface: subscription config vs. delivery engine, secret sealing, the contract-first payload pipeline (`api/public-events.yaml` + `gen-payloads` + the typed `EmitEvent` seam) and its additive-only versioning, the retry/dead-letter state machine, the owner-scope fan-out gate (incl. the ratified deferred-delivery exceptions), and the Settings → Integrations UI.
- [privacy-and-consent.md](explanation/privacy-and-consent.md) — the consent gate and the GDPR engines (erasure / SAR / retention).

**AI, retrieval and automation**

- [ai-runtime.md](explanation/ai-runtime.md) — the AI task contract, tiers/ladders, the routing config, the one Router gate, honest tracing, and certification.
- [agent-surface.md](explanation/agent-surface.md) — the Surface-B reasoning loop and the model runtime.
- [ai-activity-rail.md](explanation/ai-activity-rail.md) — what the AI is doing for you while it does it: the one `ai_task_run` projection, who reports into it (router vs. carrier vs. step), how an occurrence is attributed to a person, the read's one-statement/two-arm shape and its derived `stalled`, and the separate question of which of the 23 kinds a reader is actually shown — with the written reason for each of the 17 that are not.
- [search-and-retrieval.md](explanation/search-and-retrieval.md) — the lexical and hybrid lanes, row scope inside the query, embedding identity, and the two kinds of staleness with their two different answers.
- [relationship-graph.md](explanation/relationship-graph.md) — who on our team knows this contact: participants, the interaction projection, warmth, deal coverage and its risk rules.
- [company-context.md](explanation/company-context.md) — the cold start, the governed company profile (profile fields, facts, site reads), and how bounded company context reaches AI tasks. This is the *installation's own* company; for the company **record** page see below.
- [automation.md](explanation/automation.md) — the closed 7×7 trigger/action catalog: the two vocabularies, the one firing path, the anchor occurrence key, and both permission gates.

**The product surface**

- [frontend-architecture.md](explanation/frontend-architecture.md) — the SPA's layers, the shell and its nav rules, the colour and theme contract, the evidence mark, and the gates that fail a frontend push.
- [company-record-page.md](explanation/company-record-page.md) — the company record page: one gated 360 read, the work-in-flight card, Ask, record-derived suggestions, the visit baseline, and why view state carries no audit row.

**Modes and extension**

- [overlay-augmentation.md](explanation/overlay-augmentation.md) — the two SoR modes, the frozen seam + inner incumbent seam, the mirror-as-cache, fail-closed visibility, and teardown for the HubSpot overlay (branch 1: read + continuous sync).
- [extensibility.md](explanation/extensibility.md) — the stable extension tier: the inert compile-time declaration, the marker-allowlisted surface, the composition build, the `GOWORK` binding that decides which composition module the compiler links, boot reconciliation, and the fitness functions that hold the boundary.
### Operate — run it in production
- [deployment.md](deployment.md) — self-hosting: the container materials, the two-role non-superuser database model the grant wall requires, env-only configuration, one-host routing for `/v1` + `/mcp` + the OAuth flow, health checks, and order of operations.
- [desktop-distribution.md](explanation/desktop-distribution.md) — the other shape: one folder a non-technical user runs on macOS or Windows with no Docker. Why it must carry its own Postgres (pgvector is not in `contrib`), how relocatability is enforced and verified on each platform, the update contract the folder layout encodes, single-file configuration, the four places the two platforms are forced apart (socket vs. loopback auth, `pg_ctl` vs. a child process, Valkey vs. Redis, signing), and the limits — collation, signing, and the socket-path ceiling.

## Reading order for a new contributor

1. [tutorials/getting-started.md](tutorials/getting-started.md) — get it running.
2. [explanation/backend-onboarding.md](explanation/backend-onboarding.md) — the map + reading order hub.
3. [architecture.md](explanation/architecture.md) → [contract-first.md](explanation/contract-first.md) → [authorization.md](explanation/authorization.md).
4. Then read the one page nearest the change you came to make — pick from the
   grouped **Explanation** map above; *The platform spine* is the group that
   applies to almost every backend change. Working on the SPA instead? Start at
   [frontend-architecture.md](explanation/frontend-architecture.md).
5. [CONTRIBUTING.md](../CONTRIBUTING.md) + `AGENTS.md` — the PR loop and the binding engineering rules.
