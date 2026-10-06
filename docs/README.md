# Margince documentation

**Using the product rather than changing it?** The [handbook](handbook/README.md)
answers "how do I…" in the app, and it is also the corpus behind the in-app ask.
End-to-end walkthroughs for a rep or a delivery lead live in
[`user-guide/`](../user-guide/README.md). The rest of this tree is for the
developer changing or operating the code, so a product task belongs in the
handbook and not in `how-to/`.

Documentation for building and operating **Margince**, a governed, single-tenant CRM (a Go `/v1` API
backend; the Vite/React web UI ships separately). One installation serves one company, and boot
refuses a second. The docs follow the [Diátaxis](https://diataxis.fr/) split: **tutorials** to learn,
**how-to** guides for tasks, **reference** for lookup, **explanation** for the *why*. The
**[principles](principles/README.md)** add the statements about this codebase's shape that
settle a class of arguments before they start.

**Designing anything a reader can see?** [`DESIGN.md`](../DESIGN.md) at the
repository root is the visual language: the look every new surface is designed
against, and the research behind it. The plan for landing it, one PR per
step, is [how-to/adopt-the-design.md](how-to/adopt-the-design.md), continued in
[adopt-the-design-records.md](how-to/adopt-the-design-records.md) for the record
pages and [adopt-the-design-surfaces.md](how-to/adopt-the-design-surfaces.md)
for the rest. The component catalog stays in
[`frontend/src/design-system/README.md`](../frontend/src/design-system/README.md).

**New to the backend?** Start with [tutorials/getting-started.md](tutorials/getting-started.md), then
[explanation/backend-onboarding.md](explanation/backend-onboarding.md): the orientation hub that
maps the codebase and links everything below.

## Map

### Handbook: how to use the product

The one tree here written for the user of Margince rather than building it:
no code, no API, just the app.

- [handbook/README.md](handbook/README.md): the handbook pages covering records,
  the pipeline, partners, capture, mail, what the AI does and does not do, approvals,
  offers, documents, retention, seats and settings. It is also the default
  document set behind **Ask your documents** on a new installation, so
  [how-to/update-the-handbook.md](how-to/update-the-handbook.md) says how to
  write a page that can answer a question, and how to measure it.

### Compliance: the German pack a customer signs

The documents an installation needs before it may read employee mail in
Germany. They are the customer's paperwork: Margince does not check them, and
[handbook/compliance.md](handbook/compliance.md) says why.

- [compliance/en/README.md](compliance/en/README.md): the index, in English,
  with each German original beside its translation. The documents to **execute**
  are the German ones in [compliance/de/](compliance/de/), because a translation
  of a legal document is not the binding text.

### Incident reporting: our duty as manufacturer

What we do when a weakness in Margince is being exploited in the wild: who
files, to whom, and against which clocks. It is written in advance because the
first deadline is 24 hours.

- [compliance/cra/README.md](compliance/cra/README.md): the runbook, the three
  filled report skeletons and the tabletop walk (CRA Article 14).

### Principles: how this codebase decides things

- [principles/README.md](principles/README.md): the index. Each page carries the statement, the method for checking the tree still holds it, and what it does not ask for.
- [one-source-of-truth.md](principles/one-source-of-truth.md): one place decides each topic, and module boundaries decide where that place may live. Carries the duplication scan.
- [the-record-is-the-code.md](principles/the-record-is-the-code.md): what outranks what when two sources disagree.
- [every-mutation-leaves-a-trace.md](principles/every-mutation-leaves-a-trace.md): domain row, audit row and event commit together.
- [legibility-is-the-product.md](principles/legibility-is-the-product.md): why the craft bar is a gate rather than taste.
- [derive-the-obligation.md](principles/derive-the-obligation.md): how to write a fitness function that holds.
- [nothing-here-is-private.md](principles/nothing-here-is-private.md): the public reader, and why a working exploit takes the private path.

### Tutorials: learn by doing
- [getting-started.md](tutorials/getting-started.md): from clone to a running instance with a bootstrapped workspace.

### How-to: accomplish a task
- [Import purchased employment history](how-to/import-employment-history.md): contact channels, employer matching, installation and backfill.
- [Inventory what history left in the Worklist](how-to/inventory-home-attention.md): a read-only count of imported mail, old requests, privacy duties and proposals before a repair.
- [Operate sales reporting](how-to/operate-reporting.md): setup, captures, schedules, privacy and durable schedule pause.
- [add-an-endpoint.md](how-to/add-an-endpoint.md): add or change an API operation (contract, gen, handler).
- [add-a-module.md](how-to/add-a-module.md): add a new capability (module) or a cross-module edge, wired into compose.
- [add-a-job.md](how-to/add-a-job.md): declare a background job kind in the job contract, then write and register its worker.
- [add-an-rbac-object.md](how-to/add-an-rbac-object.md): add a new RBAC object across the policy, the contract enum, the backfill migration and the published matrix.
- [create-a-workflow.md](how-to/create-a-workflow.md): scaffold and wire a new automation starter workflow into the closed catalog.
- [apply-migrations.md](how-to/apply-migrations.md): write and apply a database migration.
- [claim-a-red-main.md](how-to/claim-a-red-main.md): say you are fixing a red `main` before you start, so parallel sessions do not all diagnose it.
- [work-on-an-issue.md](how-to/work-on-an-issue.md): check whether an issue is taken, claim the one you work on, release it when you stop, and re-derive its ruling from the code first.
- [mint-a-passport.md](how-to/mint-a-passport.md): issue an agent passport token.
- [connect-an-mcp-client.md](how-to/connect-an-mcp-client.md): connect a client to the governed MCP tool surface.
- [test-the-mcp-surface-end-to-end.md](how-to/test-the-mcp-surface-end-to-end.md): drive the MCP surface with a real Claude, GPT or Mistral (`make e2e-llm`), tell a harness stop from a finding, and publish to the coverage page.
- [improve-mcp-quality.md](how-to/improve-mcp-quality.md): raise the score for Claude and Codex by triaging each red case to the layer at fault (harness, grader, copy, result shape or model) and re-running with controls.
- [run-the-frontend.md](how-to/run-the-frontend.md): run the SPA in dev.
- [connect-a-mailbox.md](how-to/connect-a-mailbox.md): connect a mailbox for capture through Gmail OAuth, IMAP app-password, Microsoft Graph OAuth or Google Calendar.
- [enrich-with-a-local-llm.md](how-to/enrich-with-a-local-llm.md): point the AI lanes at a local Ollama and enrich a company with no cloud key.
- [read-what-a-company-runs.md](how-to/read-what-a-company-runs.md): what DNS, certificate logs and a homepage fingerprint write onto a company, what triggers it, and the setting that turns it on.
- [check-a-vat-number.md](how-to/check-a-vat-number.md): ask the EU register whether a company's VAT ID is real, read the receipt a tax authority accepts, and the setting that turns it on.
- [set-up-outbound-mail.md](how-to/set-up-outbound-mail.md): which mail goes out through a rep's mailbox and which through the SMTP relay, the `email:` block, and what works without a relay.
- [connect-telegram.md](how-to/connect-telegram.md): bind a workspace-level Telegram bot for pull ingress and governed replies.
- [import-your-linkedin-network.md](how-to/import-your-linkedin-network.md): import your own `Connections.csv` as graph substrate, and read the reach it buys.
- [import-a-company-spreadsheet.md](how-to/import-a-company-spreadsheet.md): bring in a CSV of companies, with the column mapping, the preview counts, and how a row names the company it corrects.
- [connect-a-cloud-model-provider.md](how-to/connect-a-cloud-model-provider.md): bind the AI lanes to a BYOK cloud key (Anthropic, OpenAI, Gemini or any OpenAI-compatible vendor).
- [recover-after-a-provider-outage.md](how-to/recover-after-a-provider-outage.md): read the AI provider status card, fix the cause, and reopen the sender questions and company enrichments an outage parked (`worker reopen-parked`, dry run first).
- [certify-an-ai-model.md](how-to/certify-an-ai-model.md): certify a model against a task's fixture corpus and benchmark a candidate swap (`make e2e-ai`).
- [certify-a-decision-site.md](how-to/certify-a-decision-site.md): certify a decision model for a site so the `decisions:` lane may answer it, and re-certify or drop it when the site changes.
- [re-certify-the-whole-corpus.md](how-to/re-certify-the-whole-corpus.md): the sweep after a tree-wide change stales every record, and how to tell a moved question from a model regression.
- [add-an-ai-task.md](how-to/add-an-ai-task.md): add a new AI task or invocation site: declare it in the contract, wire the lane, register the site, certify it.
- [write-a-certification-case.md](how-to/write-a-certification-case.md): bind a site to the production request builder and validator that certify it, test first, with scenario and rubric authoring.
- [register-a-webhook.md](how-to/register-a-webhook.md): register an HTTPS endpoint for signed, retried event delivery, and inspect or replay a delivery.
- [add-an-extension.md](how-to/add-an-extension.md): ship a stable-tier extension unit (a jurisdiction pack) under `extensions/`, composed and verified.
- [debug-an-ai-task.md](how-to/debug-an-ai-task.md): run one production AI invocation site against your own input (`make ai-probe`) and read each step to the verdict as numbers.
- [tune-ai-requests.md](how-to/tune-ai-requests.md): read what the model calls did per provider, tier and task, then set OpenRouter routing and privacy and each task's thinking level and timeouts.
- [build-the-desktop-app.md](how-to/build-the-desktop-app.md): build the folder that runs the whole stack with no Docker, on macOS (`make desktop`) or Windows (`make desktop-win`), then run, configure and update it.
- [update-the-handbook.md](how-to/update-the-handbook.md): change the handbook so the in-app ask can answer from it, with the probe (`scripts/handbook-ask/probe.sh`) to run before and after.
- [cut-a-release.md](how-to/cut-a-release.md): push a `v*` tag and get a GitHub release with both desktop bundles attached, and recover from a failed lane.

### Reference: look it up
- [modules.md](reference/modules.md): the modules, what each owns, its tables, its HTTP surface.
- [entity-model/](reference/entity-model/README.md): every table and column and what each one is, one page per owning area. Generated.
- [brief-priorities.md](reference/brief-priorities.md): how morning priorities, scoped risk and weekly coverage are derived.
- [meeting-brief.md](reference/meeting-brief.md): the pre-meeting brief and its preparation plan, what a caller is and is not shown, and how a year of history becomes a few moments.
- [agent-tools.md](reference/agent-tools.md): the governed tool catalog, with each tool's tier, the passport scope it spends, and its egress.
- [mcp-info.md](reference/mcp-info.md): the served MCP surface as a client receives it, with `mcp-info.json` beside it. Generated; a lookup table, not something to read through.
- [ai-prompts.md](reference/ai-prompts.md): every instruction this build sends a model, read off real requests. Generated.
- [agent-tool-budget.md](reference/agent-tool-budget.md): what each agent's tool menu costs in prompt tokens against the published ceiling, with its `.json` sibling. Generated.
- [ai-certification.md](reference/ai-certification.md): which AI features are certified, per model family, preset and invocation site, with `ai-certification.json` beside it. Generated.
- [mcp-tool-coverage.md](reference/mcp-tool-coverage.md): which served MCP tools the use-case lane drives, what those cases (from `e2e/llm/scenarios`) scored, and what each tool costs, with `mcp-tool-coverage.json` beside it. Generated.
- [rbac-matrix.md](reference/rbac-matrix.md): what each seeded role may do to each kind of record. Generated.
- [performance-budgets.md](reference/performance-budgets.md): every published performance budget, its last measurement and the machine it ran on. Generated by the `make bench-*` targets; a record, not a gate.
- [supply-chain.md](reference/supply-chain.md): the source-tree SBOMs, the license gate, keyless signing, and the pinned toolchain.
- [ci-workflows.md](reference/ci-workflows.md): the workflows that run beside the merge gate: what each triggers on, what it gates, and what a red one means.
- [platform-toolkit.md](reference/platform-toolkit.md): the reusable `platform/*` + `shared/*` utilities.
- [gate-patterns.md](reference/gate-patterns.md): the shapes a fitness gate comes in, how strong each can be, and how each one can pass while blind. Read it before writing a gate; [principles/derive-the-obligation.md](principles/derive-the-obligation.md) is the method.
- [gate-inventory.md](reference/gate-inventory.md): every gate in `backend/`, grouped by shape, with what it holds. Generated from the `//gate:kind` line each gate declares.
- [configuration.md](reference/configuration.md): every binary flag and environment variable.
- [ai-provider-key-test.md](reference/ai-provider-key-test.md): what the Models page's Test button asks each provider, what counts as a pass, and what each failure reason means.
- [ollama-self-hosting.md](reference/ollama-self-hosting.md): local models measured through Ollama on a 24GB Mac mini M4, with speed, fit, pass rate per task and the traps (default thinking, `think:false` on gpt-oss). Read it before re-testing a local model.
- [vllm-self-hosting.md](reference/vllm-self-hosting.md): the same machine serving models through vLLM (vllm-metal), the server flags that change answers, and a pass rate and latency per model.
- [ai-thinking.md](reference/ai-thinking.md): how much a model thinks before it answers: the levels, where they are set, their precedence, and the wire field each provider is sent.
- [openrouter.md](reference/openrouter.md): how upstream host selection works, the shipped `routing:` default and why, and which preferences filter versus reorder.
- [openrouter-routing-fields.md](reference/openrouter-routing-fields.md): every key a `routing:` value may carry, whether a tier or the connection owns it, and how a misplaced or unknown key is refused.
- [make-targets.md](reference/make-targets.md): every `make` target.
- [system-requirements.md](reference/system-requirements.md): what an installation needs, on one node or with the api, worker, web and database on separate nodes.
- [ai-egress.md](reference/ai-egress.md): every declared AI task, and whether the text it reads can leave the installation. Generated from `backend/api/ai-tasks.yaml`.
- [ai-provider-outages.md](reference/ai-provider-outages.md): every declared AI task, and what it does while its provider is down, out of credit or refusing its key. Generated from `backend/api/ai-tasks.yaml`.
- [record-vocabulary.md](reference/record-vocabulary.md): the rule that the reader and the program use the same record noun, and the gates that keep a second spelling out.
- [ui-copy-style.md](reference/ui-copy-style.md): the standard for the English UI catalog, and which rules `frontend/src/i18n/copy-style.test.ts` holds.
- [docs-prose-style.md](reference/docs-prose-style.md): the voice and the house bar every Markdown page is held to, and which rules `backend/gates/docsprose_test.go` checks.
- [ui-copy-style-de.md](reference/ui-copy-style-de.md): what German copy adds to that standard, with what `copy-style-de.test.ts` and `address-register.test.ts` hold.
- [issue-labels.md](reference/issue-labels.md): the full issue-label taxonomy. The binding short form is in `AGENTS.md`.
- [license-release-rule.md](reference/license-release-rule.md): the BUSL Change-Date release-stamping rule. The per-file SPDX header rule is in [backend-onboarding.md](explanation/backend-onboarding.md) and `AGENTS.md`.
- [sonarcloud-deviations.md](reference/sonarcloud-deviations.md): the SonarCloud findings that stay open by decision, each with what would break if the rule were applied.

Generated pages, and the `perfbench/` records, say so in their first line. Do not hand-edit them.

### Explanation: understand the why

**Start here: the shape of the system**

- [backend-onboarding.md](explanation/backend-onboarding.md): the contributor hub: system overview, the map, what is generated and what is hand-written, the store shape, the gates.
- [architecture.md](explanation/architecture.md): the module DAG, the spine shapes, tenancy as structure.
- [contract-first.md](explanation/contract-first.md): how code is generated from `crm.yaml`.
- [authorization.md](explanation/authorization.md): why the auth check lives at the store entry point, the structural backstop, and what a passport is.
- [rbac-roles-and-teams.md](explanation/rbac-roles-and-teams.md): the role matrix, row scope (own, team, all), teams, role assignment and per-record sharing.

**The platform spine: how a change is written**

- [write-backbone.md](explanation/write-backbone.md): storekit, `audit_log`, the outbox, and who consumes the events.
- [composition-layer.md](explanation/composition-layer.md): how `internal/compose/` boots and where every cross-module edge is wired.
- [job-fleet.md](explanation/job-fleet.md): the job contract: declaration before code, dispatchers and workspace workers, why args name rows, and the failure vocabulary.
- [prompt-shape.md](explanation/prompt-shape.md): how a prompt is built, why prompt caching cannot help us, and how many items a task asks about per call.
- [ai-certification.md](explanation/ai-certification.md): how an AI feature earns its grade, from one try to the pooled grade, and the order to fix a failing feature in.
- [prompt-principles.md](explanation/prompt-principles.md): the rules a production prompt keeps, the check that holds each, the optimisation order (the `gemini_cloud` baseline first), and the pre-merge checklist.
- [raw-capture-part-slimming.md](explanation/raw-capture-part-slimming.md): why the stored original keeps its attachments by reference, and why the strip is by byte offset.
- [custom-fields.md](explanation/custom-fields.md): the one runtime `ALTER TABLE` chokepoint, the closed type and object sets, and the `fieldcatalog` seam.

**Capture, messaging and privacy**

- [capture-connectors.md](explanation/capture-connectors.md): the governed ingress surface: the connector seam, the Sink that owns every write, the scope gate, the ingestion modes, OAuth and sealed credentials.
- [ingress-gate-and-auto-capture.md](explanation/ingress-gate-and-auto-capture.md): what happens to a message after a connector fetches it, which steps use AI, and what the **Capture activity** tab shows.
- [mail-history-import.md](explanation/mail-history-import.md): the bounded backward scan a fresh mailbox is offered, its consent estimate (`observed` or `heuristic`, never a false `$0`), and its resumable page loop.
- [channel-capture-parity.md](explanation/channel-capture-parity.md): whose correspondence a captured chat is, and what the member-bound and shared paths each get.
- [outbound-messaging.md](explanation/outbound-messaging.md): the egress twin of capture: the staging row, the transmit-time gates, receipt before bookkeeping, and the channel reply.
- [outbound-webhooks.md](explanation/outbound-webhooks.md): subscriptions and the delivery engine, the contract-first payload pipeline (`api/public-events.yaml`, `gen-payloads`, the `EmitEvent` seam), retries and the fan-out gate.
- [scheduling.md](explanation/scheduling.md): how a meeting time is proposed, whose working hours decide the slots, which clock they are read on, and how `activities` reaches a fact `identity` owns.
- [privacy-and-consent.md](explanation/privacy-and-consent.md): the engine that decides whether each message may go, and the GDPR engines (erasure, SAR, retention).

**AI, retrieval and automation**

- [ai-runtime.md](explanation/ai-runtime.md): the AI task contract, tiers and ladders, the routing config, the Router gate, tracing and certification.
- [ai-provider-health.md](explanation/ai-provider-health.md): the provider health states, what trips and clears each, why a blocked provider is skipped with no call and no charge, and why a throttle is not a state.
- [ai-request-settings.md](explanation/ai-request-settings.md): per-task thinking and timeouts, the deadline every ladder rung runs under, and the call figures the admin screens read.
- [agent-surface.md](explanation/agent-surface.md): the Surface-B reasoning loop and the model runtime.
- [ai-provenance-notice.md](explanation/ai-provenance-notice.md): the sentence a model-written draft carries, why it discharges no EU AI Act Art. 50 duty, and the path that would change that.
- [ai-activity-rail.md](explanation/ai-activity-rail.md): what the AI is doing for you while it does it: the `ai_task_run` projection, who reports into it, the derived `stalled`, and which kinds a reader is shown.
- [search-and-retrieval.md](explanation/search-and-retrieval.md): the lexical and hybrid lanes, row scope inside the query, embedding identity, and the two kinds of staleness.
- [relationship-graph.md](explanation/relationship-graph.md): who on our team knows this contact: participants, the interaction projection, warmth, deal coverage and its risk rules.
- [company-context.md](explanation/company-context.md): the cold start, the governed profile of the *installation's own* company, and how its context reaches AI tasks. The company **record** page is below.
- [automation.md](explanation/automation.md): the closed trigger and action catalog, the firing path, the anchor occurrence key, and both permission gates.

**The product surface**

- [customer-requests.md](explanation/customer-requests.md): recognition, acceptance and completion of customer requests across email, tasks, Home and closed deals.
- [frontend-architecture.md](explanation/frontend-architecture.md): the SPA's layers, the shell and its nav rules, the colour and theme contract, and the gates that fail a frontend push.
- [pwa.md](explanation/pwa.md): the installable app: the manifest, the service worker, the offline page, the install offer, and how to turn the worker off in an emergency.
- [contact-record-page.md](explanation/contact-record-page.md): sparse and active contact layouts, communication permissions, and Focus ranking.
- [company-record-page.md](explanation/company-record-page.md): the company record page, its gated 360 read, Ask, suggestions, and why view state carries no audit row.
- [concurrent-record-edits.md](explanation/concurrent-record-edits.md): how the record editors merge independent field changes and recover from a conflict.

**Modes and extension**

- [extensibility.md](explanation/extensibility.md): the stable extension tier: the compile-time declaration, the allowlisted surface, the composition build, the `GOWORK` binding, and the fitness functions that hold the boundary.

**Building and merging**

- [ci-pipeline.md](explanation/ci-pipeline.md): the merge gate as GitHub Actions: the merge queue, the change classifier, the job graph, the Go build cache, and coverage to SonarCloud.

### Operate: run it in production
- [deployment.md](deployment.md): self-hosting: the container materials, the two-role database model, env-only configuration, one-host routing for `/v1` + `/mcp` + OAuth, health checks, and order of operations.
- [desktop-distribution.md](explanation/desktop-distribution.md): one folder a non-technical user runs on macOS or Windows with no Docker, why it carries its own Postgres (pgvector is not in `contrib`), and the update contract. It also covers where the two platforms differ (socket or loopback auth, `pg_ctl` or a child process, Valkey or Redis, signing).

### Evidence: kept records, not current behaviour

- [evidence/extension-tier/README.md](evidence/extension-tier/README.md): the
  acceptance evidence for the extension tier, recorded 2026-08-28. The unit it
  shows is no longer in the tree, so read it as how the tier was proved and not
  as what ships. It stays as the record of what was checked.

## Reading order for a new contributor

1. [tutorials/getting-started.md](tutorials/getting-started.md): get it running.
2. [explanation/backend-onboarding.md](explanation/backend-onboarding.md): the map and reading-order hub.
3. [architecture.md](explanation/architecture.md), then [contract-first.md](explanation/contract-first.md), then [authorization.md](explanation/authorization.md).
4. Then read the page nearest the change you came to make, from the
   **Explanation** map above. *The platform spine* applies to almost every
   backend change. Working on the SPA instead? Start at
   [frontend-architecture.md](explanation/frontend-architecture.md).
5. [CONTRIBUTING.md](../CONTRIBUTING.md) + `AGENTS.md`: the PR loop and the binding engineering rules.
