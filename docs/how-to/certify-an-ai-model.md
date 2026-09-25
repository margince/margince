# Certify an AI model

Prove a model is good enough for a Margince AI task **before** you bind it in
production — and benchmark a candidate swap against the one you run today. The
certification lane (`compose/aicert`) drives a hand-authored **fixture** corpus
through a real model — each site's own production request builder and production
validator, never a copy of either — scores each answer with a pinned rubric
judge, folds the runs into a `certified` / `supported_degraded` /
`not_supported` verdict, and commits the result as a JSON record.

This is the **paid, opt-in** lane: real provider calls billed to your own **BYOK**
(bring-your-own-key) budget, since Margince runs no inference of its own. A
developer/CI tool, never part of a request path.

> **Start free.** `make e2e-ai-report` ([§3](#3-read-the-readiness-report)) needs
> no key, no network and no database: it prints what every shipped site's record
> already says, including the ones nothing has ever certified. Read it before you
> spend — it tells you whether the run you are about to pay for is the missing one.

See also [ai-runtime.md](../explanation/ai-runtime.md), [connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md),
[add-an-ai-task.md](add-an-ai-task.md) (adding one rather than certifying it), and
[reference/ai-certification.md](../reference/ai-certification.md) — the committed page these records render to.

## Prerequisites

1. **What to certify, named outright** — one of two things, never a default:

   - `MODEL=provider:model` — ONE candidate, bound to every task under test.
     The A/B shape: change the model, leave everything else, compare.
   - `ROUTING=<deployment config>` — a **deployment**: each task is certified
     against the model that config's `seeds.ai_routing` binds at the task's
     *leading ladder rung*, the rung that would actually serve it, so one run
     writes records across several models. Nobody deploys a model; they deploy a
     binding, and that is the question an install actually depends on. A task
     whose leading rung the config leaves unbound is reported and skipped, since
     one unbound tier must not cost every other record.

   The two are mutually exclusive and a run with both is refused: one names a
   deployment, the other one candidate. Neither reads the *installation's*
   binding — that is the `ai.routing` setting, this lane opens no database, and
   `ROUTING=` reads what a fresh install would be *seeded* with.

   `JUDGE=provider:model` is the second model that grades the answers, and **one
   judge grades every task of a run** — a judge swap flips verdicts on its own.
   It defaults to `claude_cli:claude-sonnet-4-6`, graded through `claude -p` on a
   Claude Code subscription (needs `claude` on PATH and `CLAUDE_CODE_OAUTH_TOKEN`);
   an exported `MARGINCE_AICERT_JUDGE_MODEL` replaces it and `JUDGE=` overrides
   both — `openai_compatible:anthropic/claude-sonnet-4.6` with
   `JUDGE_UPSTREAM='{}'` (same model, paid per call) or `gemini:gemini-3.5-flash`.
   It is **never resolved from the routing**, and a model never grades itself: a
   run in which any task it certifies has the judge's family as its candidate is
   refused before the first paid call, naming those tasks, so a preset binding a
   Claude model names a non-Claude judge.

   For an OpenAI-wire broker — one OpenRouter key reaching every open-weight
   model — add the endpoint, which `openai_compatible` fails closed without:

   ```bash
   make e2e-ai TASK=cold_start \
     MODEL=openai_compatible:z-ai/glm-5.2 \
     BASE_URL=https://openrouter.ai/api
   ```

   `PROFILE=` names the environment class a record is filed under
   (`cloud_frontier`, the default, `eu_hosted` or `sovereign`). It is enforced:
   `sovereign` refuses a cloud vendor, and `eu_hosted` a broker candidate that
   `UPSTREAM=` does not pin to EU-region hosts (`{"only":["mistral/eu"]}`).
   Under `ROUTING=` it is **ignored**: a record's profile is part of its
   identity, so it comes from the file that named the models.

2. The provider's **BYOK key in the environment** — e.g. `GEMINI_API_KEY`,
   `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENAI_COMPATIBLE_API_KEY` (the
   OpenRouter example reads that last one). Keys live in the env, never in the
   config file (a stray `api_key:` there is a boot error). Keep them in a
   gitignored `.env.local` and `source` it.
3. No database: the lane runs on the DB-less local router, so no `make db-up`.

## 1. Certify a task

```bash
make e2e-ai TASK=cold_start MODEL=gemini:gemini-3.1-flash-lite
```

This certifies **the model you name**, not any binding this installation holds.
It runs every scenario in the task's corpus `N` times (an odd number, with
response caching off so every run is a fresh model call), judges each answer, and
prints the verdict:

```text
cold_start: certified (reliability=1.00 judge_score_p50=100 self_judged=false)
```

`self_judged` is `true` when candidate and judge are the **same model family**
(publisher or model line; older records flagged only an exact match). It is not a
failure and does not change the verdict, but it weakens the *score*: read such a
band as the deterministic pass (what the production validator accepted) plus an
opinion the candidate has an interest in. A passing run writes/refreshes a record
under `backend/internal/compose/aicert/records/<task>/<provider>_<model>_<env>.json`.

To certify **what a deployment binds** rather than one model you typed, point
`ROUTING=` at that deployment's config (path read from the repo root):

```bash
make e2e-ai ROUTING=config/margince.dev.yaml
```

It resolves a model per task from that config's `seeds.ai_routing` and logs the
resolution before it spends — task, leading rung, model bound there — so a run
against a config you have not read is still not a run against a binding you
cannot see. Records land under the resolved models' own names, so one run writes
several.

The **task** names come from the contract (`backend/api/ai-tasks.yaml`), and only
a task it marks `status: shipped` can be certified — including `cert_judge`,
since the rubric judge is certified like any other task. Read the list off the
build rather than from a copy here: `make ai-probe ARGS='list'` prints every
shipped site from the same census the report enumerates. Omit `TASK=` to run the
whole corpus.

A `planned` task (`nl_search`, `transcript`) owns no scenarios, so naming it fails
with `task "…" has no scenarios under corpus`: a scenario for a prompt nobody
ships would score a copy (`aicert/corpus_test.go` holds that both ways).

A task is not one prompt. `cold_start` ships four invocation **sites** and
`voice_build` three, each with its own scenarios; `TASK=` selects the task, so
certifying one runs every site it ships, and §3's report breaks the result back
down per site.

## 2. Benchmark a candidate swap

Certify a *different* model against the same corpus — change `MODEL=`, leave
`JUDGE=` where it is, so the two runs differ in exactly one thing:

```bash
make e2e-ai TASK=cold_start MODEL=gemini:gemini-3.5-flash
```

Certify both the incumbent and the candidate, then compare their records before
you change the binding.

The binding carries its own endpoint, so an `openai_compatible` candidate is the
same one-liner with `BASE_URL=` added (the Prerequisites example above).
A broker slug may carry its own variant suffix (`:free`, `:batch`, `:thinking`);
the provider/model split cuts at the FIRST colon, so
`openai_compatible:openai/gpt-oss-20b:free` binds the whole slug.

Other knobs: `RUNS=5` (odd repeat count), `PROFILE=` (environment class),
`JUDGE_BASE_URL=` for an `openai_compatible` judge you name — the OpenRouter host
is the default only for the default judge, and unset it falls back to
`BASE_URL=`, since a judge on the candidate's broker is the common case.
A broker binding is served under production's upstream default (fp16/bf16 hosts
only); `UPSTREAM='{}'` and `JUDGE_UPSTREAM='{}'` lift it. Each record names what
applied and any `thinking_level`, crediting only presets set alike. A pre-flight
call per binding makes a key, slug or preference no host serves fail in seconds.

## Choosing a judge transport

`JUDGE=provider:model` grades through a provider adapter, billed per token;
`JUDGE=claude_cli:<model>` (`sonnet`, or an id like `claude-sonnet-4-6`) grades
through `claude -p` on a Claude Code subscription. It needs the CLI on `PATH`
and `CLAUDE_CODE_OAUTH_TOKEN` (`claude setup-token`, read from `.env.local`;
`ANTHROPIC_API_KEY` works too). Each call runs from an empty directory with no
tools, no settings and the judge's system prompt in place of the CLI's, and the
record names the model the CLI reports serving. Use it when a subscription beats
per-token grading: it costs 2–5 s of start-up a call, counts against the plan's
limits, has no temperature or max-token control, and refuses Claude candidates.

## 3. Read the readiness report

```bash
make e2e-ai-report
```

Free, no network: it reads the census, the corpus and the JSON under `records/`,
and prints one row per shipped invocation site — including the sites nothing has
ever certified, which is why it enumerates the census rather than the records:

```text
AI certification readiness: 1 of 36 shipped sites carry a current record.

SITE                      SCOPE            STATUS   SCENARIOS  BAND       PROVIDER  MODEL             ENV        RUNS  PASSED  RELIABILITY  ACCEPTED  WRONG_ANSWER  INVALID  ABSTAINED
agent_loop/morning_brief  single_turn      absent   -          -          -         -                 -          -     -       -            -         -             -        -
cold_start/acts           single_turn      current  3/3        certified  gemini    gemini-3.5-flash  eu_hosted  3     3       1.00         3         0             0        0
cold_start/company        single_turn      partial  9/10       certified  gemini    gemini-3.5-flash  eu_hosted  27    27      1.00         27        0             0        0
rate_extract/pricing      full_invocation  stale    2/3        certified  gemini    gemini-3.5-flash  eu_hosted  3     3       1.00         3         0             0        0
```

**Every row's numbers are that SITE's own.** A record is written per task and a
task can ship several sites — `cold_start` ships four — so the record carries each
scenario's own counts and the row folds the ones that ran on its site. A site the
record never ran a scenario on reads `absent`, not as its sibling's numbers.
`RUNS`/`PASSED` is how often the site did what its scenarios asked; the four
columns after `RELIABILITY` are what the site's own validator **reported**, never
a pass/fail column — a run can be `ACCEPTED` and still fail, when the scenario
asked for an abstention.

Four states, and they never collapse into each other:

- **`current`** — every scenario this site ships was measured, and each one's
  stamp is the one this build computes, so the band describes the request this
  build actually sends. A stamp covers the scenario, the request the site's own
  code builds from it, and how a run is graded (the grader's request and rule).
- **`partial`** — everything the record measured is still current, and the corpus
  has since grown cases it has never seen. Explicitly **not** stale: the record is
  wrong about nothing, merely incomplete, and clearing it costs the new scenarios
  rather than the whole task.
- **`stale`** — a scenario the record *did* measure has changed since, or the code
  that turns it into a prompt did, or how a run is graded did. The band describes
  requests or grading this build no longer uses; re-certify that task.
- **`absent`** — nothing has ever been measured. The columns are dashes rather
  than zeroes, because a zero is a result and this is not one.

Only `current` counts toward the headline count: a `partial` has a measurement you
can read plus an unpaid remainder. `SCENARIOS` is `measured/total` — how many of
this site's *current* scenarios the record still describes, out of how many the
corpus ships today — which is what makes a `partial` actionable, since `9/10` and
`1/10` are the same word and very different bills. A scenario the corpus has
since **dropped** counts in neither half: nobody can re-run it.

**Per-scenario stamps make re-certification affordable.** A record carries each
scenario's own stamp beside the task-level `PromptVersion` (their fold), so a new
scenario in a ten-scenario task reads `partial 9/10` and costs one re-run. A
record older than those stamps is judged by its task stamp and reads `-` there.

`SCOPE` is how much of the site a run covers, from the most to the least:

- **`full_invocation`** — the run drives the whole production invocation, so
  certifying it certifies the site.
- **`single_turn`** — the scenario seeds the window and grades the one reply that
  follows; the surrounding conversation or tool loop is supplied, not exercised.
- **`single_call`** — the run makes ONE of the calls the site makes for one
  invocation. Where the site re-asks a below-floor item, asks again after an
  unreadable answer, or fans out over pages, the answer the product serves is
  assembled from calls the run never made, by a fold equally unmeasured.

**Every row is one (provider, model, env) binding.** A `certified` band
green-lights that deployment and says nothing about another, which is why the
binding sits in the row. The report is a view for a human release decision, not a
gate: it always exits 0, because the lane it reports on is paid and manual.

## 4. See the prompts — trace request/response for tuning

When a task lands `not_supported` or `supported_degraded`, the verdict alone
doesn't tell you *why*. The payload trace reads back exactly what each model saw
and said, and is ON by default: every candidate **and** judge call is dumped to a
JSONL file under the repo-root `.tmp/aicert/` (gitignored), path printed to
stdout:

> **Except a `no_payload` task**, whose content the contract forbids retaining
> whatever the capture posture says (`ai.NoPayload` — today the counterparty
> verdict, which judges other contacts's mail). Its calls carry no payload, so the
> trace has no line for them and the run's `WARN … did not pass its
> validator/caps gate` detail is the only evidence of what went wrong. That is
> the prohibition working, not a gap to widen.

```text
aicert: payload trace → /…/margince-next/.tmp/aicert/aicert-trace-20260719T054005Z.jsonl
```

One JSON object per call, in the **same shape as the `ai_call_payload` table** —
`request_payload` (system + messages) and `response_payload`, both run through the
*same* SecretStripper that guards egress. Scrubbing is not why this is safe on by
default — payloads are written in full. It is safe because every `corpus/`
fixture is synthetic by rule and the file is local-only and gitignored; trace
real input (`make ai-probe`) and you write that input to disk. Each line also carries `role` (`candidate`/`judge`),
`task`, `scenario`, `run`, `call`, `served_model` and the token/latency numbers, so
you can pinpoint the failing run — and the failing call inside it, since a site
may answer in several:

```json
{"task":"enrich","role":"candidate","scenario":"…","run":1,"call":1,
 "served_model":"gemini-3.5-flash",
 "request_payload":{"system":"…","messages":[…]},
 "response_payload":"{\"fields\":[{\"field\":\"title\",\"value\":\"Head of Quality\",\"evidence_snippet\":\"heads up quality assurance\"…"}
```

That `evidence_snippet` is a paraphrase, not a span the signature states
character-for-character — so the site's own evidence gate drops the field and the
run fails on a reply that is perfectly well-formed. That is the typical find: a
`not_supported` verdict driven by a reply the site's own validator refuses, not a
quality problem.

The trace is **on by default** because the corpus is a fixed, hand-authored
scenario set and the content is post-stripper and local-only — there is nothing
to leak. `TRACE=<dir>` picks a directory; `TRACE=` (empty) turns it off.

## 5. When the network drops mid-run

A record covers a whole task, so a fault anywhere in one used to discard every
run already paid for — twenty-one real model calls lost to the twenty-second.
Two things now stand in the way.

**The run is re-driven** when the router comes back having failed on every bound
tier — three attempts, waiting 2s then 8s. Only an exhausted ladder is retried: a
validator failure or a caps miss is a *measurement*, and an exhausted account is
a human's to fix. A withheld answer fails the run ungraded, naming the filter; a
rejected request stops the task with no record. A run is re-driven whole, since a
conversation or tool loop cannot resume mid-way.

**Every scored run is journaled** to `.tmp/aicert/resume/` as it is scored, so a
restart replays what it can (`… run(s) replayable`) instead of paying again. A
journaled run stands in for a fresh one only when nothing it measured can have
moved: the **same candidate binding, judge, profile, corpus version, scenario
stamp, binary and repeat index**, and **within six hours**. Edit a prompt and
that scenario's stamp moves; rebuild after tightening a site's validator and the
binary moves, which the stamp does *not* cover — either way those runs are
measured again. A replayed run passes the same served-identity and degrade gates
a live one does, so a resumed record is the same measurement.

On by default and gitignored, like the trace. `RESUME=<dir>` relocates it;
`RESUME=` measures everything fresh. A journal cut off mid-write still replays
every whole run before the cut. One run owns a directory at a time — parallel
`TASK=` runs each need their own `RESUME=<dir>`, and a killed run leaves a
`.lock` to delete.

## How the verdict is decided

Each run either **HardPasses** — the site's own production validator accepted
the reply, the reply is the answer the scenario expects, and the run stayed
inside its token/latency caps — or fails. The judge scores the answer 0–100
against the rubric. `certified` takes a pooled pass **rate** (so the bar does not
rise with the corpus), a majority of every scenario's own runs (so one case that
always fails cannot hide in the pool), and every scenario's median and minimum
score at its bands; `supported_degraded` a pooled majority and medians at
`degraded_min`; anything else, including a scenario no judge scored, is
`not_supported`. The numbers are in [The exact rule](../reference/ai-certification.md#how-the-scoring-works),
and every one lives in [`thresholds.go`](../../backend/internal/compose/aicert/thresholds.go):
edit it there, bump `gradingRule`, and regenerate the page. **reliability** is the
fraction of runs that HardPassed (0–1), the number to trend. A run whose served
model is not uniform (a fallback, between runs or calls) **voids** the record: you cannot certify a moving target.

A run is not always one model call — a site may retry, fall back, or turn a tool
loop — and everything the run is judged and charged for is pooled across all of
them: any degraded call degrades the run, and the caps, tokens, latency and cost
are the run's totals.

## Notes

- **Reasoning models think before they answer.** Gemini 2.5 / o-series spend
  output tokens on internal thinking that counts against `maxOutputTokens`; the
  lane gives both candidate and judge headroom so a thinking burst doesn't starve
  the answer into `MAX_TOKENS`. Leave room for it in a tight `caps.max_tokens`.
- **Markdown-fenced JSON** is tolerated, and records are committed artifacts.

## When certification passes but the field does not

A record measures a model against the CORPUS fixture, so a site certified at 1.00
can still fail on production input (`rate_extract/pricing`, certified on a
two-line fixture, failed on a 530 KB catalog). Run a site against real input with
[debug an AI task](debug-an-ai-task.md) (`make ai-probe`).
