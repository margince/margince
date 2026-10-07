# Changelog

All notable changes to this project are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Releases are cut by pushing a `v*` tag, which publishes a GitHub release with
the desktop bundles attached; see
[docs/how-to/cut-a-release.md](docs/how-to/cut-a-release.md). The constellation
dist release is versioned separately, on the `YYYY.edition.bugfix` scheme.

## [Unreleased]

### Added

- AI provider health. A provider that is out of credit, rejecting its key,
  unreachable or degraded is marked on Settings → AI models and in the new
  **AI provider status** card under System health. A blocked provider is skipped
  with no call and no charge. Mail and enrichment wait instead of spending their
  attempts, and a request you make yourself fails fast with a message to contact
  the system administrator. The call that first meets an outage is refunded and
  names the reason, read from the vendor's reply text as well as its status code.
  A successful key test clears the provider's state at once. The API and the
  worker share the status through Redis (the worst status wins; without Redis each
  process shows only its own), so Settings shows an outage only the worker saw.
  Failed probes are refunded, and one runs at a time. A vendor's "does not have
  permission" refusal does not mark the key rejected. Settings → AI shows website
  reads, account scans and voice builds waiting for a provider as their own
  "Waiting for the AI provider" number, apart from work waiting on the allowance,
  and they resume by themselves. `worker reopen-parked --from … --to … [--dry-run]`
  reopens the sender questions and company enrichments an earlier outage parked.
- Model prices sync themselves. Once a day (and on **Refresh model prices**) every
  provider with a usable key is re-priced from models.dev or OpenRouter. Newly
  listed chat and embedding models the catalogue prices are added, and a price you
  set by hand is kept. The new **Model prices** card under Settings → AI shows when it
  last ran and what changed, and can turn the daily sync off.
- `web.security_txt` in `margince.yaml` publishes an RFC 9116
  `/.well-known/security.txt` naming the operator's security contact. Unset, the
  path is a 404. Route it to the api by its exact path.
- `GET /v1/status`: an anonymous, fixed-body reachability probe for external
  uptime monitors. It does no dependency work; `/healthz` and `/readyz` stay
  internal.
- Visual analytics for sellers and managers: bookings trends, pipeline
  stages and age, SDR outcomes, target attainment, forecast composition and
  captured movement, with permission-checked evidence and CSV export.
- Saved report revisions, weekly/monthly schedules and dated editions retain
  their chart data for later comparison. Shared metric definitions, explicit
  scoped targets and the `read_reporting` MCP tool use the same reporting engine.

- Mail history: setup and Settings share a dropdown through ten years, show the
  preview start date, and qualify capped message and cost estimates.
- Undo reaches what Margince did on its own. A record it created can be
  archived again, and an archived contact, company or deal can be brought back.
  A lead it promoted can be demoted, and the fields a mail signature or a website
  filled on a contact can be cleared. Each is one **Undo** on the record's
  history and on the "Since your last brief" panel, which now lists what was
  created and archived, one line per job, kind of record and day. A decision
  waiting there offers **Decide** instead of saying it cannot be undone.
- Mailbox imports report where their time goes on `/metrics`: every Gmail API
  call by op and result, each message's fetch, parse, transaction and
  follow-up time, pages and waits by cause, and the fleet's runs and progress.

### Removed

- **A company no longer carries a `classification`.** Use `lifecycle` (where an
  account stands with us, one value at a time) and `relationship_types` (what the
  company is to us, possibly several things at once). Nothing wrote the old field,
  so every unassessed account showed the default `prospect`. The API stops
  returning it, the company list stops offering it as a filter, and both
  generated clients drop it. A segment already written against it keeps
  evaluating, but no surface offers it for a new one.
- **The backfill stops reporting `dedupe_candidates`.** The counter was never
  written and always read zero. The column stays; the wire field is gone.

### Changed

- Analytics uses one reporting implementation across the web, API, MCP and workers.
  The old performance screen and rollout switch are removed. Existing
  `analytics.performance_enabled` configuration is accepted but ignored.
  Installations that disabled reporting after a pilot must pause schedules before
  upgrading to avoid resuming enabled schedules; see [the reporting upgrade guide](docs/how-to/operate-reporting.md#upgrade-from-a-flag-gated-installation).

- **`/metrics` on the api is closed by default.** It requires `--metrics-token`
  as a Bearer credential. A deployment whose scraper discovers its targets by
  annotation and cannot carry one sets `--metrics-access=open`, where the port is
  already contained. An installation that scraped the api's `/metrics` without a
  token must set one of the two on upgrade, or its scrapes answer 401.

- **The web tier answers 404 for missing files.** A path with a dot in
  its last segment, or a dot-segment such as `/.env`, gets a plain-text 404
  instead of the app shell under a 200. Extensionless app routes are unchanged.
- The api and the web tier add `Permissions-Policy` (every powerful feature
  denied except `clipboard-write` for the app's own origin),
  `Cross-Origin-Opener-Policy: same-origin` and
  `Cross-Origin-Resource-Policy: same-origin`, and nginx no longer sends its
  version. `robots.txt` refuses the common AI crawlers by name.

### Fixed

- **The contacts-without-consent retention rule acts again** on older installations.
  An installation seeded before the record was renamed kept the rule under the old
  `person/no_consent_no_deal` scope. The nightly evaluator has no selector for it,
  so it skipped the rule every night while Settings → Privacy listed it as acting.
  A migration carries it over to `contact/no_consent_no_deal` with its window and
  action unchanged. Where an admin already added the contact rule, theirs stands.

- **Sweep `_failed` gauges read the last finished run.**
  `margince_sweep_workspaces_failed` and `margince_sweep_units_failed` read the
  newest child of a pass, which is usually a pending next tick, so a tick that
  exhausted its attempts read as healthy. They now take the outcome from the
  newest run in a terminal state. Coverage still counts a pending child, and a
  later `completed` run still supersedes an earlier failure.

- Mobile record action bars and truncated contact deal-room lists keep their
  explanatory text inset from the surface edge. The unchecked Forecast story
  includes its first-run preview.

- **A failed-login lock no longer keeps out a browser that has signed in
  before.** Signing in sets a `crm_device` cookie; while an account is locked, a
  browser presenting it for that account is let in with the correct password.
  Every other sign-in attempt is refused as before.
- **The MCP connector's OAuth discovery documents name the configured public
  origin.** Their issuer and endpoint URLs, and the pointer the transport's 401
  carries, are built from `--public-base-url` rather than from the request, and
  are sent `Cache-Control: no-store`. With no public base URL configured they
  answer 404, as they do with the connector off.
- The exchange-rate refresh accepts a rate from the page it reads only as a plain
  decimal (digits and one decimal point, within bounded widths), the shape the
  currency sheet itself accepts. It drops any other form before parsing it.
- **One MCP connection no longer disconnects the others.**
  A client identified by a Client ID Metadata Document presents the same
  `client_id` for every user, so each new consent revoked the previous user's
  grant. A connection is now superseded per client registration **per human**:
  reconnecting from the same client replaces your own earlier connection, and
  never anyone else's.
- Customer meeting requests remain actionable on won or lost deals and outside
  recent email history. Source-linked reminders reconcile without duplicates;
  accepting or completing one updates request state across the deal and email
  views. Background deal refresh updates facts without model calls.
- **An offer's PDF is no longer served to a reader who cannot open its buyer
  company.** The PDF prints the buyer's legal block, which the offer already
  withheld from that reader along with the buyer's id. For them the offer reads
  as never rendered, a PDF they rendered themselves included, and its download
  answers 404. Changing a draft's buyer removes its PDF, which printed the
  previous one.

## [0.0.1] - 2026-09-10

### Removed

- **The human-set sales quota is gone.** Attainment and the pace band were
  arithmetic on one number a manager typed, so a missing target read as an
  absence. The Reports screen now holds the three deal reports. The `quota`
  table is dropped and the `quota` RBAC object is stripped from every role in the
  same migration. Operational agent budgets (the per-agent volume meter, the AI
  provider spend budget and the incumbent rate allowance) are safety limits and
  keep working as before.

### Changed

- **A meeting brief lights the AI-activity rail.** The rail now lights for every
  request that waits on a model. That covers the meeting brief, the intro drafts, the role
  proposals, the onboarding conversation, and the cache-backed readings (the
  dossier, the contact brief, the deal status) once they run longer than a stored
  answer takes. The contract marks those operations with
  `x-waits-on-model: always | on-miss`, and the client mirrors that set. A stored
  reading still never lights the orb.

- **Email addresses open Margince's composer.** This applies on a contact's or a
  lead's header, in the contact rail's details, and on an account page's people
  cards. The address opens the header's Email composer with the address in the
  To line. Without a connected mailbox the address stays a
  `mailto:` link. Phone numbers still dial through `tel:`.

- **The relationship brief is written by a model.** The contact brief now runs
  on the `summarize` lane as `person_brief`. It reads extracted claims with their
  quotes and open/overdue state, recent changes, the moment the page selected,
  and a one-line summary of each recent message. Every sentence is cited or
  dropped, and a message the reader may not open contributes only its date. With
  no model lane, no budget left or a refused reply, the deterministic brief is
  used, and `generated_by` says which one wrote it. The deterministic brief now
  leads with what is due and quotes the last message.

- **"Set the next step" writes a named step.** The advice names the step
  ("Agree the next step on *Fleet retrofit 2026*", or the account's name when
  several deals are open), and its button sends a prepared `POST /tasks` body
  unchanged. The recommendation also appears on the record's **Tasks** tab,
  marked as the agent's. An archived account offers no button. No task exists
  until a rep presses the button, and the write is audited and undoable.

- **Agent budgets are no longer called quotas.** The volume ceiling on agent
  reads, writes and spend is renamed; its behaviour, thresholds and refusals are
  unchanged. Two stored values are migrated in place: the approval kind
  `quota_release` becomes `volume_release`, and the AI failure code
  `provider_quota` becomes `provider_budget`. A client that has not been updated
  treats the new code as a general failure.

- **Home's test suite is split in two.** It is back under the 1000-line
  ceiling, and the shared harness is now `home.testkit.tsx`.

- **One changelog list per change type.** Three
  `### Changed` lists were merged; `make changelog-sections` now enforces one per
  type.

- **Certification fixtures check their trigger ref.** The ref is compared
  segment by segment with one minted by `AgentSpec.TriggerRef`, so a fixture with
  a stale shape fails by name.

- **BREAKING (metrics): sweep gauges drop `_total`.** Rename
  `margince_sweep_workspaces_total` to `margince_sweep_workspaces` and
  `margince_sweep_units_total` to `margince_sweep_units` in your dashboards and
  alerts. The old names are not aliased. The `_failed` gauges are unchanged.
  These are levels, and Prometheus reserves `_total` for counters; a gate now
  rejects the suffix on a gauge and requires it on a counter.

- **One schema describes the whole of margince.yaml.**
  `config/margince.schema.json` replaces `config/ai-routing.schema.json` and
  covers every section, with the model binding under `$defs.aiRouting`. It is
  generated from `deployconfig.Config` by `tools/gen-configschema`, with
  descriptions from the Go doc comments, and `additionalProperties: false`
  mirrors the loader's `KnownFields(true)`. Gates check that every struct field
  is in the schema and that every shipped config validates. The shipped configs
  carry a `# yaml-language-server:` line so an editor finds the schema.

- **The dev stack's Compose project is named `margince`**, replacing
  `margince-poc-v1`. **The first `make db-up` after upgrading starts on an empty
  database**: the `margince-poc-v1_*` volumes are left in place, not migrated.
  Re-seed with `make seed-dev`, or bring a stack up with `-p margince-poc-v1` to
  read the old data. `MARGINCE_PG_CONTAINER` still overrides the container name
  `scripts/migration-baseline.sh` uses.

- **The AI routing file is gone.** The tier-to-model binding is the
  `ai.routing` setting, seeded from `seeds.ai_routing` in `margince.yaml` and
  changed under Settings → AI. `--ai-routing` and `MARGINCE_AI_ROUTING` are
  ignored with a warning. `ai.LoadRoutingFile` and the
  `config/ai-routing.*.example.yaml` files are deleted; `ai.ParseRouting` still
  reads the seed and the stored setting. A dev stack is bound by
  `seeds.ai_routing` in `config/margince.dev.yaml`.
  - The DB-less lanes take `--model provider:model` or `--ai-fake`
    (`worker siteread`, `worker aitask`).
  - `make e2e-ai` takes `MODEL=`, `JUDGE=`, `BASE_URL=` and `PROFILE=`. `JUDGE=`
    is required, and the run refuses a judge equal to the candidate. `BASE_URL=`
    applies to every rung of the ladder. `PROFILE=sovereign` with a cloud vendor
    is refused through `ai.ValidateTierBinding`, the same rule a parsed config
    meets.
  - The certification runner binds every tier, so a demotion under budget
    pressure lands on a bound tier.
  - The pricing gate reads `seeds.ai_routing` from the shipped YAML files.
  - The desktop launcher passes `--ai-fake` on every boot. A binding stored under
    Settings → AI outranks it and takes effect without a restart.

- **Settings pages use the record page's components.** Every settings card is a
  `Panel`, and pages that drew their own chrome (the company profile drew five
  surfaces, including a gradient hero) now use it. The pages gained a reading
  measure, one vertical rhythm, and real heading styles.

- **`SurfaceState` moved into the design system.** Eight screens imported it from
  `company360.tsx`; they now import it from the design system. `Eyebrow`,
  `PanelPlate`, `Panel`'s accent tone and its actions band moved with it, and
  `SectionHeader` gained level 3 for nested sections.

- **Settings entries open on a read grant.** An entry opens if you may read any
  part of it; the write controls inside say who may use them. A read-only seat
  was hidden from eight of the eleven entries the server answers. On a freshly
  seeded installation every role reaches every entry except Maintenance.

- **Connections are split in two.** *You → Connections* holds the per-user
  surfaces (your mailbox, your LinkedIn network). *Organization → Integrations*
  holds the installation-wide ones (the contact-data credential, webhooks, the
  HubSpot mirror), each with its own permission. The connector OAuth callback
  still returns to `#/settings/connections`; the system-of-record chip links to
  `#/settings/integrations`.

- **Every seat can read the member roster.** People opens
  on it. Inviting, and changing a role or status, stay with the admin. A role nobody may change
  reads as text.

- **Three settings pages are reordered.** AI reads automations → spend → prices →
  per-call trace. General puts the base currency beside its rate sheet. Card
  titles no longer repeat the heading above them, "Your agents" drops the
  possessive its group heading carries, and "Voice" is "Writing voice".

- **Settings has 12 entries, down from 24.** Two groups: Account, Writing voice,
  Agents and Connections under *You*; General, People & access, Integrations,
  Capture, Data model, AI, Privacy & audit and Maintenance under *Organization*.
  `#/custom-fields`, `#/products`, `#/offer-templates` and `#/automations` are
  now content on the page that owns them and are gone as routes; Automations'
  primary-nav slot went to the dedupe queue. `#/design` is deleted.
  `GET /admin/job-health` has a UI on Maintenance, beside the search-index
  rebuild and the danger zone. **No seat lost a surface it could use**: a merged
  entry opens on the union of what its parts asked for. The connector OAuth
  callback redirects to `#/settings/connections` (a hard-coded string in
  `internal/compose/connectors_outcome.go`).

- **Every dropdown uses the design system's `Select`.** It is a button trigger
  plus a portalled listbox with the full keyboard contract (arrows, Home/End,
  typeahead, Escape without committing, focus back to the trigger), styled with
  the product's tokens. It anchors to the trigger inside a scrolling toolbar and
  flips when the room below runs out. Callers pass `options` and receive the
  value in `onChange`. A `<select>`, `<option>` or `<optgroup>` anywhere else
  under `frontend/src` fails `make frontend-check`.
  `frontend/src/design-system/README.md` is the catalog to read before building
  a control.

- **The sign-in screen is a split page.** The identity region runs full-bleed,
  divided from the form by one hairline, and the wordmark sits top-left (above
  the form when the layout stacks). The form is a single 400px column: heading,
  full-width provider buttons, fields, locale row and fine print, with the
  fields left-aligned. On phones (≤560px) the identity region is dropped,
  including the AI disclosure; that gap is tracked in
  [issue 562](https://github.com/margince/margince/issues/562). Every wider
  layout shows the disclosure in full.

- **The sign-in animation runs once per load.** A React remount
  renders the surface already arrived.

- **The Core holds its position.** The sphere's 11-second vertical drift is
  gone; it still breathes, and the beat carries its state.

- **The Core stops in an unfocused window.** The WebGL liquid and
  the CSS rhythms (breath, sheen, halo, feed) stop and resume on one
  document-level `focus`/`blur` signal.

- **BREAKING: passport scopes come from the contract.**
  The scope an operation spends is declared by the contract. Every
  `x-mcp-tool` annotation declares its `scope` beside its tier, and the
  REST agent gate spends that scope instead of a hard-coded `write`. Scopes are
  exact membership, so `write` does not imply `send` or `enrich`. A passport
  minted with `read`+`write` and no `enrich`/`send` is now refused
  `POST /organizations/{id}/enrich`, `/deep-read`, `/coldstart`,
  `POST /offers/{id}/send` and `POST /overlay/reconcile` with `scope_exceeded`.
  Re-mint with the scopes you mean to grant; the per-tool scope is in
  [docs/reference/agent-tools.md](docs/reference/agent-tools.md).

- **Connected agents are listed apart from passports.**
  `GET /v1/passports` carries a `connection` object on grant-issued rows and
  lists one row per connection, however often its credential has rotated.
  Revoking a grant-bound passport ends the whole connection.

- **Language and theme moved into the account menu.** The top bar holds search
  and the account button; the menu reads Settings · Language · Theme · Sign out,
  with each preference showing its current value. Changing one keeps the menu
  open, and closing it returns focus to the avatar. The language row is a nested
  menu: one Escape closes the language list, the next closes the account menu.

- **One orb in the product.** The agent panel at the sidebar foot now shows the
  real Core instead of a CSS lookalike. The Core draws at its displayed size,
  caps itself at 24fps on a timer, and stops on a hidden tab or an off-screen
  canvas.

### Added

- **The lit window edge can be switched off.** Waves run along the window rim
  during a run or a mailbox import. The switch is in the foot of the agent panel
  (the card that opens from the agent block at the bottom of the rail). It is on
  by default, and off is remembered per browser, beside the theme. Everything the
  waves show is also written in the rail, so nothing is lost. A machine set to
  reduced motion still calms the waves without the switch.

- **Introductions are recorded.** `intro_request` records an ask to a colleague
  who can reach a contact, the colleague's answer and the outcome. It ships with
  its API, a requester's composer, and a decision surface for the colleague.
  Approving a name-drop permits a mention only, and the ask completes as
  `name_dropped`; the outcome is read from the ask's state, never from what the
  caller sends. The domain row, the event and the audit after-image use the same
  word. `replied` is set only from captured activity, never by a person.

- **An overnight grant says when it is outgrown.** When the
  agent gains a tool that needs a wider scope than the grant minted, the runner
  drops that tool and the overnight work stops without an error.
  `credential_funds_agent` is a second renewal cause, computed from what the
  build would mint today, and the Settings card offers renewal with copy that
  says the authority was outgrown.

- **Data model**: the core schema as reversible migrations, with workspace
  isolation through `database.WithWorkspaceTx`, composite same-workspace
  foreign keys, an append-only audit log, a transactional event outbox, and the
  core and custom migration namespaces.

- **Contract pipeline**: `api/crm.yaml` (OpenAPI 3.1) → generated types
  + chi server. Every operation is mounted and implemented, and regeneration
  drift blocks a merge.

- **Auth and tenancy**: workspace bootstrap, Argon2id login, opaque
  server-side sessions, seeded system roles, object RBAC with own/team/all row
  scopes, and the read/full seat ceiling.

- **Core CRM**: contacts, organizations, leads (with promotion),
  pipelines/stages, deals (stage-semantic advance, FX freeze at close),
  activities and polymorphic links, two-record merge, lists/tags,
  relationships/partners, deal stakeholders, scheduling.

- **Event bus**: the event envelope over a transactional outbox → Redis
  Streams relay, consumer groups, and at-least-once dedupe.

- **Governed agent surface**: Agent Seat Passports and the remote MCP connector at
  `/mcp` (OAuth 2.1 + PKCE, dynamic client registration, refresh rotation; the
  stdio server is retired). Also the 🟢/🟡 autonomy tiers enforced below the
  transport on MCP and REST alike, and the approval engine (stage → human
  decision → single-use redemption).

- **Consent lends a passport**: `GET /oauth/authorize` opens a consent screen
  where the user picks one of their own agent passports. The connection gets
  that passport's scopes through a new grant-bound passport, so revoking a
  connection never touches the user's own credential. Deny answers the client
  `access_denied`. A user with no passport is guided to mint one and brought
  back, so `claude mcp add` no longer completes unattended for a fresh account.
  The lend is audited. Deactivating a member ends their unredeemed consents and
  their existing connections.

- **AI surfaces**: model routing (the `ai.routing` setting: BYOK cloud via the
  native Anthropic / OpenAI / Gemini adapters or the generic
  `openai_compatible` wire, local Ollama / vLLM, an offline fake). Also the agent
  runner and scheduler, search (FTS + pgvector hybrid), the capture connector
  seam, and cold-start read-back.

- **Model certification**: a hand-authored fixture corpus run through each
  site's own production request builder and validator, scored by a pinned
  judge and committed as a JSON record. `make ai-probe` runs one site against
  operator-supplied input through the same case.

- **The job contract** (`api/jobs.yaml`): every River job kind is declared
  before it is written. A kind missing from the file does not compile, and a
  kind with no timeout fails generation. Each kind declares its role: a
  *dispatcher* enqueues work for the fleet; a *workspace worker* does one
  tenant's work. Job args name rows and never carry content, so Art. 17 erasure
  reaches an in-flight job through the row it names.

- **Fleet observability**: `/metrics` carries the job-runtime section.
  `GET /v1/admin/job-health` shows the same table to an admin (human session
  only, workspace-scoped, failure reasons from a vetted vocabulary).
  `cmd/worker --observe-addr` gives the worker its own `/healthz`, `/readyz`
  and `/metrics`, so you can see which process is wedged.

- **Messaging channels**: a workspace-level Telegram bot binding
  (`/channel-connections`) with pull ingress; the installation long-polls, so
  it needs no public address. Replies go through a governed endpoint
  (`POST /activities/{id}/send-message`) whose recipient is the channel
  identity of the contact in the conversation, never named by the caller.

- **Outbound mail**: a Gmail send surface bound to the same consent as capture.
  A message is staged as a durable `comms_outbound` row, re-checked against the
  sender's live seat and consent at transmit time, and keyed on the
  `Message-ID` Gmail stamps (Gmail rewrites the one we request).

- **The relationship graph**: activity participants projected into an
  interaction edge that shows who on our team already knows a contact. Warmth is
  per user (never summed into a workspace score), with deal coverage and its
  risk rules. A LinkedIn `Connections.csv` import feeds the graph only: its rows
  are invisible to search, lists, contact screens and agent record tools, and
  private to their owner.

- **Company record page**: one gated 360 read behind the whole page, and a
  per-viewer account brief that falls back to a deterministic summary with no
  model lane. Also Ask, record-derived next-step suggestions that each carry their
  reason, a dwell-gated visit baseline, and the one-hop connections graph.

- **Overlay mode (HubSpot as system of record)** and the overlay → native
  cutover: mirror-backed reads, incumbent-first write-back on
  `Update`/`Archive`, a preflight that seals the mirror when green, and a
  confirm-first flip behind a typed phrase. Both flip operations are human-only.
  Reversal is reconstruction from the pre-flip bundle; there is no rollback.

- **Supply chain**: three source-tree SBOMs (CycloneDX, SPDX 2.2.1, SPDX 3.0)
  generated from a clean export of HEAD, normalized and parity-gated,
  license-gated against an allowlist, and keyless-signed on `main` from a job
  isolated from PR-controlled code.

- **Self-hosting materials**: one root `Dockerfile` with `api`, `worker` and
  `web` targets (all non-root), the entrypoints, the one-time
  `db-bootstrap.sql` that creates two non-superuser roles (a DML-only runtime
  role and the role that applies DDL), and a runbook
  ([docs/deployment.md](docs/deployment.md)).

- **Non-production data reset**: `POST /v1/admin/reset-data` wipes workspace
  data back to the bootstrapped state behind four checks: non-production
  posture, human only, `admin` role, and a typed organization-name
  confirmation. In production the operation answers 404, because the posture
  check runs before auth.

- **Embedding drift self-heal**: a periodic worker sweep re-embeds entities
  whose embed event the bus lost, with no operator confirm. The preview →
  confirm reindex is only for a changed embed binding, and the ops banner fires
  only then.

- **GDPR**: per-purpose consent with default-deny suppression, a retention
  evaluator with DE (GoBD) statutory floors, legal hold, Art. 17 erasure with
  re-capture suppression, and Art. 15 SAR assembly.

- **Web UI**: login/bootstrap, contacts, leads, deal board, timeline, search,
  reports, privacy inbox. It is the Vite/React app in `frontend/`, a standalone
  static build served separately from the API.

- **Quality gates**: golangci-lint + depguard, go-arch-lint, tree-derived
  architecture/schema/license fitness tests, contract drift-lint, and a
  real-Postgres integration lane covering the security invariants.

- **Strict craftsmanship gate**: `craft static` fails on MAJOR as well as
  BLOCKER findings, in the pre-push hook (diff-scoped) and in CI's
  `craftsmanship` job (whole tree). MINOR stays advisory. Test files have their
  own size ceilings, 160 body lines / 1000 file lines against 80 / 500 for
  product code. The existing backlog was cleared first.

- **Company 360**: `GET /organizations/{id}/360` serves the company record page
  in one transaction: profile, contacts with relationship strength and
  per-purpose consent, deals, timeline, tags, list memberships, decidable
  approvals, open next steps, and what changed since the caller's last visit. A
  section the caller may not read is omitted and named in `sections_omitted`.
  `POST /organizations/{id}/view-ack` is the explicit, human-only, monotonic
  visit baseline.

- **Company page verbs**: the record page opens a deal on the company it shows
  (open stages only), and applies a tag or a list membership by typed name,
  creating either when the name is new. Each verb renders only on a section the
  caller may read. An already-applied tag or membership counts as success.

- **Company connections**: `GET /organizations/{id}/graph` serves the account's
  one-hop neighbourhood as nodes and edges. It includes its contacts by employment (weighted
  by relationship strength), its open deals and their stakeholders, its parent,
  child and partner companies, and the contact the active signal's warm-intro
  path routes through. Authorization is per group. Node selection is
  deterministic, and `dropped_count` reports what the caps left out. The rail
  draws it as a diagram over a keyboard-reachable node list that carries the
  same content.

- **The installation's own company is excluded from prospects.** The anchor
  organization is left out of the organization list, lexical and vector
  search, dynamic segments and their exports, duplicate detection, and signal
  candidate resolution. `include_anchor` opts in on the native list (shaped
  like `include_archived`); an overlay-mode list refuses it with 422. It stays
  reachable by id, and available where naming it is needed, such as recording
  that a contact works there. `is_anchor` is on the wire, and the agent surface
  learns the id through its company context. Archiving or merging the anchor is
  refused in the schema and in the service.

- **Admins can manage the workspace's own email domains**: `GET`, `POST` and
  `DELETE` on `/capture/email-domains`, admin/ops and human only. An added
  domain is stored verified and applies to the next message; adding one a
  mailbox already contributed confirms that candidate. The list shows what the
  company profile claims separately from the registry. Removing a domain
  resumes capture from that point; mail skipped meanwhile is never offered
  again.

- **Custom fields and tags are filter vocabulary** for dynamic lists, saved
  views and filtered exports. A retired custom field stays filterable, so a
  saved segment naming it keeps returning the same rows. A tag filter can say
  "does not carry this tag" and "carries no tags at all", on contacts,
  organizations, deals, leads and projects. The three surfaces share one
  vocabulary. Authoring such a filter in the product still needs the Filters &
  views screen, which does not exist yet.

### Fixed

- **Empty-state text no longer touches the card edge.** `EmptyState`'s plate now
  uses the card's inset (`--padCard`). The first-run instructional state, the
  heading of the onboarding read's facts block, and a board's failed-read caveat
  had the same defect and are fixed. In a stacked table, a row's title no longer
  paints an opaque patch over the card. `make fe-edge-padding` renders the story
  catalog at two widths and fails on a visible edge with no inline padding.

- **Record pages offer only allowed writes.** The deal,
  contact, lead and company pages read the server's per-row `writable` flag,
  with the object grant and the seat, through one shared answer. A record you
  cannot change shows one sentence saying why and whom to ask, and its write
  verbs are disabled, as the project page already did. The relationships panel
  and the activity composer check the same.

- **A replayed body re-checks every record it names.**
  `POST /people/quick-capture`, `POST /leads/{id}/promote` and
  `POST /leads/{id}/demote` returned other record ids (such as the employer
  organization) without re-checking that the caller can still see them. Every
  record reference in a replayed body is now probed, and a gate derived from the
  contract fails when a response schema adds one that nothing re-checks.

- **A refused API key stops the `e2e-llm` lane.** A `401` was scored as a
  scenario that did nothing, so every run of the lane failed its criteria and
  the verdict blamed the product. A run that never reached the model is now
  reported as such and stops the lane.

- **Outbound mail is written in the installation's language.** The password
  reset, the invitation and the weekly retrospective were always English. One
  catalog now serves all three, keyed off the installation's base language, and
  the weekly reuses the Home panel's strings. A language the catalog lacks is
  sent in English. A gate fails when the contract admits a language the catalog
  does not carry, or leaves a line empty.

- **Dev psql and the DSN share a database.** Ad-hoc statements
  went through `docker compose exec` while the binaries used a published port,
  so a seed run from a second checkout could write another checkout's data.
  Statements now use the DSN's port. When no container publishes that port, or
  more than one does, the command refuses and names what it found.

- **Technology lanes announce only real changes.** A
  completed lane still writes an audit row, but emits `organization.updated`
  only when a fact appeared, moved or went.

- **A 501 is asked once.** The retry policy retried every status from 500 up,
  including `501 Not Implemented`, so an installation with no embeddings model
  sent three reindex-status requests and logged three console errors. 501 and
  505 are no longer retried.

- **Provider names are validated at registration.** The contract publishes
  `Provider` as a pattern-constrained string; the registry now enforces that
  pattern, and a gate keeps the two in step. The messaging-transport registry
  (`ProviderRef`) follows the same rule.

- **Provider runs audit their own provider.** Every run was audited
  as `connector:surfe`, whatever vendor it belonged to. The actor is now the
  run's own connector.

- **A partial job-health payload no longer crashes.** The maintenance
  page dereferenced a required field, and the only error boundary was
  app-level. The payload is now rejected at the query boundary (defaulting it
  would have shown a false idle state), and `CardBoundary` contains a card that
  throws.

- **`Switch` dropped `aria-checked`** when `checked` was `undefined`, so a
  screen reader announced no state and the track drew as off.

- **`Badge` contrast holds on every surface.** The translucent tone tints
  measured 4.54:1 over a panel and 4.05:1 (an AA failure) over a recessed
  plate. Each tone now composites over an explicit surface; the token values are
  unchanged.

- **`SegmentedControl` could not wrap.** The company record's seven-tab strip
  measured 543px inside a 374px column at 390px width and caused the page's
  horizontal overflow.

- **Small text moved to an AA token.** `--textMuted` (1.54:1 on a 13px form
  label) and `--textTertiary` (2.52:1 in nine rules) were replaced by
  `--textMeta`, the token `tokens.css` names for small text.

- **The layout sweeps measure the real page.** The 390px sweep
  measured `document.body`, which never scrolls in this shell. It now measures
  the elements that scroll, covers all twelve settings tabs, and checks that the
  shell rendered (both sweeps had passed on a crashed page).

- **Storybook stories show their named state.** The
  `aiusage`/`aicalls` stories never stubbed `GET /me`, so ten stories captured
  the `/me` error state. The shared stub now refuses to guess a session.

- **Absent, disabled and withheld follow one rule**, written in
  `frontend/src/design-system/README.md`. A surface a permission denies says
  so; a precondition the reader could fix disables the control and says what
  would enable it; only "does not apply" is absent. On the Privacy page, the
  retention card and one other card now state a denial where they used to
  vanish, without asking the server. The installation settings card now says
  why its three fields are disabled.

- **The scroll position resets on a route change.** The content column kept the
  last page's offset.

- **A skip link exists** (WCAG 2.4.1). It is a button, because the app is
  hash-routed and a fragment link would navigate.

- **Entering the settings level keeps focus.** Focus fell to `<body>` on the way
  in.

- **Modals restore focus after their opener goes.**
  Passport revoke, member deactivate, connection end and DSR transition all
  dropped focus to `<body>`. `Modal` takes a `returnFocusTo` resolver evaluated
  at restore time, and checks that the opener is still in the document.

- **Card load failures are announced.** `QueryStates`' error branch has a live
  region, and its skeleton has a busy state and a spoken name. The shared
  create/edit and archive-confirm errors are covered too.

- **Two WCAG Level A defects on the AI attempt trail and the tool console are
  fixed.** A row's expand control was a `<tr onClick>`, unreachable by keyboard.
  The tool console dimmed unreachable rows to `opacity: 0.4`, pushing the
  "scope not granted" caption below the contrast floor; it now uses a
  strikethrough, like the passport list. Credit-pool meters have a label.

- **Headings mark real sections.** Pipeline names, provider names and block
  titles were styled spans; two dialogs opened with a raw `<h3>` under the
  page's `h1`.

- **Passport scope checkboxes are translated.** They showed the raw protocol
  words (`read`, `draft`, `write`, `send`, `enrich`).

- **German Connections strings use *du*.** Twenty-one strings used *Sie*,
  against `de.ts`'s informal register.

- **Retention pauses with a `Switch`.** It used a `Checkbox` and shared Save's
  mutation, so pausing a policy collapsed the edit panel.

- **The 390px and axe sweeps cover more pages.** Data model,
  Integrations and People are swept.

- **`#/settings/capture` links to where company domains are edited.**

[0.0.1]: https://github.com/margince/margince/releases/tag/v0.0.1
