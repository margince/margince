# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
This is a pre-release proof of concept: nothing has been versioned or
released yet, so everything that exists lives under Unreleased. Version
numbers appear here when releases start.

## [Unreleased]

### Removed

- **The human-set sales quota is gone.** A quota was a revenue target a manager
  typed, and everything the product said downstream of it — attainment, the pace
  band — was that one number's arithmetic, so a target nobody had entered read as
  an absence rather than as a question with no denominator. The Reports screen now
  holds the three deal reports and nothing else. The `quota` table is dropped and
  the `quota` RBAC object is stripped from every role document in the same
  migration. Operational agent budgets are untouched: the per-agent volume meter,
  the AI provider spend budget and the incumbent rate allowance are safety limits,
  not sales targets, and keep working exactly as before.

### Changed

- **The relationship brief is written by a model, and reads what people actually
  wrote.** The person page's standing brief has always been a deterministic
  composition; it now runs on the `summarize` lane as the invocation site
  `person_brief`, alongside the account brief, the prepared questions, the
  company dossier and the meeting plan. The change that matters is not the model
  but what the model is shown: the brief now reads the claims extracted from
  conversations with their verbatim quotes and open/overdue state, what CHANGED
  about the relationship, the moment the page's own ladder selected, and — for
  each recent message — the server's own one-line summary of what was written
  rather than its subject alone. A brief assembled from subjects, kinds and
  directions could say no more than that mail had been exchanged, which is true
  of every contact in the system. Every sentence is still cited or dropped
  whole, the brief is still assembled and cached per reader under that reader's
  own gated read, and a message the reader may not open contributes its date and
  nothing else. A deployment with no model lane, an exhausted AI budget or a
  reply the grounding filter refuses gets the deterministic floor as before, and
  `generated_by` says which of the two wrote what is on screen — that floor now
  leads with what is due and quotes what the last message said, so it is a
  plainer brief rather than a thinner one.

- **"Set the next step" names the step, and the button writes it.** An account
  with an open deal and nothing scheduled raised advice that restated the finding
  as an instruction — and offered an "Add the next step" button the company page
  had no surface to route to, so pressing it did nothing at all. The rule now
  names the step in the words it will be written in ("Agree the next step on
  *Fleet retrofit 2026*", or the account's own name where several deals are open
  and picking one would be a guess) and prepares the exact `POST /tasks` body
  beside it, which the client sends unchanged — so the sentence a rep accepts and
  the task they get cannot come apart. The recommendation also appears on the
  record's **Tasks** tab above the open work, marked as the agent's rather than a
  person's, where a reader who came looking for what happens next will meet it;
  an archived account offers no button, because the write would only be refused.
  Still deterministic, still nothing staged: no task exists until a rep presses
  the button, and the write goes through the same governed endpoint the task form
  uses, audited and undoable.

- **The operational agent budgets stop being called quotas.** One word named two
  unrelated things until the sales quota was retired: the revenue target a
  manager typed, and the volume ceiling that stops an agent reading, writing or
  spending without bound. The second is a safety limit and is unchanged in
  behaviour, threshold and refusal — only its name moves. The Go package,
  types and files are renamed, and two stored values move with them:
  the approval kind `quota_release` becomes `volume_release` and the AI failure
  code `provider_quota` becomes `provider_budget`, both migrated in place. The
  contract already tells a reader to treat an unrecognized failure code as
  "some failure", so a client that has not been updated degrades to the honest
  general answer rather than breaking.

- **Home's test suite is under the 1000-line ceiling again.** It crossed while
  Home gained its weekly-review section, and the split had one honest shape: the
  harness both halves need — the recording fetch stub, the render wrapper and
  the fixtures — is a module now (`home.testkit.tsx`) rather than something to
  export from a test file or copy. A second answer to "what does an unrouted
  read reply with" is exactly what these cases turn on.

- **The Unreleased section carries one list per change type.** Three separate
  `### Changed` sections had accreted under it, each appended by an author with
  no signal that the others existed, so a reader looking for what changed had to
  find all three. Every entry is preserved and unchanged; only the headings
  moved. `make changelog-sections` holds it, reading the change types from the
  file rather than from a list somebody has to maintain.

- **A certification fixture's trigger ref is checked against the shape
  production mints.** The validator accepted any non-empty string, so a corpus
  scenario could drift arbitrarily far from the runner and stay green: when the
  ref grew a seat segment, a fixture kept the old two-segment value and nothing
  failed, because a fixture that describes a different system has no failing
  assertion to notice. The check is now derived from `AgentSpec.TriggerRef`
  itself — a reference ref is minted and compared segment for segment — so the
  day the writer changes shape, every stale fixture fails naming itself.

- **BREAKING (metrics): the sweep gauges no longer carry a `_total` suffix.**
  `margince_sweep_workspaces_total` is now `margince_sweep_workspaces`, and
  `margince_sweep_units_total` is now `margince_sweep_units`. The two `_failed`
  halves are unchanged. Any dashboard or alert reading the old names has to be
  updated; nothing is aliased, because a series published under two names is a
  worse surface than a rename with a note.

  All four are levels — how many workspaces or units a fleet pass currently
  covers, read out of `river_job` at scrape time, numbers that go DOWN — and
  Prometheus reserves `_total` for monotonic counters. The `# TYPE` line always
  said `gauge`; the NAME is what a dashboard author reads, and this one invited
  `rate()` and `increase()` over a level, which render as plausible noise rather
  than as an error.

  Both pairs move together. Renaming one half would have left an operator
  reading `margince_sweep_workspaces_total` beside `margince_sweep_units`,
  which is a worse surface than the convention it satisfies.

  A gate now holds the rule in both directions, over what the exposition writers
  actually emit rather than over a list of names: a gauge may not carry the
  suffix, and a counter must.

- **The schema describes margince.yaml, not just the routing block.**
  `config/ai-routing.schema.json` is gone; `config/margince.schema.json` takes
  its place and covers the whole file — all fifteen sections — with the model
  binding under `$defs.aiRouting`, where `seeds.ai_routing` points.

  The old file was right while a routing FILE existed. Once the binding moved
  into margince.yaml and the database, a schema for one subtree left the other
  fourteen sections unchecked: an editor validating the smallest part of the
  file and saying nothing about the rest.

  **Generated, because a hand-written one would be a second copy of
  `deployconfig.Config`** — and the copy goes stale silently, reporting a new
  field as an error against last quarter's shape. `tools/gen-configschema`
  reflects over the struct the loader decodes into and reads the field
  descriptions from the Go doc comments, so hover text is the explanation that
  sits beside the code rather than a second one free to disagree.
  `additionalProperties: false` throughout mirrors the loader's
  `KnownFields(true)`.

  Two gates hold it, both derived rather than listed: every field the struct
  declares is a field the schema accepts, and every config this repo ships
  validates against the schema this repo ships. The routing shape keeps the
  enum-parity gate it already had, following it into `$defs`.

  The shipped configs now carry a `# yaml-language-server:` line, so an editor
  picks the schema up without being told.

- **The dev stack's Docker Compose project is named `margince`**, not
  `margince-poc-v1`. The old name outlived the repository it was named after.
  A project name namespaces the stack's volumes, so **the first `make db-up`
  after this change starts on an empty database**: the `margince-poc-v1_*`
  volumes are orphaned rather than migrated. They are not deleted — their data
  survives until someone removes them, and a stack brought up with
  `-p margince-poc-v1` still reads it — but the renamed stack will not pick it
  up, so re-seed with `make seed-dev`. `margince-dev` was not an option: the
  frozen poc-era stack left `margince-dev_*` volumes behind, Postgres skips
  initdb on a non-empty volume, and the owner role would then silently never
  be created. `MARGINCE_PG_CONTAINER` still overrides the container name that
  `scripts/migration-baseline.sh` reaches for.

- **The AI routing FILE is gone, format and all.** The binding moved to the
  database earlier; what stayed behind was a file format three DB-less lanes
  still read, and the example yamls that fed them. Those lanes are now TOLD
  their model instead: `worker siteread` and `worker aitask` already took
  `--model provider:model` or `--ai-fake`, so `--ai-routing` was a redundant
  third arm and is gone from both; `make e2e-ai` takes `MODEL=` and `JUDGE=`.
  `ai.LoadRoutingFile` and `config/ai-routing.*.example.yaml` are deleted.
  `ai.ParseRouting` stays — it is what reads `seeds.ai_routing` and the stored
  setting — and so does `config/ai-routing.schema.json`, which still describes
  that shape for an editor validating a seeds block.

  Three things the file had been supplying quietly, which had to become
  explicit rather than disappear:

  **The judge.** The certification runner graded a candidate with a SECOND
  model, and it got that model by having the judge ride the file's own binding
  while `MODEL=` moved only the candidate. Deleting the file with nothing in
  its place would have collapsed the two, and a model grading itself passes by
  construction. `JUDGE=` is now required and the run refuses the two being
  equal before it pays for a single call.

  **The endpoint.** `openai_compatible` fails closed without a `base_url`, so
  the file was the only thing making broker certification runnable at all.
  `BASE_URL=` carries it, on every rung of the ladder rather than the first.

  **The profile.** It is part of a record's identity — its path and its sort
  key — and it is enforced, not a label: `PROFILE=sovereign` with a cloud
  vendor is refused rather than run, through the same rule a parsed config
  meets. That rule is now `ai.ValidateTierBinding`, shared rather than spelled
  a second time in the cert lane.

  Two smaller corrections fell out. The runner binds EVERY tier, not just the
  task's ladder — the router demotes under budget pressure onto tiers a ladder
  does not name, and an unbound one surfaces as "no bound tier can serve",
  which reads like an unsupported task rather than a partial binding. And
  `RoutingConfig.Validate` is unexported again: it was exported for exactly one
  caller, the override path this change replaces.

  The pricing gate moved with its subject. It asserted that every binding the
  shipped EXAMPLES named was priced by `SeedModelRates`; with the examples gone
  it now reads `seeds.ai_routing` out of the shipped Margince YAML files, which is
  what a fresh installation actually boots bound to.

- **The AI routing file is retired: one binding, declared in one place.** The
  installation's tier→model binding has been the `ai.routing` setting for a
  while, but `--ai-routing <file>` still seeded it on any boot that found the
  setting unset — and `seeds.ai_routing` in `margince.yaml` seeded the same
  setting at bootstrap. Two files could plant one installation's binding, nothing
  interlocked them, and nothing said which had. The boot file is the survivor,
  because it reaches every path that creates an installation (the claim flow, the
  file bootstrap and a data reset all run the same seed inside the creating
  transaction) where the flag only ever fired at boot.

  `--ai-routing` and `MARGINCE_AI_ROUTING` are now **ignored, and warn** — the
  flag stays registered so an existing command line does not die on an unknown
  one, and the warning distinguishes the two situations it can be in, because on
  an installation with nothing stored this is the boot where the AI lanes go
  absent. A dev stack is bound by `seeds.ai_routing` in
  `config/margince.dev.yaml` rather than by a per-engineer `config/ai-routing.yaml`
  that `make install` used to seed; `make e2e-ai` now defaults to the **tracked**
  `config/ai-routing.example.yaml`, so a recorded verdict is comparable between
  engineers instead of being about whatever each had bound.

  The file FORMAT stays, and so does its schema: `worker siteread`, `worker
  aitask` and the certification runner read one deliberately, with no database
  open. Those probe a binding rather than serve one, which is the one job a file
  is right for.

  The desktop launcher no longer looks for `ai-routing.yaml` either. It passes
  `--ai-fake` on every boot, which is not the same as forcing the fake on: a
  stored binding outranks that flag, so a user binds real models under
  Settings → AI — with the provider key beside them — and it takes effect without
  a restart, instead of copying a YAML file into the app folder.

- **The twelve settings pages speak the record page's language.** Settings and
  the company record were built from two different card primitives, and that was
  most of the visual distance between them: the record page is `Panel` — an 8px
  radius, a 48px header band with a hairline under it, full-bleed rows, a footer
  band for a figure belonging to the whole card — while settings was `Card`, at a
  12px radius with a 15px/600 title and no band. Across all twelve tabs `Panel`
  was used zero times, `FieldGrid` zero, `FactList` twice. Every settings card is
  a `Panel` now, and the surfaces that had invented their own chrome give it up —
  the company profile alone drew five, including a gradient hero with a 180px
  decorative circle; its stylesheet went from 292 lines to about 130 and none of
  what remains is a surface. The pages also gained a reading measure (a single
  text input on the Account tab was ~950px wide), one vertical rhythm (nineteen
  inline margins and ten `card-stack` classes predated `.settings-stack`'s gap
  and margins do not collapse in a flex container, so real gaps were a mix of 16
  and 32 within one tab), and headings that are headings — Tailwind v4 preflight
  is live with no global `h1`-`h6` rule, so a bare heading rendered at 14px/400,
  identical to body text, which is what the Integrations lead card and the
  company profile hero were doing with their titles.

- **The record page's honest-state vocabulary left the screen it was written on.**
  `SurfaceState` — ready, empty, withheld, unavailable, loading, unsupported,
  failed, stale, partial — lived in `company360.tsx` and seven other screens
  imported it from a screen file. `Eyebrow` was spelled out five times across
  three stylesheets with no two agreeing. `PanelPlate`, `Panel`'s accent tone and
  its actions band were all being reached into from a screen sheet.
  `SectionHeader` learned level 3, because six nested sections were minting a
  second `h2` inside an `h2` for want of a way to say what they meant.

- **Opening a settings page is a READ, and every entry now asks like it.** Each entry's predicate was a WRITE grant, because each was written to
  answer "can you *use* this" — and measured against the live API a read-only
  seat was hidden from eight of the eleven entries the server answers 200 on,
  including three surfaces that were ungated routes of their own before the
  merge. One rule now covers all of them: the entry opens if the principal may
  read any part of it, and the write affordances inside say for themselves who
  may use them. A read grant is still a real question rather than a formality —
  a role edited to drop `custom_field:read` loses the Data model row, which
  `true` could never express. The consequence, stated: on a freshly seeded
  installation every role reaches eleven of the twelve entries and only
  Maintenance narrows, so what distinguishes seats has moved inside the pages,
  where cards state their own denials.

- **Connections split in two, and the entry with no predicate disappeared with
  it.** One Organization row used to hold both a reader's own mailbox and
  LinkedIn network *and* the installation's contact-data credential, webhooks and
  HubSpot mirror — which is exactly why it could not be gated: any honest
  predicate took a personal task away from whoever it hid it from. *You →
  Connections* now holds the three per-user surfaces (every one reads a `/me` or
  caller-scoped seam) and *Organization → Integrations* the four
  installation-wide ones, each with a predicate of its own. The seam was never a
  missing group; it was one entry belonging to both. The connector OAuth callback
  keeps its `#/settings/connections` return route, because the connectors are the
  half that stayed; the system-of-record chip follows the mirror to
  `#/settings/integrations`.

- **The member roster is no longer admin-only, and People opens on it.**
  `GET /users` answers 200 to any authenticated principal, and who is on the team
  with what role is not an admin's private question — so every seat reads the
  list, while inviting somebody and changing a role or status stay the admin's
  and withhold themselves. A role nobody may change reads as text rather than as
  a picker that could only be refused. The page also used to open on an empty
  invite form: three blank fields ahead of the answer, for a task most visits are
  not about.

- **Three settings pages were ordered against the reader.** AI opened on spend
  and buried the automations that *cause* it four screens down, past a price
  table — and for manager, rep and read_only the two spend cards are withheld
  anyway, so the page opened on a price sheet; it now reads automations → spend →
  prices → the per-call trace last, that trace being a debugging instrument
  rather than a setting. General claimed in a comment that the base currency and
  its rate sheet were adjacent while the company profile sat between them; they
  are adjacent now. Four card titles stopped repeating the heading above them
  ("You" inside *You*, "Organization" inside *Organization*, "Capture" on
  Capture), "Your agents" dropped a possessive the group heading already carried,
  and "Voice" became "Writing voice" — in a product with mail capture, a bare
  "Voice" reads as call recording.

### Added

- **An introduction is something the product records, not something a rep
  remembers.** The Network tab already answered "who here can reach this
  contact"; asking them was a conversation that happened elsewhere and came
  back as a memory. `intro_request` records the ask, the colleague's bounded
  answer and what came of it — five endpoints, a requester's composer, and a
  decision surface of the colleague's own.

  The lifecycle is built so it cannot overstate an outcome. Approving a
  name-drop permits a mention and nothing more, and no path turns it into a
  handshake: the ask completes as `name_dropped`, and WHICH outcome that is
  gets read from the state the ask was in rather than from anything the caller
  sends. The domain row, the event and the audit after-image all say the same
  word, because a dispute about whether an introduction happened is settled
  from the trail.

  `replied` is unreachable by any person. The contact answering is observed
  from captured activity, so no button produces it.

- **A standing overnight grant says when the agent has outgrown it.** A grant
  mints the scopes the agent needed at the moment the rep answered; when the
  agent later gains a tool needing a wider scope, every already-minted passport
  is short. Nothing failed — the runner degrades the unfunded tools before the
  first model step, so the rep's overnight work simply stopped, with no error,
  no expiry and no prompt, and the grant still reported a usable credential
  because liveness was all that was checked. `credential_funds_agent` is the
  second renewal cause, computed at request time from what the build would mint
  today, and the Settings card now offers renewal on it with copy that says the
  authority was outgrown rather than that it expired.

- **Foundation (WP0)**: the full core data model as reversible
  migrations — RLS (`ENABLE`+`FORCE`, deny-on-unset) on every tenant
  table, composite same-workspace foreign keys, append-only audit log,
  transactional event outbox, and the core/custom migration namespaces.

- **Contract pipeline**: `api/crm.yaml` (OpenAPI 3.1) → generated types
  + chi server; every operation mounted; regeneration drift is
  merge-blocking; every `crm.yaml` operation is implemented.

- **Auth and tenancy**: workspace bootstrap, Argon2id login, opaque
  server-side sessions, five seeded system roles, object RBAC +
  own/team/all row scopes, and the read/full seat ceiling.

- **Core CRM spine (WP1)**: people, organizations, leads (with
  promotion), pipelines/stages, deals (stage-semantic advance, FX freeze
  at close), activities and polymorphic links, two-record merge,
  lists/tags, relationships/partners, deal stakeholders, scheduling.

- **Event bus**: the events.md envelope over a transactional outbox →
  Redis Streams relay, consumer groups, and at-least-once dedupe.

- **Governed agent surface**: Agent Seat Passports, the remote MCP
  connector at `/mcp` (OAuth 2.1 + PKCE, dynamic client registration,
  refresh rotation; the A1 stdio server is retired, SCR-9), the 🟢/🟡
  autonomy tiers enforced below the transport on MCP and REST alike, and
  the approval engine (stage → human decision → single-use redemption).

- **Consent lends a passport**: `GET /oauth/authorize` redirects to an SPA
  consent screen where the human selects one of their own existing agent
  passports; the connection receives exactly that passport's scopes,
  carried by a *new* grant-bound passport, so revoking a connection never
  touches the human's own credential. Deny
  answers the client `access_denied`. A human with no passport is guided
  to mint one and brought back to finish connecting, which means
  `claude mcp add` no longer completes unattended for a fresh account.
  The lend is recorded in the audit trail. Deactivating a member ends
  their consents that no client redeemed yet, alongside the connections
  that already exist — so reactivating them later cannot hand out a
  connection on authority an admin took away.

- **AI surfaces**: model routing (the `ai.routing` setting — BYOK cloud via
  the native Anthropic / OpenAI / Gemini adapters or the generic
  `openai_compatible` wire, local Ollama / vLLM, an offline fake), the
  Surface-B runner + scheduler, search (FTS + pgvector hybrid), capture
  connector seam, cold-start read-back.

- **Model certification** (ADR-0074): a hand-authored fixture corpus
  driven through each site's own production request builder and
  validator, scored by a pinned judge and committed as a JSON record —
  plus `make ai-probe`, which runs one site against operator-supplied
  input through the same case.

- **The job contract** (`api/jobs.yaml`): every River job kind is declared
  before it is written — a kind not in the file does not compile, and a
  kind with no chosen timeout fails generation rather than running on
  River's silent default. Each kind declares its role: a *dispatcher*
  enumerates the fleet and enqueues, a *workspace worker* does one
  tenant's work and fails its own job row. Job args name rows and never
  carry content, so Art. 17 erasure reaches an in-flight job through the
  row it names.

- **Fleet observability** (OPS-MET-2): `/metrics` carries the job-runtime
  section, `GET /v1/admin/job-health` is the same table read for an admin
  (human-session-only, workspace-scoped, failure reasons drawn from a
  vetted vocabulary rather than the raw error column), and
  `cmd/worker --observe-addr` gives the worker its own `/healthz`,
  `/readyz` and `/metrics` — which *process* is wedged is a question the
  fleet-wide gauges structurally cannot answer.

- **Messaging channels**: a workspace-level Telegram bot binding
  (`/channel-connections`) with pull ingress — the installation
  long-polls, so it needs no public address and no inbound route — and a
  governed reply (`POST /activities/{id}/send-message`) whose recipient
  is the channel identity of the person the conversation is with, never
  named by the caller.

- **Outbound mail**: a Gmail send surface bound to the same consent as
  capture, staged as a durable `comms_outbound` row, re-checked against
  the staging human's live seat at transmit time, consent-gated, and
  keyed on the identity the provider stamped rather than the one we asked
  for — Gmail rewrites `Message-ID`, and bookkeeping keyed on the id we
  requested loses the receipt.

- **The relationship graph**: activity participants projected into an
  interaction edge, answering who on our team already knows a contact —
  with per-user warmth (never summed into a workspace score), deal
  coverage and its risk rules, and a LinkedIn `Connections.csv` import
  whose rows are graph substrate: invisible to search, lists, people
  screens and agent record tools, and private to their owner.

- **Company record page**: one gated 360 read behind the whole page, a
  per-viewer account brief that degrades to a deterministic summary with
  no model lane, Ask, record-derived next-step suggestions that each
  carry their why, a dwell-gated visit baseline, and the one-hop
  connections graph.

- **Overlay mode (HubSpot as system of record)** and the **ADR-0071
  overlay→native cutover**: mirror-backed reads behind the frozen
  datasource seam, incumbent-first write-back on `Update`/`Archive`, a
  conjunctive preflight that seals the mirror when green, and a
  confirm-first flip behind a typed phrase. Both flip operations are
  human-only — a one-way, estate-wide cutover must not collapse to a
  single approval click. Reversibility is reconstruction from the
  pre-flip bundle, not rollback.

- **Supply chain**: three source-tree SBOMs (CycloneDX + SPDX 2.2.1 +
  SPDX 3.0) generated from a clean export of HEAD, normalized to one file
  set and parity-gated, license-gated against an explicit allowlist, and
  keyless-signed on `main` from a job isolated from all PR-controlled
  code — a keyless signature lands permanently in a public transparency
  log and cannot be retracted.

- **Self-hosting materials**: deployment-target-agnostic
  `Dockerfile.{api,web,worker}` (all non-root), the entrypoints, the
  one-time `db-bootstrap.sql` that creates the two non-superuser roles
  `FORCE ROW LEVEL SECURITY` requires, and a runbook
  ([docs/deployment.md](docs/deployment.md)).

- **Non-production data reset**: `POST /v1/admin/reset-data` wipes
  workspace data back to the bootstrapped state behind a four-gate chain
  (non-production posture → human-only → `admin` role → typed
  organization-name confirmation). In production the operation does not
  exist: the posture check runs before auth, so it 404s rather than 403s.

- **Embedding drift self-heal** (ADR-0069 §3a): a periodic worker sweep
  re-embeds entities whose embed event the at-least-once bus lost, with
  no operator confirm; the preview → confirm reindex remains solely for
  a changed embed binding, and the ops banner fires only on that case.

- **GDPR arm**: per-purpose consent with default-deny suppression,
  retention evaluator with DE (GoBD) statutory floors, legal hold,
  Art. 17 erasure with re-capture suppression, Art. 15 SAR assembly.

- **Web UI**: login/bootstrap, people, leads, deal board, timeline,
  search, reports, privacy inbox — the Vite/React app in `frontend/`, a
  standalone static build served separately from the API.

- **Quality gates**: golangci-lint + depguard, go-arch-lint, tree-derived
  architecture/schema/license fitness tests, contract drift-lint, and a
  real-Postgres integration lane covering the security invariants.

- **Craftsmanship gate, strict** (ADR-0045): `craft static` now fails on
  MAJOR findings as well as BLOCKER ones, in the pre-push hook (diff-scoped)
  and in CI's `craftsmanship` job (whole tree). MINOR stays advisory. Test
  files carry their own size ceilings — 160 body lines / 1000 file lines,
  against 80 / 500 for product code — because a long scenario test that sets
  up, acts and asserts once is not the god-function smell the product
  thresholds hunt. Arming this meant clearing the whole backlog first: every
  swallowed error, bare `any` in a signature, boolean-trap signature,
  assertion-free test and over-long function in the tree.

- **Company 360**: `GET /organizations/{id}/360` serves the whole company
  record page in one transaction — profile, contacts with §4 relationship
  strength and per-purpose consent, deals, timeline, tags, list
  memberships, decidable approvals, open next steps, and what changed
  since the caller last visited. Authorization is per section: a section
  the caller may not read is omitted and named in `sections_omitted`,
  never returned empty. `POST /organizations/{id}/view-ack` is the
  explicit, human-only, monotonic visit baseline those counts run against.

- **Company page verbs**: the record page opens a deal on the company it is
  showing (open stages only, the organization implied rather than asked for),
  and applies a tag or a list membership by typed name, creating either when
  the name is new. Each verb renders only on a section the caller's grants let
  them read, and an already-applied tag or membership is treated as the asked-
  for state rather than reported as a collision.

- **Company connections**: `GET /organizations/{id}/graph` serves the
  account's one-hop neighbourhood as nodes and edges the client lays out —
  its contacts by employment (weighted by §4 strength), its open deals and
  the stakeholder seats on them, its parent, children and partner
  companies, and which contact the active signal's warm-intro path routes
  through. Authorization is per group, the same posture the 360 takes for
  its sections; node selection is deterministic and `dropped_count` says
  what the caps left out. The rail's connections card draws it as an ego
  diagram over a keyboard-reachable node list, and the diagram is
  decorative — everything it shows is in the list.

- **The installation's own company is not one of its prospects**
  (ADR-0082/A127). The anchor organization is excluded by default from the
  surfaces that answer "which companies are we selling to" — the
  organization list, lexical and vector search, dynamic segments and their
  exports, duplicate detection, and signal candidate resolution — with
  `include_anchor` as the opt-in on the NATIVE list, shaped like
  `include_archived`; an overlay-mode list refuses it with 422, because the
  mirror holds the incumbent's accounts and the anchor is a native row that
  is never among them. It stays reachable by id everywhere, and stays
  deliberately available where naming it is the point, such as recording
  that a person works there. `is_anchor` is on the wire so a client can
  tell it apart, and the governed agent surface learns the id through its
  company context, since the company operation is human-only. Archiving it
  or merging in either direction is refused in the schema as well as in the
  service: losing the anchor makes the company read answer not-found, and
  the application reads that as a workspace that was never configured.

- **An administrator can see and change the workspace's own email
  domains**: `GET`, `POST` and `DELETE` on `/capture/email-domains`,
  admin/ops and human-only, since the set decides which mail the
  installation may hold at all. Adding a domain IS the human vouching for
  it, so it stores verified and takes effect on the next message; adding
  one a connected mailbox already contributed confirms that candidate
  rather than failing. The list reports what the company profile claims
  separately from the registry, because only the second is editable here.
  The screen states what removal does and does not undo: capture resumes
  from that point on, and mail skipped meanwhile is never offered again by
  any mailbox.

- **Custom fields and tags are filter vocabulary**, so a dynamic list, a
  saved view and a filtered export can all select on them. A custom field
  stays filterable after it is retired: retirement is a status change and
  never a column drop, so an already-saved segment naming a retired field
  keeps returning the same rows rather than silently widening to every
  record. A tag filter is a correlated `EXISTS` over the polymorphic
  `taggable` join, which makes "does not carry this tag" and "carries no
  tags at all" both expressible, on every record type the join admits —
  people, organizations, deals, leads and projects. The operator set is
  unchanged and no operation gained a parameter; the three surfaces
  resolve one vocabulary through one method, so what a filter may say,
  what it selects, and what an export of it contains cannot disagree.
  Authoring such a filter in the product still needs the Filters & views
  screen, which does not exist yet.

### Fixed

- **A replayed body re-checks every record it names, not only the one it
  replays by.** `POST /people/quick-capture` answers the person created plus the
  `organization_id` they were attached to; the replay gate probed the person and
  handed the rest back, including an employer id the caller may since have lost
  sight of. `POST /leads/{id}/promote` and `POST /leads/{id}/demote` have the
  same shape — the third was found by widening the gate rather than by reading
  the code. Every record reference a wrapper body carries is probed now, and a
  gate derived from the contract fails when a response schema grows a reference
  that nothing re-checks.

- **A refused API key is no longer reported as six broken use cases.** The
  `e2e-llm` lane guarded against an empty transcript but not against one the API
  refused: a `401` produces an init line and an error result, which the checker
  scored as a scenario that called nothing and said nothing. All eighteen runs
  of a lane failed that way, every scenario was recorded as failing its
  criteria, and the verdict named the product — nothing outside the transcripts
  said otherwise. A run that never reached the model is now named as one and
  stops the lane, and a run that answered badly is still a finding.

- **Outbound mail is written in the installation's own language.** Every message
  the product sent was hard-coded English while the screens are translated three
  ways. The weekly retrospective is the one that shows: it arrives unasked every
  Monday and it is the product talking to a rep about their own week, so a
  German-speaking rep read their Home panel in German and then an English
  summary of the same numbers. One catalog now serves all three senders — the
  password reset, the invitation and the weekly — keyed off the installation's
  base language, and the weekly's labels and counts are the panel's own strings
  rather than a second translation of them. A language the catalog has no copy
  for is sent in English rather than not sent, and a gate fails when the
  contract admits one the catalog does not carry or leaves a line of it empty.

- **The dev tooling's psql path and its DSN name one database.** They were two,
  with nothing tying them together: ad-hoc statements went through `docker
  compose exec`, and the api, worker and migrator connected through a published
  host port. `infra/docker-compose.dev.yml` pins one project name, so every
  checkout on a machine resolves to the same project — and a seed run from a
  second checkout landed in the first one's container while the api served the
  migrated database on `:15432`, untouched. That run failed loudly because the
  wrong database was empty; the ordinary case is a migrated one, where the seed
  succeeds and writes another checkout's data. There is one way to name the
  database now, the DSN's own port, so the statements land where the binaries
  are looking. A DSN pointed at another checkout still reaches that checkout —
  it is the DSN's job to say which database — but the two paths can no longer
  disagree about which one that is without saying so: when no container
  publishes the port, or more than one does, the command refuses and names what
  it found.

- **A technology lane announces an update only when something moved.** A
  completed lane is worth an audit row whatever it found — it is what says when
  the record was last looked at, and what lets a technology the company dropped
  leave — but it emitted `organization.updated` even when nothing appeared,
  moved or went. Both shapes are common: most companies' sites declare nothing
  this build recognises, and most that do declare the same thing they declared
  yesterday. Between them the lane produced a no-op event on nearly every read,
  which every subscriber had to learn to ignore. A lane that REMOVES a stale
  fact still announces, because that is a change.

- **A refusal the server has settled is asked once, not three times.** The
  retry policy retried every status at or above 500, which swept in `501 Not
  Implemented` — a final answer, not a failure the server may recover from. An
  installation with no embeddings model bound answers 501 to the reindex status
  by design, so every reader of that surface produced three requests and three
  console errors for one settled answer, which an operator scanning for a real
  fault reads first. 501 and 505 are excluded now; their neighbours are still
  retried.

- **A provider name the contract cannot carry is refused at registration.** The
  API publishes `Provider` as a pattern-constrained string — a pattern rather
  than an enum, because which providers exist is a deployment fact — and nothing
  enforced it where a name enters the system: the registry asked only that a
  name be non-empty and unique, and the database CHECK that pinned it to one
  vendor is gone. An adapter called `Acme-Data` would have registered, written
  rows, and been serialized into responses that violate the published schema,
  with the failure surfacing at a client that validates rather than at the
  author who chose the name. A gate compares the registry's rule against the
  contract's own, so the two cannot drift. The same rule now holds at the
  messaging-transport registry, whose names the contract publishes the same way
  under `ProviderRef`; three test fixtures were already named with hyphens.

- **A provider run's audit actor names the provider it is for.** Every run was
  audited as `connector:surfe`, whichever vendor it actually belonged to,
  because the workers that execute a run hold a run id and the poll sweep drains
  many at once — the name could only be a guess, and it was correct only while a
  CHECK constraint made a second provider impossible. Those constraints are
  gone, and the claim rows already derived their provenance from the run's own
  provider, so the audit log and the evidence on the record would have named
  different vendors for one purchase. The actor is now narrowed to the run's own
  connector inside the module that reads it.

- **A partial payload took down the whole application.** `settings/maintenance`
  dereferenced a contract-required field on `/admin/job-health`, and the only
  error boundary was app-level, so one card's throw cost the reader the rail and
  the navigation along with the page. The payload is rejected at the query
  boundary rather than defaulted: an absent `kinds` rendered through `?? []`
  draws the *idle* state, which tells an operator the background system has
  nothing queued and nothing failed — a claim about the installation the
  response never made. `CardBoundary` now contains a card that throws.

- **`Switch` dropped `aria-checked`** whenever `checked` arrived `undefined`,
  because React omits an undefined attribute. A `role="switch"` with no
  `aria-checked` announces no state at all, and the stylesheet keys the knob off
  the same attribute, so the track drew as off with nothing saying so.

- **A `Badge`'s contrast was a property of its ancestor.** The tone tints are
  translucent by canon, so `--success` on `--successBg` measured 4.54:1 over a
  panel and 4.05:1 over a recessed plate — four hundredths of headroom on one
  surface and a WCAG AA failure on the other, same component, same tokens. Each
  tone now composites over an explicit surface. The canon values are untouched.

- **`SegmentedControl` could not wrap.** An unwrapped `inline-flex` sets its own
  minimum width and pushes its container: the company record's seven-tab strip
  measured 543px inside a 374px column at 390px, which was the entire horizontal
  overflow of the page every other surface is modelled on.

- **Small text sat on tokens that do not carry it** — `--textMuted` at 1.54:1 on
  a 13px form label, `--textTertiary` at 2.52:1 in nine rules. `tokens.css`
  already names `--textMeta` the AA small-text role.

- **The gates could not see any of it.** The 390px sweep asserted on
  `document.body`, which `.main { overflow: hidden }` makes structurally
  incapable of growing — measured across twelve tabs the body read 0 while the
  shell's own scroller overflowed by 273px. Seven of the twelve settings tabs
  were in neither sweep. And both sweeps passed on the crashed maintenance page,
  scoring zero violations and zero overflow on a page that had rendered the error
  boundary. The sweep now measures the elements that actually scroll, covers all
  twelve tabs, and asserts the shell survived before measuring anything.

- **Ten Storybook stories showed a state they were not named for.**
  `aiusage`/`aicalls` never stubbed `GET /me`, so the fallback made `useMe` throw
  and every capability hook failed closed — each of those stories captured the
  `/me`-error branch. The shared stub now refuses to guess a session rather than
  answering with a body that reads as a malformed one.

- **Absent, disabled and withheld are now decided by CAUSE, and it is written
  down** (`frontend/src/design-system/README.md`). A surface a permission denies
  says so; a precondition the reader could fix disables the control and states
  what would make it live; only "does not apply" is absent. Two cards returned
  `null` on a denial while their neighbours on the same page explained
  themselves — so for an ops seat the Privacy page showed the consent registry
  working, retention silently gone and the subject queue saying why. An absent
  retention card does not read as "not yours"; it reads as "this installation
  keeps nothing". Both now state the denial and still ask the server for nothing,
  because the answer was already known. The installation settings card disabled
  three fields with no reason at all, under a comment claiming parity with the
  card that does it right.

- **The scroll position survived a route change.** The document never scrolls
  here — the content column does, and it is the same element on every route, so
  it carried the last page's offset into the next one. Reading a scrolled AI
  settings page and opening another entry landed the reader partway down whatever
  the next page happened to hold at that offset.

- **No skip link existed anywhere** (WCAG 2.4.1). Every page put the brand,
  search, up to twelve navigation rows, More, the settings door and the account
  menu ahead of the content, and a keyboard reader walked all of it again on
  every page. It is a button rather than a fragment link because the app
  is hash-routed and a fragment link would navigate.

- **Entering the settings level dropped focus to `<body>`.** The walk out of a
  section was pinned and the walk in was not — the guard that was supposed to arm
  it fires only for a row with children, and nothing in the tree gives a row
  children. Two comments claimed both directions were covered.

- **A modal returned focus to whatever opened it, even after the mutation
  removed that element** — passport revoke, member deactivate, connection end and
  DSR transition all landed on `<body>`. `Modal` now takes a `returnFocusTo`
  resolver evaluated at restore time, and checks the opener is still connected
  before reaching for it.

- **Every card's load failure was announced to nobody**: `QueryStates`' error
  branch carried no live region and its skeleton no busy state or spoken name.
  One fix covers most surfaces in the product, plus the shared create/edit and
  archive-confirm errors.

- **Two WCAG Level A defects on the AI attempt trail and the tool console.** A
  row's expand affordance was a `<tr onClick>`, unreachable by keyboard; the tool
  console dimmed unreachable rows to `opacity: 0.4`, taking the "scope not
  granted" caption that is meant to be the text equivalent below the AA contrast
  floor — while the passport list 150 lines above it had chosen a strikethrough
  over dimming for exactly that reason. Every credit-pool meter was also
  announced as an anonymous number triple (`label=""`).

- **Heading navigation landed on sections with nothing inside them.** Pipeline
  names, provider names and block titles were styled spans and divs; two dialogs
  opened with a raw `<h3>` under the page's `h1`.

- **Passport scope checkboxes rendered the raw protocol words** (`read`,
  `draft`, `write`, `send`, `enrich`), untranslated, on the page where a human
  decides what to lend an agent — where "write" and "send" read as near-synonyms
  until one of them names the mailbox.

- **Twenty-one German strings addressed the reader as *Sie*** against `de.ts`'s
  own mandated informal *du*, every one of them on a Connections surface — the
  register flipped exactly where a rep does their own personal setup.

- **Retention wrote on flip through a `Checkbox`.** The design system reserves
  `Switch` for a control that *is* the action; the flip also shared Save's
  mutation, so pausing a policy collapsed the edit panel — discarding the panel,
  the focus, and the operator's place — under a comment claiming it existed so
  they would not have to save mid-edit.

- **The 390px and axe sweeps covered two settings pages of twelve**, both of
  them short ones, so the widest page in the product went unmeasured. Data model,
  Integrations and People are swept now.

- **`#/settings/capture` told the reader to change company domains elsewhere
  and gave them no way there**, on a card whose whole point is that it cannot be
  edited in place.

- **Settings had 24 doors; it has 12 entries.** Fifteen tabs plus nine
  routes outside them collapsed into eleven and then split back to twelve, in two
  groups: Account, Writing voice, Agents and Connections under *You*; General,
  People & access, Integrations, Capture, Data model, AI, Privacy & audit and
  Maintenance under *Organization*. What merged, merged because it was one subject
  all along — the installation and the company profile describe the same
  organization, currency rates belong beside the base currency they
  convert to while model prices belong beside the runtime they price, and
  two different surfaces were both called "Capture". Four screens that
  were reached only by a card whose job was to send you there are now
  content on the page that owns them (`#/custom-fields`, `#/products`,
  `#/offer-templates` and `#/automations` are gone as routes;
  Automations' primary-nav slot went to the dedupe queue, which had no
  address of its own outside a home digest card). `#/design` is deleted.
  `GET /admin/job-health` shipped with no UI at all and now has one, on
  Maintenance beside the search-index rebuild and the danger zone. **No
  seat lost a surface it could use**: a merged entry opens on the union of
  what its parts asked for, never the intersection, and the parts that
  are narrower than their page gate themselves inside it. The connector
  OAuth callback's return route moved with the page it lands on: the
  server redirects to `#/settings/connections`, which is a hard-coded
  string in `internal/compose/connectors_outcome.go` and the one part of
  this change that a rename alone would have broken silently.

- **Every dropdown in the product is the design system's, not the
  browser's.** `Select` is a button trigger plus a portalled listbox with
  the full keyboard contract (arrows, Home/End, typeahead, Escape without
  committing, focus back to the trigger), so the option list takes the
  product's tokens instead of the platform's — including inside a
  scrolling toolbar, where it anchors to the trigger and flips when the
  room below runs out. Callers pass `options` and receive the VALUE in
  `onChange`; a `<select>`, `<option>` or `<optgroup>` anywhere under
  `frontend/src` outside that one component now fails
  `make frontend-check`. `frontend/src/design-system/README.md` is the
  catalog to read before hand-rolling any control.

- **The sign-in screen is two halves of a page, not a card in a pane.** The
  identity region runs full-bleed and is divided from the form by one
  hairline; the wordmark sits in the page's top-left corner on the split
  layout and above the form when the layout stacks; each half reads down its
  own centre line, and both carry the same inset at every width above the
  phone. The form is a single 400px measure — heading, provider buttons
  (stacked full width, so every way in presents the same target), fields,
  locale row and fine print — with the fields the one thing that stays left,
  because a label centred over a line of typing points at nothing. On phones
  (≤560px) the identity region drops **entirely**: the sphere, the limits and
  the AI's own sentence with them, because the form is the only thing that
  screen is for. That leaves the phone surface disclosing nothing about the
  AI behind the installation, which is a deliberate departure from
  ADR-0076 Decision 1 at that width, tracked as issue #562; every wider
  layout still makes the disclosure in full.

- **The sign-in screen's entry animation belongs to the page load.** The
  staggered fades and the typed statement ran again on every React remount
  of the surface, which reads as the page reloading under the reader. The
  choreography is now marked spent on the document once it has run its
  course, so a remount renders the surface already arrived while a real
  page load still gets the introduction.

- **The Core holds its position.** The sphere's 11-second vertical drift is
  gone everywhere; it still breathes, and the beat is still what carries
  its state.

- **The Core goes still while the window does not have focus.** Both halves
  of it — the WebGL liquid and the CSS rhythms (breath, sheen, halo, feed) —
  stop off one document-level `focus`/`blur` signal and resume with it, so
  a Core behind another window costs nothing and the sphere and its glow
  can never disagree about whether it is moving.

- **The passport cap an operation spends comes from the contract.** Every
  `x-mcp-tool` annotation now declares its `scope` alongside its tier, and
  the REST agent gate spends that declared cap instead of a hardcoded
  `write`. Scopes are exact membership, so `write` does not imply `send`
  or `enrich`. A passport minted with `read`+`write` and no
  `enrich`/`send` is now **refused** `POST /organizations/{id}/enrich`,
  `/deep-read`, `/coldstart`, `POST /offers/{id}/send` and
  `POST /overlay/reconcile` with `scope_exceeded`, where before it was
  admitted. Re-mint with the caps you mean to grant; the per-tool cap is
  tabled in [docs/reference/agent-tools.md](docs/reference/agent-tools.md).

- **Connected agents are listed apart from passports you minted.**
  `GET /v1/passports` carries a `connection` object on grant-issued rows
  and groups the list one row per connection rather than one per
  credential, so a connection appears once however many times its
  credential has rotated. Revoking a grant-bound passport ends the whole
  connection, not just the current credential.

- **Language and theme moved into the account menu.** The top bar carried
  an icon button for each beside the avatar; both are this person's
  preferences rather than screen actions, so the bar is down to search and
  one account affordance, and the menu reads Settings · Language · Theme ·
  Sign out with the two preferences stating what they are set to. Changing
  one keeps the menu open, so the theme you pick is visible from the
  control that picked it, and dismissing the menu hands focus back to the
  avatar. The language row is the three-locale menu itself, nested: one
  Escape closes the language list, the next closes the account menu.

- **One orb in the product.** The agent panel at the sidebar foot drew a
  CSS lookalike of the Core because the real primitive held a render loop
  for the whole session. The Core now costs what it displays — it draws at
  the size it is shown at, spends its 24fps budget on a timer instead of a
  callback per display refresh, and stops entirely on a hidden tab or an
  off-screen canvas — so the shell shows the same sphere as sign-in and
  onboarding, and the duplicate is gone.

- **AI model routing is now per-engineer**: the working dev config moved
  from a committed `backend/ai-routing.yaml` to a gitignored
  `config/ai-routing.yaml`, seeded from `config/ai-routing.example.yaml`
  by `make install` / `make dev`. Engineers bind their own local models
  without touching a committed file; the annotated template stays the
  parse-guarded source of truth.

[Unreleased]: https://github.com/margince/margince
