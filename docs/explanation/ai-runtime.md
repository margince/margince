<!-- prose:plain -->
# The AI runtime: tasks, tiers, routing, and the one gate

How every AI call in Margince is *named*, *routed*, *metered*, *traced*, and *certified*. This is the
base under the features. The `cold_start` read back, the deep read, capture sorting, the agent loop and
the briefs all speak the same task vocabulary. They all pass through the same Router. For what an *agent*
does with a call, see [agent-surface.md](agent-surface.md). For how the gate admits it, see
[authorization.md](authorization.md).

## The shape

```
 WHAT (contract, a rebuild)            WHERE (config, runtime)          THE GATE (one path)
 ─────────────────────────            ─────────────────────           ───────────────────
 backend/api/ai-tasks.yaml            the `ai.routing` setting         ai.Router
   task  → ladder of tiers              tier → provider + model          • meter (workspace budget)
   + execution_mode                     profile (egress posture)         • inject company context
   + on_budget_exhausted                BYOK key ← key vault             • trace (ai_call rows)
        │                                     │                          • strip secrets
        │ make gen (drift-gated)              │                          • walk the ladder
        ▼                                     ▼                                 │
   tasks_gen.go  ───────────────────────────────────────────────────────────►  │
   (compiled task/tier/ladder)         (bound at boot, validated)               ▼
                                                                          provider adapter
   task cold_start                                                        (anthropic | openai |
     ladder [cheap_cloud, premium]   ──walk on error/schema-fail──►       gemini | gemini_vertex |
     on_budget_exhausted: degrade                                          ollama | vllm |
                                                                           openai_compatible | fake)
```

**Four rules hold this together:**

1. **Contract first.** *What* a task is (its ladder, its budget rule) sits in `ai-tasks.yaml` and
   compiles into the binary. Changing that policy is a rebuild, held by the drift gate like `crm.yaml`.
   *Which* model serves a tier is runtime config. Policy and deployment stay apart.
2. **One gate.** Every AI call (real, fake, or embedding) goes through the `ai.Router`. There is no
   second path. `--ai-fake` uses the same metered, traced Router (with the fake provider only). Two
   architecture tests fail the build if code builds a model client outside it.
3. **BYOK, declared egress.** Margince runs no model of its own. The key, the endpoint and the DPA
   belong to the customer; the `profile` names where the model may run.
4. **A trace per attempt.** One `ai_call` row per *attempt*, so every retry, step down and step up
   is visible. The served model's identity is read from the wire, and never claimed beyond that.

## The task contract

A **task** is a named AI job such as `cold_start` or `capture_classify`. The full list is
`backend/api/ai-tasks.yaml`, and [reference/ai-certification.md](../reference/ai-certification.md) lists
each task's sites. Code never picks a model; it names a task, and the Router works out the rest.

**A task is not one prompt.** The contract also names each task's **sites** (the places this build
calls the model), and whether the task ships at all. A task can have more than one site (`cold_start`
has more than one). A `planned` task has no site, no scenario and no certification record. The site is
the unit that all later counts use. A number per task would let one certified prompt stand for another
that was never measured.

Each task declares a **ladder** (an ordered list of **tiers**, each a class of model), an **`execution_mode`**,
and a **budget rule**:

```yaml
# backend/api/ai-tasks.yaml
tiers: [local_small, cheap_cloud, premium, frontier, local_large]

tasks:
  cold_start:    {ladder: [cheap_cloud, premium], execution_mode: interactive, on_budget_exhausted: degrade}
  site_extract:  {ladder: [premium],              execution_mode: background,  on_budget_exhausted: queue}
  capture_classify: {ladder: [local_small, cheap_cloud], execution_mode: background, on_budget_exhausted: queue}
```

- **A tier** is a *class of model*, not a model. `local_small` and `local_large` run on the same box,
  so no data leaves. `cheap_cloud` is fast and cheap and hosted. `premium` is strong hosted reasoning.
  `frontier` is the strongest a deployment will pay for; no task ladder names it, so it costs nothing
  until one does.
- A task's ladder is its **order to fall back**. The Router starts at the first tier, and walks to the
  next on a provider error or a failed schema check. So a short failure moves down a step, and does not
  drop the call.
- **`execution_mode`** names who is waiting: `interactive` (a human, in the middle of work) or
  `background` (a worker job). It pairs with the budget rule: an `interactive` task always declares
  `degrade`, and a `background` task `queue`. The contract's own header states this rule.
- **`on_budget_exhausted`** is what happens when the workspace's monthly model budget is spent.
  `degrade` answers on a cheaper rung (at 100% an `interactive` task is pinned to `local_small`, not
  blocked). `queue` waits and does not spend past the budget.
- A queued wait is a **typed refusal**. The Router returns `BudgetDeferralError` (which wraps
  `ErrBudgetDeferred`) with `NextAttemptAt`, the next budget window. It does so **before any provider
  attempt or `ai_call` row exists**, so a deferral costs nothing and traces nothing. A task with only
  `premium`, like `site_extract`, has no cheaper rung, so it queues.

### Every field in `ai-tasks.yaml`

At the top level:

| Field | Shape | Means |
|---|---|---|
| `tiers` | ordered list | the classes of model. **The order matters**: it sets the order of the `Tier` values and of the enum in the routing schema, the same byte for byte on every generator run. |
| `tasks` | map of name → task | the jobs. Names are `snake_case` in lower case, and map one to one onto the generated `ai.TaskX` Go value. |
| `embed` | `{tier, cost_unit}` | the embedding job. It is **not** a task, because its tier is not a chat tier, and it has no prompt, no text answer and no path that returns text. So it has no sites and no certification duty. |
| `degrade_to` | map tier → tier | where a tier falls when the budget guard moves it down. `local_small` maps to itself: the floor. |

Per task:

| Field | Values | Means |
|---|---|---|
| `ladder` | ordered tiers | the **order to fall back**. The Router starts at the first rung, and walks to the next on a provider error or a failed schema check. A ladder with one rung (`site_extract`: `[premium]`) has no rung to fall to. |
| `execution_mode` | `interactive` \| `background` | who is waiting: a human in the middle of work, or a worker job. |
| `on_budget_exhausted` | `degrade` \| `queue` | what a spent monthly budget does. **Closed pairing rule:** `interactive` always pairs with `degrade`, `background` with `queue`. `queue` returns a typed deferral to the task's own lasting carrier; the Router never makes a job that nobody owns. |
| `status` | `shipped` \| `planned` | whether the task exists in this build. `shipped` means every site must be registered, have a case, and be covered by a scenario. `planned` means it may have no site, no scenario and no record. This is what stops a task that was never built from looking certified. |
| `sites` | list | the named sites. A bare string is a site of kind `one_shot`; `{name: x, kind: y}` declares another kind. |
| `sites[].kind` | `one_shot` \| `multi_turn` \| `agent_loop` | how the model is called, and so how much of the site one certification run can cover. A closed set: a new kind is a change to code and tests, because each needs a certification plan that can run it. |
| `sites[].tools` | tool names, on an `agent_loop` site only | `agent_loop` is the engine, and each of its sites is one scheduled agent. This is the only set of tools that run is given. It must be there and not empty (the runner refuses a job with none). It is never the whole served catalog, which `TestEveryAgentSpecNamesRegisteredTools` in compose fails. |
| `no_payload` | `true` (or left out) | content from this task must **never** reach `ai_call_payload`, no matter what the capture setting of the deployment says. It is a parsed field, so a data control does not depend on text in a `doc:` string. |
| `company_context` | `none` \| `{scopes, token_budget, conditional}` | the bounded block of company profile that this task's prompts may hold. **Not optional**: a missing policy is a build error, never a runtime default. |
| `company_context.scopes` | any of `identity`, `positioning`, `sales`, `offer`, `market`, `proof`, `administrative` | which bounded views of the company profile may be put in. That order is also the wire and fingerprint order, so listing the same scopes in a new order cannot change the hash. |
| `company_context.token_budget` | positive `int` | the size the renderer bounds the block to. It must be there with any scope (at zero the scopes would reach no prompt). It is refused without a scope, since a budget on a policy that selects nothing reads as a deleted scope list. |
| `company_context.conditional` | `true` (or left out) | put it in only when the caller asks, not always. |
| `cost_unit` | rule name, or left out | which rule prices this task before it runs (`per_message`, `per_contact`; `per_entity` for embed). The math stays in code. Naming the rule here lets the build prove the mapping is **whole** in both directions. Left out means not priced. |
| `doc` | string | copied into the comment of the generated Go value. Text only: nothing may depend on it. |

`make gen` compiles this into `tasks_gen.go` (and the routing shape in `config/margince.schema.json`).
The drift gate fails the build if the generated files do not match, so the contract cannot drift without
anyone seeing it. Adding a task or a site has a check list of its own:
[how-to/add-an-ai-task.md](../how-to/add-an-ai-task.md).

## The routing config

The **runtime binding** says which real provider and model serves each tier, and nothing about policy.
It is the `ai.routing` **setting** (a row, not a file). Every serving role reads it from the database. So
the api and the worker cannot drift onto a different binding each, and a change needs no restart.

No role reads a routing file. `--ai-routing` and `MARGINCE_AI_ROUTING` are still accepted, so an old command line
still parses. Then they are ignored, with a warning that names what to use.

The lanes with no database are each told their model. `worker siteread` and `worker aitask` take
`--model provider:model` or `--ai-fake`, and the certification runner is given `MODEL=` and `JUDGE=`.
Each one probes one named binding. A file on the machine that had run it could never be compared between
engineers.

A new installation declares it under `seeds.ai_routing` in `margince.yaml`. It is read once, at
bootstrap; a dev stack's is in `config/margince.dev.yaml`. A running one is bound again under Settings →
AI, or through `PUT /v1/ai/routing`. The shape below is the shape of that binding.

```yaml
profile: cloud_frontier       # WHERE inference may run (the egress posture)
tiers:
  local_small: {provider: ollama,  model: gemma3}
  cheap_cloud: {provider: gemini,  model: gemini-2.5-flash}
  premium:     {provider: gemini,  model: gemini-2.5-pro}
embeddings:    {provider: gemini}
```

- **`profile`** is the location ladder: where the model may run. `eu_hosted` means a partner runs the
  model in the EU. `sovereign` means no data leaves, by the way it is built. `cloud_frontier` means a
  vendor's cloud, where the vendor serves.
- Only `sovereign` limits anything: it refuses a cloud provider. `eu_hosted` and `cloud_frontier` are
  labels that refuse no binding on location grounds. Where a lane is served is the connection's choice
  (`only` on the broker `upstream`, or a Vertex `location`).
- **No key ever sits in the binding.** A provider names only itself, and an `api_key:` left in it is a *boot
  error*. Where the key comes from depends on who asks. A served installation reads it from the **key
  vault**.
- The lanes with no database (`worker siteread`, `worker aitask`, the certification runner) open no
  vault. They read the usual environment value (`GEMINI_API_KEY`, `ANTHROPIC_API_KEY`, …) on every run.
- **A tier may be left with no binding.** A deployment may run only some jobs. A ladder with no bound tier is
  not a start error, but it is reported. Boot warns per task, for example:
  `task cold_start: no bound tier on ladder [cheap_cloud premium]; calls will be refused`.
- `/readyz` names the AI state (`configured` | `fake` | `unconfigured`). So an operator reads the gap
  off the boot log, not off a refused call at `3am`.
- **The models that can serve a task** are the ladder plus its closure under `degrade_to`, and readers
  tend to forget the second half. `ai.LeadingTier` is the rung that answers when nothing has gone wrong (the
  first rung of the ladder). `ai.ServableTiers` is every rung that can end up answering: the ladder,
  then every tier that `degrade_to` reaches from it, step after step.
- An example: `draft_reply` has the ladder `[cheap_cloud, premium]`, and `cheap_cloud` moves down to
  `local_small`. So the model bound at `local_small` serves `draft_reply` when the budget runs low, while
  the ladder never names that rung.
- Any claim that "these models can answer this task" must be built from the closure. That holds for a
  certification report, or an operator's own audit of what a binding opens up. If not, it answers a
  smaller question than it seems to.

Binding a tier to a provider is an edit to *this setting*; changing a task's ladder is an edit to the
*contract* (above). Moving from Gemini to a local Ollama, or pinning a `premium` Sonnet, never touches
code. See [connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md) and
[enrich-with-a-local-llm.md](../how-to/enrich-with-a-local-llm.md).

## The monthly budget: allowance, deferral, and resume

The budget that every call is metered against is itself a **setting** an admin can change (`ai.budget`,
`BudgetConfig` in `internal/modules/ai/budgetsettings.go`). It is read the same way as the routing
binding above: from the database, by every serving role, with no restart.

```json
{"tokens_per_full_user": 12000000, "company_monthly_tokens": null}
```

**Mode 1: `tokens_per_full_user` × active full users (the default).** The pool moves with the number of
active full users, so it goes up and down with the team, with no change by hand. With no user who counts,
the math uses one as the floor.

**Mode 2: `company_monthly_tokens`** (a fixed total, apart from the size of the team). It sets a fixed
monthly total, for example to match a cap in a contract or a limit from finance. When set, it wins over
the math, but it does not drop the value per user under it. So clearing it brings back
the earlier allowance. The limit is named in tokens, never money, because cost is priced when it is read
and is never the gate (see Cost, below).

Both figures, and their **product**, are bounded by `MaxMonthlyTokens` (`10^12`).

The bound acts in one of two ways, split by whether anyone is choosing the value now. A config being *written* is refused when
its product passes the bound (`MonthlyTokens`, on `PreviewBudget` and `ReplaceBudget`). A config already
*stored* stops at the bound (`SaturatingMonthlyTokens`). Enough full users can push a valid rate per user
past the bound. Routed spend, the status screen, and the read that `ReplaceBudget` needs before a fix
then all work against the bound, and return no error. Such a limit can only allow less than what was
set, never more.

**The band** (`BudgetBand(spent, monthly)` in `usage.go`) turns spend into one of three states. One set of
limits serves every part that asks "how are we doing". Those parts are the routing ladder, the admin
status screen, and the cost preview.

- `normal`: under 80%.
- `degraded`: 80% up to 100%; a task with `on_budget_exhausted: degrade` falls to a cheaper rung.
- `queued`: 100% or over, or the budget itself is zero or less. A wrong setting fails closed, never
  open.

Before saving a change, an admin can **preview** a new `BudgetConfig` or routing edit against live spend.
The preview and the save share `Revision()`, a hash of the config in its standard form. A save is refused
if the stored config changed since the preview was made (someone else saved first). So it never writes
over that change without seeing it.

**A queued background task does not go away.** It becomes a lasting carrier that can resume. The carriers
are a website read (`site_read`), a company scan (`company_scan`), and a Voice-DNA build (`voice_build`),
named here by the table each sits in. The queue job kind that resumes the first two is spelled
differently (`site_deep_read`, `account_scan`); the third is `voice_build`.

Each carrier has its own predicate for work the budget holds back (`contacts.BudgetDeferredSiteReads`,
`companyscan.BudgetDeferredScans`, `ai.BudgetDeferredVoiceBuilds`). None of them counts work waiting on
a provider. Its count is what the list of waiting work under Settings → AI shows. That list is gated on
`ai_diagnostics:read`, which is separate from the `ai_budget:read` needed to see the allowance itself. So
a user who may only edit the budget can preview an allowance change, without an `ai_diagnostics` or
`ai_routing` grant.

A job for the whole fleet is `ai_budget_resume` (`compose/jobs_aibudget.go`). It runs once a minute per
workspace, on worker start, and at once each time the budget setting is saved (`ai_budget.updated`). So
raising the allowance resumes work that may run right away, not after a wait of up to a minute:

```
 on tick / worker start / ai_budget.updated, per workspace
   │
   ▼
 spend still at BandQueued? ──yes──▶ do nothing, try again next trigger
   │ no
   ▼
 for each deferred carrier (site read / company scan / voice build):
   1. re-derive the requester from the carrier row itself — a human
      requester for any carrier; a site read alone may instead carry
      the capture pipeline's own "system:capture_auto_enrich" identity
   2. re-check a HUMAN requester's authority NOW (a revoked or
      deactivated one stays parked; restoring their access is what
      makes it eligible on a later pass) — the auto-enrich case has no
      human grant to revoke, so this step is a no-op for it
   3. lock the underlying queue job. For a site read or company scan,
      refuse one that is missing, already terminal, or has exhausted
      its own attempt limit. A voice build differs: only an ACTIVE
      original goes through that same refusal check — a missing or
      terminal one instead gets a FRESH job inserted in its place,
      starting its own attempt count
   4. put it back on its queue — same requester, same attempt count
      (except the voice-build fresh-insert case above)
```

`ResumeScheduledTx` (`platform/jobs/resume.go`) is what steps 3–4 mostly run against. It locks the
`river_job` row `FOR UPDATE`, and does nothing if the job is already `running`. It returns an error, and
does not start the job again, when the job's `attempt` has already reached `max_attempts`. A resumed
job spends one ordinary River attempt like any other retry. Waiting gives no extra attempts. The one
exception is a voice build whose first job is gone, so a new one starts.

**Known gap:** the sweep finds the requester of a site read by *identity*, not by a general rule. It
knows a human requester, or one named system actor (`system:capture_auto_enrich`). Company scans and
voice builds know a human requester only. A site read started under a *different* system identity (a
domain triage read is the one that exists) has no requester the sweep can find. It fails with an error
(`"requester cannot be resolved"`), where it should stay parked.

Take a new path that a system starts, wired into any carrier that can resume. If the sweep does not
learn its identity, this gap comes back on every tick. It lasts for as long as one such carrier waits.

RBAC: reading and changing the allowance is its own object, `ai_budget`. `admin` and `ops` can change
it, `management` can read it, and nobody else sees it. It is separate from `ai_diagnostics` (counts of
waiting work, finding tiers no ladder uses, and the receipt's workspace-wide count of machine
actions it cannot place) and `ai_routing` (the provider binding itself). So a custom role can
hold any set of the three. The full matrix: [reference/rbac-matrix.md](../reference/rbac-matrix.md).

## The decision lane

A **decision model** answers a typed question about a structured JSON `state` with a score for each label, not
with generated text. A task whose contract says `decision: true` (`site_triage` and the two capture
verdict tasks) has a second form of each site. That form is a question and one test per label, built from the
same inputs as its prompt (see [ai-prompts.md](../reference/ai-prompts.md)).

The routing config may bind one `decisions:` lane beside `embeddings:`, with the same `provider` /
`model` / `base_url` shape. When it is not bound, every call uses the ladder alone. When it is bound,
`Router.Decide` asks the lane first, inside the same call and rail entry. It falls back to the task's own
ladder and prompt unless every check passes, in this order:

1. **Local only stays local.** Nothing checks this today. `localOnlyAdmits`
   (`internal/modules/ai/localonly.go`) admits every call. It is the predicate that this lane and the
   ladder's `servableLadder` both read. See <https://github.com/margince/margince/issues/3351>.
2. **The answer stands.** The state has its secrets stripped and is capped at 48,000 bytes. The call
   has the task's [decision timeout](ai-request-settings.md). The answer must clear the **site's own**
   floor (the floor of its LLM path).

No certification row is needed, only the two checks above.

[Certifying a site](../how-to/certify-a-decision-site.md) is advice: a measured record that an operator
trusts it by, never something `Router.Decide` reads. A fallback leaves its reason on the ladder's first
attempt: `decision_local_only`, `decision_state_too_large`, `decision_error`, `decision_off_enum` or
`decision_below_floor`.

A decision attempt is its own `ai_call` row (`kind = decision`, tier `decide`), priced on the `decisions`
rate lane. It keeps its answer (`decision_choice`, `decision_confidence`) whether or not the answer was used.
That is because floors are set from real cases of falling back, `no_payload` tasks too. `GET /v1/ai/usage` counts
`decisions` per call.

Two providers speak the one wire, with `base_url` being the full endpoint. `jev` is the own API of TypeSafe,
and `jev_compatible` is any server that speaks the Jev wire. That is OpenRouter
([openrouter.md](../reference/openrouter.md#11-the-decisions-endpoint)), or a Kev, Laya or LiteLLM that you
host on your own machine. `sovereign` refuses `jev`, and holds `jev_compatible` to its endpoint rule. See
[configuration.md](../reference/configuration.md).

## The one gate: `ai.Router`

Every call ends at the Router (`internal/modules/ai`). In one pass it:

- **meters** the workspace's monthly model budget (above), and applies `execution_mode` +
  `on_budget_exhausted` when it is spent;
- **puts company context in** where the task's policy asks for it (below);
- **strips secrets** from the prompt before the request leaves the process, and again from all it
  records;
- **walks the ladder**: one attempt per rung. It moves up on a provider error or a schema failure in
  structured output, and skips a rung whose provider is blocked
  ([provider health](ai-provider-health.md));
- **traces** every attempt (below).

**Company context** is the installation's own profile (offer, ICP, voice: what the onboarding steps
confirm). It goes into task prompts as data under rules, not as free text. A request holds typed
`ContextScopes`, a `ContextFingerprint`, and byte and token estimates (`ports/model`). All of them go into
the `ai_call` trace and key the answer cache (the same prompt with different context is a different call).
They also show up as `/metrics` counters per task.

The whole lane sits behind the `company_context.rollout` switch in `margince.yaml`, which can turn it
off. Its steps are ordered `off < read < tasks < onboarding` (default `onboarding` = all the way on;
`platform/deployconfig`).

The form with no database, `ai.NewLocalRouter`, serves the same seam for each offline `fixture` and the
certification lane. `--ai-fake` binds the offline fake *through the Router*. So dev and test run the
same metering, tracing and budget path that production does. `TestNoModelClientOutsideTheGate` and
`TestOneModelPathPerRole` (in `backend/gates/arch_test.go`) hold that as a fact of the build.

## Tracing: what certification counts

Every attempt writes one `ai_call` row (migration `0100`), not one row per final answer:

- `logical_call_id` groups the attempts of one call; `attempt` orders them; `is_terminal` marks the one
  the caller was given. Every retry, step down and step up is visible; metrics count final rows only.
- **`served_identity_source`** labels how the served model's identity was learned. `response` means the
  provider reported it on the wire. `echo` means a general endpoint in the OpenAI shape sent back the id
  that was asked for. `configured` means everything failed and the binding was used. A model can never
  *claim* a source of higher trust than its adapter has proved.
- **The config snapshot** for each call is keyed by hash in `ai_call_config` (task contract hash + routing config hash).
  It holds the fixed facts of the build and the deployment, never a key or a prompt.
- **Company context provenance** goes on the same row (migration `0102`): the context scopes,
  fingerprint and size that shaped the prompt. So "what did the model know about us" can be answered per
  attempt.
- Embedding calls are traced too. The privacy retention check removes their rows after 90 days.

The write path is the standard one (`ai_call` + `ai_call_payload` in one `WithWorkspaceTx`). So a trace
is written and audited like any domain row. See [write-backbone.md](write-backbone.md).

## Cost — the meter collects tokens, a rate table prices them

Model use is on the customer's own provider bill, so cost is **there to look at, never a gate**. It is a
labeled number shown *about* their spend, while the budget guard above stays in tokens. The write path
follows that. The meter and `ai_call` collect **tokens only** and know nothing about money. Price is
worked out *when it is read*. So a fixed rate corrects every figure, and nothing extra sits on the path of
a model call.

```
 WRITE (tokens only)                      RATES (fx_rate-style)            READ (priced on demand)
 ──────────────────                       ────────────────────            ───────────────────────
 ai_call: tokens_in / cached_tokens       ai_model_rate                   • /ai/usage  → actuals   (phase 1)
          / cache_write_tokens              per (provider, model, day)     • backfill preview → estimate (phase 2)
          / tokens_out  (per attempt)       4 micro-USD/MTok components          │
                                            input · cache_read ·                 └─ cost = uncached_in×in + cached×read
                                            cache_write · output                          + cache_write×write + out×out
```

- **The rate table (`ai_model_rate`).** It has workspace scope, with one row per
  `(provider, model, effective_date)`. It is keyed on the *real model that served*, not the tier, so
  binding a tier again keeps its rates.
- Each row is four whole number prices in **micro-USD per MTok**: input, cache read, cache write,
  output. A cache price of `0` means the vendor publishes none (no lower price, no extra charge). So those
  tokens price at the input rate, not as free.
- Finding a rate works like `fx_rate`: the latest row dated on or before the call's day wins. A price change is a
  *new* row, never an edit. Local providers get rows that are all zero, so a local call prices as a
  real `0`.
- **Not priced is not free.** A call whose model has no rate row is *not priced*. It is still counted
  and shown, but flagged as different from a real `$0`. Changing a price is a single insert; no rebuild.
- **One formula, three users.** The price math over the four parts is written once, as `PriceCall`.
  `/ai/usage` reports **actuals** through `RateStore.CostReport`. That is SQL that mirrors the same math,
  row for row, to price a whole window in one query.
- The backfill preview and the certification record both call `PriceCall` directly, for an **estimate
  before the run** and a cost stamp per run. With one formula, the numbers cannot drift.
- **The estimate before the run (`compose/costestimate`).** The same estimate told as one story from
  start to end is in [mail-history-import.md](mail-history-import.md). That story covers the consent
  screen, the scope count, and the spend that comes after the import ends. The formula is here.
  Before a backfill runs, the preview estimates its cost as `Σ per-task (per-unit cost × expected units)`:
  - **Cost per unit** comes from the last 7 days of `ai_call` history, grouped into
    `(task, tier, provider, model)` groups. Each group is priced at the model that *will* serve it now.
  - That is the model that served it, if that model is still bound. If not, it is the current binding of
    the group's tier, so a new binding changes the price at once. If that tier now has no binding, it is
    the head of the ladder.
  - **Expected units** come from what the connection's completed backfill runs found: messages to sort,
    contacts to enrich, records to embed. A run measures its own yield as it pages.
  - The counterparty resolver reports whether an `ensure` call *made* a contact or company, or matched rows
    that already existed. Those counts commit in the same statement as `scanned`/`captured`, so a page
    that fails to commit counts nothing.
  - **With nothing to price from, it says so.** With no history it falls back to a priced floor based
    on the shape of the work, and labels the estimate `heuristic` (not `observed`).
  - If the whole preview would not be priced, it *hides* the cost field and does not show a false `0`.
    A failed cost read moves it down to a plain count of messages. It never blocks the consent flow.

  The estimate counts low in two known places. The contact and company counts hold only what a run's own pages
  made.

  The tier gate may hold a sender back. The verdict engine then decides that sender long
  after the page, and the contact it may make in the end belongs to no page. So a run that made nobody reports
  `ratio unavailable`, not zero contacts. That sets the enrich line to its `heuristic` floor, and does not
  quote $0 as if it were known. And the `cold_start` floor counts message embeds only, because contact and company
  embeds would quote too high at its unit size of a full email.

## How a binding proves it is good enough

A task names a contract, and the model behind it can be swapped out. So you can **certify a model for a
task** before you trust it. The cert lane (`compose/aicert`) turns more than one run, with the cache
off, into one verdict (`certified` / `supported_degraded` / `not_supported`). The verdict is saved as a
committed JSON record. It is scored per site, with the production request builder and validator.

How a grade is made, what a record claims and when it goes stale: [ai-certification.md](ai-certification.md).
The generated page per site: [reference/ai-certification.md](../reference/ai-certification.md).

Each run reports one of four results (`accepted`, `wrong_answer`, `invalid`, `abstained`). The record
names the scope it covers (`full_invocation` > `single_turn` > `single_call`). A pinned rubric judge on
its own `cert_judge` binding, never the candidate's, scores quality 0–100. Nothing here gates a merge:
`make e2e-ai-report` reports each shipped site's record as current, partial, stale or `absent`.

A stale record is an old measure: something it measured (a scenario, or the prompt the product sends)
has changed since. A missing record makes no claim. A partial record is current for the scenarios it
measured, and silent on the rest. Each scenario has its own stamp (`aicert.ScenarioStamps`, which
`FoldScenarioStamps` turns into the task's `PromptVersion`). So adding a scenario marks the record partial (for
example 9/10), and leaves the other results current.

The fixed census gates do block. They refuse a shipped task whose site nobody wrote, a site the contract
never declared, and a planned task someone built. They also refuse a site with no certification case,
and a closed answer kind that no scenario asks a model for (`TestEveryClosedAnswerKindCarriesAScenario`).

### Every field in a scenario file

One YAML file per scenario under `internal/compose/aicert/corpus/<task>/<name>.yaml`, loaded by
`LoadCorpus`:

| Field | Needed | Means |
|---|---|---|
| `name` | yes | the scenario's own name: what a record row and a failure message call it. |
| `task` | yes | must name a task the contract holds. |
| `site` | yes | which registered site is under certification. The site, not the task, is the unit: one scenario can never stand for a task's other prompts. |
| `source` | yes | provenance. Must be `hand_authored`. An `extracted:` scenario is refused at once, because the review and redaction path for one is not wired. |
| `sanitized_by` | yes | who reviewed this scenario for private content. Not empty, and it names a reviewer, not a tool. |
| `fixture` | yes | **the data production is given**, never the prompt production sends. The site's own case turns it into the request. That is what makes the run measure the shipped builder, not a copy of it. |
| `expect.outcome` | yes | which of `accepted` / `wrong_answer` / `invalid` / `abstained` the site's validator must report. Nothing puts `accepted` above the rest, so a scenario whose right answer is *silence* can exist. |
| `expect.answer` | when the result states content | the answer itself, **in that site's own vocabulary**: a bare token, a list, a map, a `{min,max}` band. There is no common shape, because what tells a right answer from a wrong one differs per site. |
| `expect.rubric` | when quality is scored | what the grader is told to look at. It may only ask for what the site's reply envelope can hold. A rubric that scores a field the schema cannot hold measures nothing, and can only mark a correct reply down. |
| `expect.bands` | yes | `certified_min` / `degraded_min` / `floor`: the 0–100 bars this scenario's judge scores are measured against. That is the pooled margin above `certified_min` or `degraded_min`, and the block and floor on its own upper bound. Leaving the block out is refused, with no default, since a missing gate would pass everything. |
| `expect.caps` | optional | the run's limits on cost and time. Going past one fails like a failed check of the shape, never in silence. `max_tokens` budgets the model's **answer** alone. It does not count the fixed input the model cannot make smaller, or the thinking inside a reasoning model. So a scenario with a large input and a small output cap tests drafting within budget, not prompt size. `p95_latency_ms` judges **cloud candidates only**, since the speed of an engine on the same host is a fact about the hardware. Both are read off the run's **pooled** calls: a site that answers in three requests spent all three. |

The full steps: [how-to/certify-an-ai-model.md](../how-to/certify-an-ai-model.md); adding a task or site:
[how-to/add-an-ai-task.md](../how-to/add-an-ai-task.md); writing the case that certifies one:
[how-to/write-a-certification-case.md](../how-to/write-a-certification-case.md).

## Reference

| What | Where |
|---|---|
| Task contract (tasks, tiers, ladders, budget rule, status, sites, context, cost unit) | `backend/api/ai-tasks.yaml` → `tasks_gen.go` (through `tools/gen-aitasks`, `make gen`) |
| Site census (which sites this build ships, and the case that certifies each) | `internal/compose/aitaskregistry.go` (`NewTaskCensus`) · `internal/compose/aitasks` |
| The runtime binding (tier → provider and model, profile) | the `ai.routing` setting, seeded from `seeds.ai_routing`, changed under Settings → AI. Shape declared under `$defs.aiRouting` in `config/margince.schema.json` |
| BYOK keys | the key vault, set under Settings → AI → Model provider keys. The usual environment values (`GEMINI_API_KEY`, `GEMINI_VERTEX_SA_JSON`, `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENAI_COMPATIBLE_API_KEY`, `TYPESAFE_API_KEY`, `JEV_COMPATIBLE_API_KEY`) are read once, to store a key in the vault on first boot |
| The gate | `internal/modules/ai`: `ai.Router` / `ai.NewLocalRouter`; `--ai-fake` flag |
| Decision lane | `decisions:` in the routing setting · `Router.Decide` (`decideroute.go`) · certified rows in `decisioncert_gen.go` |
| Providers | `anthropic`, `openai`, `gemini`, `gemini_vertex` (built in; `gemini_vertex` is the `gemini` wire on Vertex AI, bound by `location`) · `ollama`, `vllm`, `openai_compatible` · `fake` · decision lane only: `jev`, `jev_compatible` (`providerregistry.go`) |
| Tracing | `ai_call` / `ai_call_payload` / `ai_call_config` (migrations `0088`, `0089`, `0100`, `0102`) |
| Cost rates | `ai_model_rate` (per provider and model, dated, micro-USD) · seeded by `SeedModelRates` |
| Pricer (actuals) | `PriceCall` + `RateStore` (`internal/modules/ai`) → `/ai/usage` `cost_est_minor` |
| Estimate before the run | `internal/compose/costestimate` (backfill preview `estimated_cost_minor` + `estimate_quality`) |
| Monthly budget setting | `ai.budget`: `BudgetConfig` (`internal/modules/ai/budgetsettings.go`); RBAC object `ai_budget` |
| Budget deferral | `BudgetDeferralError` / `ErrBudgetDeferred` (`internal/modules/ai/budget.go`) |
| Deferred carriers & resume | `site_read` / `company_scan` / `voice_build`, each with a `BudgetDeferred*` predicate; resumed by the fleet job `ai_budget_resume` (`internal/compose/jobs_aibudget.go`) through `ResumeScheduledTx` (`internal/platform/jobs/resume.go`) |
| Company context | `companycontextprompt.go` (compose) · switch `company_context.rollout` (`margince.yaml`, `platform/deployconfig`, migration `0105`) |
| Boot and `ops` surface | `/readyz` AI state; a boot warning per task for a ladder with no bound tier |
| The certification lane | `internal/compose/aicert`: `make e2e-ai`, `make e2e-ai-report` |

**See also:**

- [agent-surface.md](agent-surface.md): what agents do with a call.
- [ai-activity-rail.md](ai-activity-rail.md): how a call reaches the rail a rep watches.
- [authorization.md](authorization.md): the admission gate.
- [handbook/settings.md](../handbook/settings.md): the Monthly AI allowance page for admins.
- [how-to/connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md),
  [how-to/enrich-with-a-local-llm.md](../how-to/enrich-with-a-local-llm.md),
  [how-to/certify-an-ai-model.md](../how-to/certify-an-ai-model.md),
  [how-to/add-an-ai-task.md](../how-to/add-an-ai-task.md),
  [reference/configuration.md](../reference/configuration.md).
