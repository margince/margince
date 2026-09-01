# Certify an AI model

Prove a model is good enough for a Margince AI task **before** you bind it in
production — and benchmark a candidate swap against the one you run today. The
certification lane (`compose/aicert`) drives a hand-authored **fixture** corpus
through a real model — each site's own production request builder and production
validator, never a copy of either — scores each answer with a pinned rubric
judge, folds the runs into a `certified` / `supported_degraded` /
`not_supported` verdict, and commits the result as a JSON record.

This is the **paid, opt-in** lane: it makes real provider calls billed to your own
**BYOK** (bring-your-own-key) budget, since Margince runs no inference of its
own. It is a developer/CI tool, never part of a request path.

> **Start free.** `make e2e-ai-report` ([§3](#3-read-the-readiness-report)) needs
> no key, no network and no database: it prints what every shipped site's record
> already says, including the ones nothing has ever certified. Read that before
> you spend anything — it tells you whether the run you are about to pay for is
> the one that is actually missing.

See also [explanation/ai-runtime.md](../explanation/ai-runtime.md) (how the
runtime works), [connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md)
(binding a provider) and [add-an-ai-task.md](add-an-ai-task.md) (adding a task or
site rather than certifying one).

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

   `JUDGE=provider:model` is the second model that grades the answers, **always
   required and never resolved from the routing**: `cert_judge` is itself a task
   and leads at `premium`, so a config binding a model there would make the
   grader collide with every `premium`-led candidate. A judge equal to any
   resolved candidate is refused — a model grading itself is certified by
   construction — and every resolved task is checked up front rather than failing
   midway through a paid corpus.

   For an OpenAI-wire broker — one OpenRouter key reaching every open-weight
   model — add the endpoint, which `openai_compatible` fails closed without:

   ```bash
   make e2e-ai TASK=cold_start \
     MODEL=openai_compatible:z-ai/glm-5.2 \
     BASE_URL=https://openrouter.ai/api \
     JUDGE=gemini:gemini-3.1-pro
   ```

   `PROFILE=` names the environment class a record is filed under (`eu_hosted`,
   the default, `sovereign` or `cloud_frontier`). It is enforced, not a label: a
   cloud vendor under `sovereign` is refused rather than run. Under `ROUTING=` it
   is **ignored** and the profile is the config file's own — the profile is part
   of a record's identity and part of what the binding is validated against, so
   it has to come from the file that named the models.

2. The provider's **BYOK key in the environment** — e.g. `GEMINI_API_KEY`,
   `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, `OPENAI_COMPATIBLE_API_KEY` (the
   OpenRouter example reads that last one). Keys live in the env, never in the
   config file (a stray `api_key:` there is a boot error). Keep them in a
   gitignored `.env.local` and `source` it.
3. No database. The lane runs on the DB-less local router, so `make db-up` is
   not required.

## 1. Certify a task

```bash
make e2e-ai TASK=cold_start \
  MODEL=gemini:gemini-3.1-flash-lite \
  JUDGE=anthropic:claude-sonnet-4-6
```

This certifies **the model you name**, not any binding this installation holds.
It runs every scenario in the task's corpus `N` times (an odd number, with
response caching off so every run is a fresh model call), judges each answer, and
prints the verdict:

```text
cold_start: certified (reliability=1.00 score_p50=100 self_judged=false)
```

`self_judged` is `true` when the candidate and the judge resolved to the **same
served model** on every run — the model graded its own answers. It is not a
failure and does not change the verdict, but it weakens the *score*: read such a
band as the deterministic pass (what the production validator accepted) plus an
opinion the candidate has an interest in. A passing run writes/refreshes a record
under `backend/internal/compose/aicert/records/<task>/<provider>_<model>_<env>.json`.

To certify **what a deployment binds** rather than one model you typed, point
`ROUTING=` at that deployment's config (path read from the repo root):

```bash
make e2e-ai ROUTING=config/margince.dev.yaml JUDGE=anthropic:claude-sonnet-4-6
```

It resolves a model per task from that config's `seeds.ai_routing` and logs the
resolution before it spends — task, leading rung, model bound there — so a run
against a config you have not read is still not a run against a binding you
cannot see. Records land under the resolved models' own names, which is why one
such run can write several.

The **task** names come from the contract (`backend/api/ai-tasks.yaml`), and only
a task it marks `status: shipped` can be certified — including `cert_judge`,
since the rubric judge is certified like any other task. Read the list off the
build rather than from a copy here: `make ai-probe ARGS='list'` prints every
shipped site from the same census the report enumerates. Omit `TASK=` to run the
whole corpus.

A `planned` task — one the contract declares but nothing implements
(`nl_search`, `transcript`) — owns no scenarios, and naming it fails the run with
`task "…" has no scenarios under corpus`. That is the point: a scenario for a
prompt nobody ships would score a hand-written copy and report the task covered,
so the corpus refuses to carry one and a fitness test (`aicert/corpus_test.go`)
holds it to that in both directions.

A task is not one prompt. `cold_start` ships four invocation **sites** and
`voice_build` three, each with its own scenarios; `TASK=` selects the task, so
certifying one runs every site it ships, and the report in §3 is what breaks a
task's result back down per site.

## 2. Benchmark a candidate swap

Certify a *different* model against the same corpus — change `MODEL=`, leave
`JUDGE=` where it is, so the two runs differ in exactly one thing:

```bash
make e2e-ai TASK=cold_start \
  MODEL=gemini:gemini-3.5-flash \
  JUDGE=anthropic:claude-sonnet-4-6
```

Certify both the incumbent and the candidate, then compare their records before
you change the binding.

The binding carries its own endpoint, so an `openai_compatible` candidate is a
one-liner — `BASE_URL` is required there and empty for a native vendor, which
uses its own default:

```bash
make e2e-ai TASK=cold_start \
  MODEL=openai_compatible:z-ai/glm-5.2 BASE_URL=https://openrouter.ai/api \
  JUDGE=gemini:gemini-3.1-pro
```

A broker slug may carry its own variant suffix (`:free`, `:batch`, `:thinking`);
the provider/model split cuts at the FIRST colon, so
`openai_compatible:openai/gpt-oss-20b:free` binds the whole slug.

Other knobs: `RUNS=5` (odd repeat count), `PROFILE=` (environment class),
`JUDGE_BASE_URL=` when the judge is on a *different* broker — unset, it falls back
to `BASE_URL=`, since a judge on the candidate's broker is the common case and
re-typing the host was the common mistake.

## 3. Read the readiness report

```bash
make e2e-ai-report
```

Free, no network: it reads the census, the corpus and the JSON under `records/`,
and prints one row per shipped invocation site — including the sites nothing has
ever certified, which is the whole reason it enumerates the census rather than
the records:

```text
AI certification readiness: 1 of 36 shipped sites carry a current record.

SITE                  SCOPE            STATUS   SCENARIOS  BAND       PROVIDER  MODEL             ENV        RUNS  PASSED  RELIABILITY  ACCEPTED  WRONG_ANSWER  INVALID  ABSTAINED
agent_loop/loop       single_turn      absent   -          -          -         -                 -          -     -       -            -         -             -        -
cold_start/acts       single_turn      current  3/3        certified  gemini    gemini-3.5-flash  eu_hosted  3     3       1.00         3         0             0        0
cold_start/company    single_turn      partial  9/10       certified  gemini    gemini-3.5-flash  eu_hosted  27    27      1.00         27        0             0        0
rate_extract/pricing  full_invocation  stale    2/3        certified  gemini    gemini-3.5-flash  eu_hosted  3     3       1.00         3         0             0        0
```

**Every row's numbers are that SITE's own.** A record is written per task and a
task can ship several sites — `cold_start` ships four — so the record carries each
scenario's own counts and the row folds the ones that ran on its site. A site the
record never ran a scenario on reads `absent`, not as its sibling's numbers.
`RUNS`/`PASSED` is how often the site did what its scenarios asked; the four
columns after `RELIABILITY` are what the site's own validator **reported** and are
not a pass/fail column, since a run can be `ACCEPTED` and still fail, when the
scenario asked for an abstention.

Four states, and they never collapse into each other:

- **`current`** — every scenario this site ships was measured, and each one's
  stamp is the one this build computes, so the band describes the request this
  build actually sends. A stamp covers all three parts of that claim: the
  scenario, the request the site's own code builds from it, and the request the
  grader those scores come from is sent.
- **`partial`** — everything the record measured is still current, and the corpus
  has since grown cases it has never seen. Explicitly **not** stale: the record is
  wrong about nothing, merely incomplete, and clearing it costs the new scenarios
  rather than the whole task.
- **`stale`** — a scenario the record *did* measure has changed since, or the code
  that turns it into a prompt did, or the grader's own prompt did. The band is a
  claim about requests no longer sent, or scores a grader no longer produces;
  re-certify that task.
- **`absent`** — nothing has ever been measured. The columns are dashes rather
  than zeroes, because a zero is a result and this is not one.

Only `current` counts toward the headline count: a `partial` has a measurement you
can read plus an unpaid remainder. `SCENARIOS` is `measured/total` — how many of
this site's *current* scenarios the record scored and still describes, out of how
many the corpus ships for it today — which is what makes a `partial` actionable,
since `9/10` and `1/10` are the same word and very different bills. A scenario the
record measured and the corpus has since **dropped** counts in neither half:
nobody can re-run it, so it cannot stand in as coverage.

**Per-scenario stamps are what make re-certification affordable.** A record
carries each scenario's own stamp (`ScenarioRecord.Stamp`) beside the task-level
`PromptVersion`, which is simply the fold of them (`aicert.ScenarioStamps` /
`FoldScenarioStamps`). Before that, adding ONE scenario to a ten-scenario task
moved the task stamp and invalidated nine measurements that were still perfectly
true, so clearing it cost a re-run of all ten; now the same edit reads
`partial 9/10` and costs one. The guarantee is finer rather than weaker: a
scenario's stamp still covers the scenario whole plus both requests this build
constructs from it, so a record still cannot describe a scenario, or a prompt, it
did not measure. A record written before those stamps existed carries none, is
judged by its task stamp exactly as before — `current` or `stale`, never
`partial` — and reads `-` under `SCENARIOS`.

`SCOPE` is how much of the site a run covers, from the most to the least:

- **`full_invocation`** — the run drives the whole production invocation, so
  certifying it certifies the site.
- **`single_turn`** — the scenario seeds the window and grades the one reply
  that follows; the surrounding conversation or tool loop is supplied, not
  exercised. The turns it leaves out are their own answers.
- **`single_call`** — the run makes ONE of the calls the site makes for one
  invocation. Where the site re-asks a below-floor item, asks again after an
  unreadable answer, or fans out over pages, the answer the product serves is
  assembled from calls the run never made — and the fold that assembles them is
  unmeasured too.

**Every row is one (provider, model, env) binding.** A `certified` band
green-lights that deployment and says nothing about another one, which is why the
binding sits in the row rather than in the file name only. The report is a view
for a human release decision, not a gate: it always exits 0, because the lane it
reports on is paid, manual and BYOK-gated.

## 4. See the prompts — trace request/response for tuning

When a task lands `not_supported` or `supported_degraded`, the verdict alone
doesn't tell you *why*. Turn on the payload trace to read exactly what each
model saw and said:

```bash
make e2e-ai TASK=enrich \
  MODEL=gemini:gemini-3.1-flash-lite \
  JUDGE=anthropic:claude-sonnet-4-6   # trace is ON by default
```

Every candidate **and** judge call is dumped to a JSONL file under the
repo-root `.tmp/aicert/` (gitignored), and the path is printed to stdout:

> **Except a `no_payload` task**, whose content the contract forbids retaining
> whatever the capture posture says (`ai.NoPayload` — today the counterparty
> verdict, which judges other people's mail). Its calls carry no payload, so the
> trace has no line for them and the run's `WARN … did not pass its
> validator/caps gate` detail is the only evidence of what went wrong. That is
> the prohibition working, not a gap to widen.

```text
aicert: payload trace → /…/margince-next/.tmp/aicert/aicert-trace-20260719T054005Z.jsonl
```

One JSON object per call, in the **same shape as the `ai_call_payload` table** —
`request_payload` (system + messages) and `response_payload`, both run through the
*same* SecretStripper that guards egress, so a credential in a prompt is scrubbed
before it reaches disk. Each line also carries `role` (`candidate`/`judge`),
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

## How the verdict is decided

Each run either **HardPasses** — the site's own production validator accepted
the reply, the reply is the answer the scenario expects, and the run stayed
inside the scenario's token/latency caps — or fails. The judge scores the
answer 0–100 against the scenario's rubric. `N` runs of one scenario fold into a verdict
against the scenario's score bands (spec §5):

| Verdict | Rule |
|---|---|
| `certified` | **every** run HardPasses ∧ median score ≥ `certified_min` ∧ min score ≥ `floor` |
| `supported_degraded` | ≥ ⌈2N/3⌉ runs HardPass ∧ median score ≥ `degraded_min` |
| `not_supported` | otherwise |

**reliability** is the fraction of runs that HardPassed (0–1), reported for every
verdict — the number to trend over time. A run whose served-model identity is not
uniform (a fallback to another model, between runs or between the calls of one
run) **voids** the record: you cannot certify a moving target.

A run is not always one model call — a site may retry, fall back, or turn a tool
loop — and everything the run is judged and charged for is pooled across all of
them: any degraded call degrades the run, and the caps, tokens, latency and cost
are the run's totals.

## Notes

- **Reasoning models think before they answer.** Gemini 2.5 / o-series spend
  output tokens on internal thinking that counts against `maxOutputTokens`; the
  lane gives both candidate and judge headroom so a thinking burst doesn't starve
  the answer into a `MAX_TOKENS` stop. If you author a tight `caps.max_tokens`,
  leave room for it.
- **Markdown-fenced JSON** is tolerated: the lane unfences ` ```json ` blocks the
  same way production parsers do.
- Records are committed artifacts — the proof travels with the code. Re-running
  refreshes latency/token numbers (network noise); the verdict is durable.

## When certification passes but the field does not

A record measures a model against the CORPUS fixture. A site can be certified at
reliability 1.00 and still fail on the input production actually hands it — the
model-cost refresh did exactly that against a 530 KB provider catalog while
`rate_extract/pricing` was certified on a two-line fixture. To run a site against
real input through the same code, use [debug an AI task](debug-an-ai-task.md)
(`make ai-probe`).
