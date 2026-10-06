<!-- prose:plain -->
# Margince documentation

**Do you only use the product?** The [handbook](handbook/README.md) answers questions
about how to use the app. **Ask your documents** in the app also reads its answers from the handbook.
Step-by-step guides for sales and delivery teams are in [user-guide/README.md](../user-guide/README.md). The rest of this tree is
for the developer who changes or runs the code. So a product task goes in the handbook, not in
`how-to/`.

These docs explain how to build and run Margince. It is a CRM with one company per installation, and
the server refuses a second one. The backend is a Go API under `/v1`, and the React web app ships
on its own. The docs follow the four kinds of [Diátaxis](https://diataxis.fr/). Tutorials help you learn, how-to
guides do tasks, reference pages are to look things up, and explanation pages say why. The
[principles](principles/README.md) answer many questions about the shape of this code before they
start.

**Do you design something a user can see?** [DESIGN.md](../DESIGN.md) holds the look of the app, and
the research behind it. The plan to add it to the app, one pull request per step, is [how-to/adopt-the-design.md](how-to/adopt-the-design.md). It goes on in
[how-to/adopt-the-design-records.md](how-to/adopt-the-design-records.md) for the record pages and
[how-to/adopt-the-design-surfaces.md](how-to/adopt-the-design-surfaces.md) for the rest. The list of
web app parts is in
[frontend/src/design-system/README.md](../frontend/src/design-system/README.md).

**New to the backend?** Start with [tutorials/getting-started.md](tutorials/getting-started.md), then
[explanation/backend-onboarding.md](explanation/backend-onboarding.md). That page maps the code and
links every page below.

## Map

### Handbook: how to use the product

The one tree here for the user of Margince, not the developer. It has no code and no API, only the app.

- [handbook/README.md](handbook/README.md): records, the pipeline, partners, capture, mail, what the AI
  does and does not do, approvals, offers, documents, retention, seats and settings.
- On a new installation, **Ask your documents** reads its answers from these pages. To write a page
  that can answer a question, and to measure it, read
  [how-to/update-the-handbook.md](how-to/update-the-handbook.md).

### Compliance: the German documents a customer signs

An installation needs these documents before it may read the mail of its own team in Germany. The
customer signs them, and Margince does not check them.
[handbook/compliance.md](handbook/compliance.md) says why.

- [compliance/en/README.md](compliance/en/README.md): the list, in English, with each German document
  next to its English copy. Sign the German ones in [compliance/de/](compliance/de/), because only
  the German text is the legal one.

### Attacks: what we must do as the maker

What we do when an attacker uses a hole in Margince: who reports it, where, and by which dates. It is
ready before we need it, because the first date comes 24 hours after we learn of the attack.

- [compliance/cra/README.md](compliance/cra/README.md): the steps, the three report forms, already
  filled in, and the test run (CRA Article 14).

### Principles: how this code decides

- [principles/README.md](principles/README.md): the list. Each page gives the rule, a way to check that
  the tree still keeps it, and what it does not ask for.
- [one-source-of-truth.md](principles/one-source-of-truth.md): one place decides each question, and
  module lines decide where that place may be. It has the scan for copies.
- [the-record-is-the-code.md](principles/the-record-is-the-code.md): which source comes first when two disagree.
- [every-mutation-leaves-a-trace.md](principles/every-mutation-leaves-a-trace.md): the data row, the
  audit row and the event commit together.
- [legibility-is-the-product.md](principles/legibility-is-the-product.md): why the code rules are a check that
  can fail.
- [derive-the-obligation.md](principles/derive-the-obligation.md): how to write a check that holds.
- [nothing-here-is-private.md](principles/nothing-here-is-private.md): the public reader, and why a
  working attack takes the private route.

### Tutorials: learn by doing

- [getting-started.md](tutorials/getting-started.md): from a new copy to a running app with a
  workspace.

### How-to: do a task

- [how-to/import-employment-history.md](how-to/import-employment-history.md): import a job history list you buy
  for contacts.
- [how-to/inventory-home-attention.md](how-to/inventory-home-attention.md): count what old data is in the
  Worklist before a repair. It changes nothing.
- [how-to/rename-key-named-deals.md](how-to/rename-key-named-deals.md): rename imported deals whose name
  is the source key. Test with a dry run first.
- [how-to/operate-reporting.md](how-to/operate-reporting.md): run sales reports and their schedules.
- [add-an-endpoint.md](how-to/add-an-endpoint.md): add or change an API call.
- [add-a-module.md](how-to/add-a-module.md): add a module, or link two modules.
- [add-a-job.md](how-to/add-a-job.md): add a job.
- [add-an-rbac-object.md](how-to/add-an-rbac-object.md): add a new kind of record to RBAC.
- [create-a-workflow.md](how-to/create-a-workflow.md): add a new starter workflow.
- [apply-migrations.md](how-to/apply-migrations.md): write and run a database migration.
- [claim-a-red-main.md](how-to/claim-a-red-main.md): say that you fix a red `main` before you start.
- [work-on-an-issue.md](how-to/work-on-an-issue.md): check that an issue is free, claim it, and release
  it when you stop.
- [mint-a-passport.md](how-to/mint-a-passport.md): make a passport token for an agent.
- [connect-an-mcp-client.md](how-to/connect-an-mcp-client.md): connect a client to the MCP tools.
- [test-the-mcp-surface-end-to-end.md](how-to/test-the-mcp-surface-end-to-end.md): test the MCP tools
  with a real model (`make e2e-llm`).
- [improve-mcp-quality.md](how-to/improve-mcp-quality.md): find why a model fails an MCP test, and fix
  the right part.
- [run-the-frontend.md](how-to/run-the-frontend.md): run the web app while you develop.
- [connect-a-mailbox.md](how-to/connect-a-mailbox.md): connect a mailbox through Gmail, IMAP, Microsoft
  or Google Calendar.
- [enrich-with-a-local-llm.md](how-to/enrich-with-a-local-llm.md): use a local Ollama model, with no
  cloud key.
- [read-what-a-company-runs.md](how-to/read-what-a-company-runs.md): see what tools a company runs, from
  its public web data.
- [check-a-vat-number.md](how-to/check-a-vat-number.md): check the VAT number of a company with the EU
  register.
- [set-up-outbound-mail.md](how-to/set-up-outbound-mail.md): which mail goes out through which server.
- [connect-telegram.md](how-to/connect-telegram.md): connect a Telegram bot to a workspace.
- [import-your-linkedin-network.md](how-to/import-your-linkedin-network.md): import your own LinkedIn
  contacts.
- [import-a-company-spreadsheet.md](how-to/import-a-company-spreadsheet.md): import a CSV file of
  companies.
- [connect-a-cloud-model-provider.md](how-to/connect-a-cloud-model-provider.md): use your own key for a
  cloud model provider.
- [recover-after-a-provider-outage.md](how-to/recover-after-a-provider-outage.md): what to do after a
  model provider goes down.
- [certify-an-ai-model.md](how-to/certify-an-ai-model.md): test a model against a task before you use it
  (`make e2e-ai`).
- [certify-a-decision-site.md](how-to/certify-a-decision-site.md): test a model for one place in the code
  where it decides.
- [re-certify-the-whole-corpus.md](how-to/re-certify-the-whole-corpus.md): test every model after a change to
  the whole tree.
- [add-an-ai-task.md](how-to/add-an-ai-task.md): add a new AI task.
- [write-a-certification-case.md](how-to/write-a-certification-case.md): write a test case that
  certifies a model.
- [register-a-webhook.md](how-to/register-a-webhook.md): send events to your own HTTPS endpoint.
- [add-an-extension.md](how-to/add-an-extension.md): add an extension under `extensions/`.
- [debug-an-ai-task.md](how-to/debug-an-ai-task.md): run one AI task on your own input
  (`make ai-probe`).
- [tune-ai-requests.md](how-to/tune-ai-requests.md): read what model calls cost, and set how each task
  calls its model.
- [build-the-desktop-app.md](how-to/build-the-desktop-app.md): build the desktop app for macOS or
  Windows.
- [update-the-handbook.md](how-to/update-the-handbook.md): change the handbook so **Ask your documents** can answer
  from it.
- [cut-a-release.md](how-to/cut-a-release.md): push a `v*` tag to make a GitHub release.

### Reference: look it up

- [modules.md](reference/modules.md): each module, what it owns, its tables and its API.
- [entity-model/](reference/entity-model/README.md): every table and field, one page per part
  of the product.
  Generated.
- [brief-priorities.md](reference/brief-priorities.md): how the app builds the morning list of work.
- [meeting-brief.md](reference/meeting-brief.md): the brief a user reads before a meeting.
- [agent-tools.md](reference/agent-tools.md): each agent tool, and what it may do.
- [mcp-info.md](reference/mcp-info.md): the MCP tools as a client sees them. Generated.
- [ai-prompts.md](reference/ai-prompts.md): every text this build sends to a model. Generated.
- [agent-tool-budget.md](reference/agent-tool-budget.md): what the tool list of each agent costs a
  model. Generated.
- [ai-certification.md](reference/ai-certification.md): which AI features each model passed. Generated.
- [mcp-tool-coverage.md](reference/mcp-tool-coverage.md): which MCP tools the tests use, and how they
  score. Generated.
- [rbac-matrix.md](reference/rbac-matrix.md): what each role may do to each kind of record. Generated.
- [performance-budgets.md](reference/performance-budgets.md): each speed limit and its last
  score. Generated.
- [supply-chain.md](reference/supply-chain.md): what goes into a build, and how we sign it.
- [ci-workflows.md](reference/ci-workflows.md): the GitHub workflows that run next to the merge check.
- [platform-toolkit.md](reference/platform-toolkit.md): shared code for every module.
- [gate-patterns.md](reference/gate-patterns.md): the kinds of check, and how each can miss things.
  Read it, and [principles/derive-the-obligation.md](principles/derive-the-obligation.md), before you
  write one.
- [gate-inventory.md](reference/gate-inventory.md): every check in `backend/`, by kind. Generated.
- [configuration.md](reference/configuration.md): every setting of each program.
- [ai-provider-key-test.md](reference/ai-provider-key-test.md): what the Test button on the Models page
  checks.
- [ollama-self-hosting.md](reference/ollama-self-hosting.md): local models in Ollama, measured on one
  small machine.
- [vllm-self-hosting.md](reference/vllm-self-hosting.md): the same machine with vLLM.
- [ai-thinking.md](reference/ai-thinking.md): how much a model thinks before it answers, and where you
  set it.
- [openrouter.md](reference/openrouter.md): how OpenRouter chooses a host, and the setting
  we ship.
- [openrouter-routing-fields.md](reference/openrouter-routing-fields.md): each field of a `routing:`
  setting.
- [make-targets.md](reference/make-targets.md): every `make` command.
- [system-requirements.md](reference/system-requirements.md): what an installation needs, on one
  machine or on many.
- [ai-egress.md](reference/ai-egress.md): which AI tasks can send data out of the installation.
  Generated.
- [ai-provider-outages.md](reference/ai-provider-outages.md): what each AI task does while its provider
  is down. Generated.
- [record-vocabulary.md](reference/record-vocabulary.md): one name for each kind of record, in the app
  and in the code.
- [ui-copy-style.md](reference/ui-copy-style.md): how to write English text in the app.
- [docs-prose-style.md](reference/docs-prose-style.md): how to write a doc page.
- [ui-copy-style-de.md](reference/ui-copy-style-de.md): what German text in the app adds.
- [issue-labels.md](reference/issue-labels.md): every issue label. `AGENTS.md` has the short form.
- [license-release-rule.md](reference/license-release-rule.md): the license date on each release.
  [backend-onboarding.md](explanation/backend-onboarding.md) and `AGENTS.md` have the license line each
  file starts with.
- [sonarcloud-deviations.md](reference/sonarcloud-deviations.md): the SonarCloud findings we keep open,
  and why.

A generated page, and each `perfbench/` record, says so in its first line. Do not change them by hand.

### Explanation: understand the why

#### The shape of the system

- [backend-onboarding.md](explanation/backend-onboarding.md): start here. A map of the backend, and the
  order to read it in.
- [architecture.md](explanation/architecture.md): how the modules work together.
- [contract-first.md](explanation/contract-first.md): how code is built from `crm.yaml`.
- [authorization.md](explanation/authorization.md): where the code checks rights, and what a passport
  is.
- [rbac-roles-and-teams.md](explanation/rbac-roles-and-teams.md): roles, teams, row scope and sharing a
  record.

#### How the code writes a change

- [write-backbone.md](explanation/write-backbone.md): the audit log, the outbox, and who reads the
  events.
- [composition-layer.md](explanation/composition-layer.md): how `internal/compose/` starts the server
  and links the modules.
- [job-fleet.md](explanation/job-fleet.md): how a job is declared, run and failed.
- [prompt-shape.md](explanation/prompt-shape.md): how a prompt is built.
- [ai-certification.md](explanation/ai-certification.md): how an AI feature gets its grade.
- [prompt-principles.md](explanation/prompt-principles.md): the rules a prompt keeps, and the checks
  before a merge.
- [raw-capture-part-slimming.md](explanation/raw-capture-part-slimming.md): how a stored mail holds its
  files.
- [custom-fields.md](explanation/custom-fields.md): how a custom field is added to a table.

#### Capture, messages and privacy

- [capture-connectors.md](explanation/capture-connectors.md): how mail and chat come in.
- [ingress-gate-and-auto-capture.md](explanation/ingress-gate-and-auto-capture.md): what happens to a
  message once it is in, and where AI helps.
- [mail-history-import.md](explanation/mail-history-import.md): how a new mailbox imports its old mail.
- [channel-capture-parity.md](explanation/channel-capture-parity.md): whose messages a captured chat
  holds.
- [outbound-messaging.md](explanation/outbound-messaging.md): how a message goes out.
- [outbound-webhooks.md](explanation/outbound-webhooks.md): how events reach a webhook.
- [scheduling.md](explanation/scheduling.md): how the app finds a meeting time.
- [privacy-and-consent.md](explanation/privacy-and-consent.md): whether a message may go, and the GDPR
  rules.

#### AI, search and workflows

- [ai-runtime.md](explanation/ai-runtime.md): how an AI task chooses and calls its model.
- [ai-provider-health.md](explanation/ai-provider-health.md): how we know a model provider is down.
- [ai-request-settings.md](explanation/ai-request-settings.md): the time and thinking limits of each
  task.
- [agent-surface.md](explanation/agent-surface.md): how an agent works.
- [ai-provenance-notice.md](explanation/ai-provenance-notice.md): the line on a draft from a model, and
  the EU AI Act rule it does not meet.
- [ai-activity-rail.md](explanation/ai-activity-rail.md): how the app shows what the AI does while it
  works.
- [search-and-retrieval.md](explanation/search-and-retrieval.md): how search finds records.
- [relationship-graph.md](explanation/relationship-graph.md): who on our team knows a contact.
- [company-context.md](explanation/company-context.md): the profile of the company that runs the
  installation. The company record page is below.
- [automation.md](explanation/automation.md): what can start a workflow, and what it can do.

#### The product

- [customer-requests.md](explanation/customer-requests.md): how the app finds and tracks what a customer
  asks for.
- [frontend-architecture.md](explanation/frontend-architecture.md): how the web app is built.
- [pwa.md](explanation/pwa.md): the app you can install from the web.
- [contact-record-page.md](explanation/contact-record-page.md): the contact record page.
- [company-record-page.md](explanation/company-record-page.md): the company record page.
- [concurrent-record-edits.md](explanation/concurrent-record-edits.md): what happens when two users
  change one record.

#### Extensions

- [extensibility.md](explanation/extensibility.md): how an extension adds to Margince, and the checks
  that keep it in line.

#### Build and merge

- [ci-pipeline.md](explanation/ci-pipeline.md): how the merge check runs on GitHub.

### Run it in production

- [deployment.md](deployment.md): run Margince on your own servers.
- [desktop-distribution.md](explanation/desktop-distribution.md): the one folder that runs Margince on
  macOS or Windows, with no Docker.

### Kept records: not how it works today

- [evidence/extension-tier/README.md](evidence/extension-tier/README.md): the proof that the extension
  tier worked, kept from 2026-08-28. The extension it shows is not in the tree today.

## Reading order for a new developer

1. [tutorials/getting-started.md](tutorials/getting-started.md): get it running.
2. [explanation/backend-onboarding.md](explanation/backend-onboarding.md): the map of the code.
3. [architecture.md](explanation/architecture.md), then [contract-first.md](explanation/contract-first.md),
   then [authorization.md](explanation/authorization.md).
4. Then read the page about your change, from the Explanation list above. *How the code writes a
   change* is for most backend changes. For the web app, start at
   [frontend-architecture.md](explanation/frontend-architecture.md).
5. [CONTRIBUTING.md](../CONTRIBUTING.md) and `AGENTS.md`: how to send a change, and the rules it must
   follow.
