<!-- prose:plain -->
# Certify an AI model

Prove a model is good enough for a Margince AI task **before** you bind it in production. You can also
measure a candidate model against the one you run today, before you swap them. The certification lane
(`compose/aicert`) sends a **fixture** corpus that a human wrote through a real model. It uses the
production request builder and the production validator of each site, never a copy of either.

The lane scores each answer with a pinned rubric judge. It folds the runs into a `certified` /
`supported_degraded` / `not_supported` verdict, and commits the result as a JSON record.

This lane **costs money**, and runs only when you ask for it. It makes real provider calls, paid from
your own **BYOK** key, because Margince runs no model of its own. The judge can use a Claude Code plan
instead. The lane is never part of a request path.

> **Start for free.** `make e2e-ai-report` ([step 3](#3-read-the-report)) needs no key,
> network or database. It prints what the record of every site we ship says, also for the sites with no
> certified record. Read it before you pay for a run.

See also [ai-runtime.md](../explanation/ai-runtime.md), [connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md), [add-an-ai-task.md](add-an-ai-task.md),
[certify-a-decision-site.md](certify-a-decision-site.md) (the decisions lane) and [reference/ai-certification.md](../reference/ai-certification.md), the page these records fill.

## Prerequisites

1. **Name what to certify.** Name one of two things; there is no default:

   - `MODEL=provider:model`: one candidate, which the run binds to every task under test. Use it to compare two
     models: change the model, keep all the rest the same, and compare.
   - `ROUTING=<deployment config>`: a **deployment**. The run certifies each task against every model
     its ladder binds in the `seeds.ai_routing` of that config, with one record each. That covers the
     rung that answers, and each fallback that a failed call falls to. The run skips a fallback in the
     family of the judge, and names it. It reports a ladder that binds nothing, because one empty tier
     must not cost every other record.

   The run refuses both at once: one names a deployment, the other one candidate. Neither reads the
   binding of the *installation*. That binding is the `ai.routing` setting, and this lane opens no
   database. `ROUTING=` reads what a new install would get as its *seed* data.

   `JUDGE=provider:model` is the second model, which grades the answers. **One judge grades every task of
   a run**, because a new judge changes verdicts on its own. The default is
   `claude_cli:claude-sonnet-4-6`, graded through `claude -p` on a Claude Code plan. It needs `claude`
   on your `PATH`, and `CLAUDE_CODE_OAUTH_TOKEN`.

   An exported `MARGINCE_AICERT_JUDGE_MODEL` replaces the default, and `JUDGE=` replaces both. For
   example, use `openai_compatible:anthropic/claude-sonnet-4.6` with `JUDGE_UPSTREAM='{}'` (the same
   model, paid for each call), or `gemini:gemini-3.5-flash`.

   The run **never takes the judge from the routing**, and a model never grades itself. The run refuses,
   before the first paid call, when any task it certifies has a candidate in the family of the judge. It
   names those tasks. So a preset that binds a Claude model names a judge that is not Claude.

   For a broker on the OpenAI wire, such as one OpenRouter key that reaches every model with open weights, add
   the endpoint. Without it, `openai_compatible` fails closed:

   ```bash
   make e2e-ai TASK=cold_start \
     MODEL=openai_compatible:z-ai/glm-5.2 \
     BASE_URL=https://openrouter.ai/api
   ```

   `PROFILE=` names the kind of setup that a record is filed under: `cloud_frontier` (the default),
   `eu_hosted` or `sovereign`. `sovereign` refuses a cloud candidate, but not a cloud judge. Under
   `ROUTING=` the run **does not read** `PROFILE=`. The profile is part of what a record is, so it comes
   from the file that named the models.

2. Put the **BYOK key of the provider in the environment**, such as `GEMINI_API_KEY`.

   Other names are `ANTHROPIC_API_KEY`, `OPENAI_API_KEY` and `OPENAI_COMPATIBLE_API_KEY`. The OpenRouter
   example reads the last one. Keys live in the environment, never in the config file: an `api_key:`
   there is an error at start. Keep them in a `.env.local` file that git does not track, and `source` it.
3. You need no database. The lane runs on the local router, which has no database, so skip `make db-up`.

## 1. Certify a task

```bash
make e2e-ai TASK=cold_start MODEL=gemini:gemini-3.1-flash-lite
```

This certifies **the model you name**, whatever binding this installation holds. It runs every
scenario in the corpus of the task `N` times. The cache for answers is off, so every run is a new model
call. It runs a scenario near the pass line more often (see below). Then it judges each answer and prints the
verdict:

```text
cold_start: certified (reliability=1.00 judge_score_p50=100 self_judged=false)
```

`self_judged` is `true` when the candidate and the judge are the **same model family**: from the same
company, or the same model line. Old records marked only a model that matched in full. It does not fail the
run or change the verdict, but it means the *score* counts for less. Read such a band as two things. One is the
fixed pass: what the production validator accepted. The other is the score of a judge from the same
family as the candidate.

A run that passes writes a new record, or changes the old one, under
`backend/internal/compose/aicert/records/<task>/<provider>_<model>_<env>.json`.

To certify **what a deployment binds**, and not one model you typed, point `ROUTING=` at the config of
that deployment. The run reads the path from the root of the repository:

```bash
make e2e-ai ROUTING=config/margince.dev.yaml
```

It logs the rungs of each task (task, tier, model) before it spends money, and writes a record for
each model. `STALE_ONLY` is **on by default**. The run skips a model whose record is already current
for this build, so a full run pays only for what is missing or stale. `STALE_ONLY=0` measures it again.

The **task** names come from the contract (`backend/api/ai-tasks.yaml`). Only a task it marks
`status: shipped` can be certified. That includes `cert_judge`, because the rubric judge is certified
like any other task. Read the list from the build: `make ai-probe ARGS='list'` prints every site we
ship, from the same census the report reads. Leave out `TASK=` to run the whole corpus.

A `planned` task (`nl_search`, `transcript`) has no scenarios, so a run that names it fails with
`task "…" has no scenarios under corpus`. A scenario for a prompt that we do not ship would score a
copy, and `aicert/corpus_test.go` holds the rule both ways.

A task can ship more than one call **site**: `cold_start` ships four and `voice_build` ships three.
Each site has its own scenarios. `TASK=` picks the task, so a run that certifies one task runs every
site it ships. The report of step 3 then splits the result per site.

## 2. Compare a candidate before you swap

Certify a *different* model against the same corpus. Change `MODEL=` and leave `JUDGE=` as it is, so
the two runs change in one thing only:

```bash
make e2e-ai TASK=cold_start MODEL=gemini:gemini-3.5-flash
```

Certify both the model you run today and the candidate. Then compare their records before you change
the binding.

The binding has its own endpoint. So an `openai_compatible` candidate needs the same one line, with
`BASE_URL=` added (see the example in Prerequisites above). A broker slug may end in its own tag,
such as `:free`, `:batch` or `:thinking`. The run splits provider from model at the first `:`, so
`openai_compatible:openai/gpt-oss-20b:free` binds the whole slug.

Other settings:

- `RUNS=5` sets the runs in the first pass; 9 or more turns off the added runs. `PROFILE=` sets the kind of setup.
- `JUDGE_BASE_URL=` sets the host for an `openai_compatible` judge. When it is not set, the judge uses
  the `BASE_URL=` of the candidate (or `MARGINCE_AICERT_BASE_URL`). It uses the OpenRouter host only
  when neither is set.
- Production serves a broker binding under its default upstream rule, under which only `fp16` or `bf16`
  hosts serve. `UPSTREAM='{}'` and `JUDGE_UPSTREAM='{}'` turn it off. Each record names the rule in use, and any
  `thinking_level`. It counts only for presets with the same setting.
- A first test call for each binding makes a bad key, a bad slug, or a rule that no host serves fail in
  seconds.

## Choose how the judge runs

`JUDGE=provider:model` grades through the client for that provider, and you pay for each token.
`JUDGE=claude_cli:<model>` (`sonnet`, or an ID such as `claude-sonnet-4-6`) grades through `claude -p`
on a Claude Code plan. It needs the CLI on your `PATH`, and `CLAUDE_CODE_OAUTH_TOKEN`. Get that token
from `claude setup-token`; the run reads it from `.env.local`. `ANTHROPIC_API_KEY` works too.

Each call runs from an empty folder, with no tools and no settings. The system prompt of the judge
stands in for the one of the CLI. The record names the model that the CLI reports it served.

Use it when a plan costs less than to pay for each token. Each call costs 2–5 seconds to start, and counts
against the limits of the plan. It has no setting for `temperature` or for the most tokens. It refuses
Claude candidates.

## 3. Read the report

```bash
make e2e-ai-report
```

It is free, and needs no network. It reads the census, the corpus and the JSON under `records/`, and
prints one row for each call site we ship. It reads the census, not the records, so it also lists the
sites that nothing has certified yet:

```text
AI certification readiness: 1 of 36 shipped sites carry a current record.

SITE                      SCOPE            STATUS   SCENARIOS  BAND       PROVIDER  MODEL             ENV        RUNS  PASSED  RELIABILITY  ACCEPTED  WRONG_ANSWER  INVALID  ABSTAINED
agent_loop/morning_brief  single_turn      absent   -          -          -         -                 -          -     -       -            -         -             -        -
cold_start/acts           single_turn      current  3/3        certified  gemini    gemini-3.5-flash  eu_hosted  3     3       1.00         3         0             0        0
cold_start/company        single_turn      partial  9/10       certified  gemini    gemini-3.5-flash  eu_hosted  27    27      1.00         27        0             0        0
rate_extract/fx           full_invocation  stale    2/3        certified  gemini    gemini-3.5-flash  eu_hosted  3     3       1.00         3         0             0        0
```

**Each row counts only its own site.** The run writes one record per task, and a task
can ship more than one site. So the record holds the counts of each scenario, and the row adds up the
runs on its site. A site where the record never measured a scenario reads `absent`. It does not
show the numbers of another site.

`RUNS`/`PASSED` is how often the site gave what its scenarios asked for. The four columns after
`RELIABILITY` are what the validator of the site **reported**. None of them is a pass or fail column. A
run can be `ACCEPTED` and still fail, when the scenario asked the model to hold back its answer.

There are four states, and each one means something different:

- **`current`**: the run measured every scenario this site ships, and the stamp of each one is the stamp
  this build makes. So the band describes the request this build sends. A stamp covers three
  things: the scenario, the request the site builds from it, and how a run is graded. That last one is
  the request and the rule of the grader.
- **`partial`**: all that the record measured is still current, but the corpus has new cases since then.
  A `partial` record is wrong about nothing; it only misses the new cases. To fix it costs only the new
  scenarios, not the whole task.
- **`stale`**: a scenario the record *measured* has changed since then. Or the code that turns it into
  a prompt changed, or how a run is graded changed. The band describes requests or grades this build
  does not use any more, so certify that task again.
- **`absent`**: the run has measured nothing yet. The columns show `-`, not `0`, because a `0` would be a
  result.

Only `current` adds to the count in the first line. A `partial` has numbers you can read, plus a part
you have not paid for yet. `SCENARIOS` is `measured/total`: how many of the *current* scenarios of this
site the record still describes, out of how many the corpus ships today. That tells you what to do
with a `partial`, because `9/10` and `1/10` are the same word but two different costs. A scenario the
corpus has since **dropped** counts in neither number, because no one can run it again.

**Scenario stamps make it cheap to certify again.** A record holds the stamp of each
scenario next to the `PromptVersion` of the task, which folds them all. So a new scenario in a task of
10 scenarios reads `partial 9/10`, and costs one more run. A record older than those stamps is judged
by its task stamp, and reads `-` there.

`SCOPE` is how much of the site a run covers, from the most to the least:

- **`full_invocation`**: the run covers the whole call path of production, so to certify it is to
  certify the site.
- **`single_turn`**: the scenario fills the chat up to one point, and grades the one reply that comes next.
  The run gives the chat or tool loop before it, and does not test it.
- **`single_call`**: the run makes one of the calls the site makes for one use. The site may ask again
  for a value below the floor, ask again after an answer it cannot read, or split the work over pages.
  Then the answer the product serves is built from calls the run never sent, by a fold that nothing
  measured either.

**Every row is one (provider, model, `env`) binding.** A `certified` band lets that deployment go live,
and says nothing about another one. That is why the binding is in the row. The report helps a human
decide on a release, and it gates nothing. It always returns 0, because the lane it reports on is paid
and you run it by hand.

A `gemini_vertex` rung with no record of its own is graded by the `gemini` record for the same model,
under `cloud_frontier`. Vertex serves the same model on the same wire, so it gets the request
that record measured. The page marks each such grade "measured on `gemini`". Once you pay for a Vertex
run, it grades its rungs in place of the `gemini` record.

## 4. See the prompts — trace request/response for tuning

When a task ends at `not_supported` or `supported_degraded`, the verdict by itself does not tell you *why*.
The payload trace reads back what each model was sent and said, and it is on by default. The run writes every
candidate **and** judge call to a JSONL file under `.tmp/aicert/` at the root of the repository. That
folder is out of git. The run prints the path:

> **Not for a `no_payload` task.** The contract does not let the run keep its content, whatever the
> capture setting says (`ai.NoPayload`). Today that is the counterparty verdict, which judges mail from
> other senders. Its calls have no payload, so the trace has no line for them. The run then
> logs `WARN … did not pass its validator/caps gate`, and that line is the only sign of what failed.

```text
aicert: payload trace → /…/margince-next/.tmp/aicert/aicert-trace-20260719T054005Z.jsonl
```

There is one JSON line for each call, in the **same shape as the `ai_call_payload` table**:
`request_payload` (system and messages) and `response_payload`. Both pass through the same
`SecretStripper` that takes secrets out of data that leaves the system. The trace is on by default because the corpus
fixture is not real data, and the file is local and out of git. To trace real input with
`make ai-probe` writes that input to a file.

Each line also holds `role` (`candidate`/`judge`), `task`, `scenario`, `run`, `call`, `served_model`,
and the numbers for tokens and time. You can find the run that failed, and the call inside it that
failed, because a site may answer in more than one call:

```json
{"task":"enrich","role":"candidate","scenario":"…","run":1,"call":1,
 "served_model":"gemini-3.5-flash",
 "request_payload":{"system":"…","messages":[…]},
 "response_payload":"{\"fields\":[{\"field\":\"title\",\"value\":\"Head of Quality\",\"evidence_snippet\":\"heads up quality assurance\"…"}
```

That `evidence_snippet` says the signature in other words, and does not copy it letter for letter. So
the evidence gate of the site drops the field, and the run fails on a reply whose form is right. That is
the usual find. The `not_supported` verdict comes from a reply the validator of the site refuses, and
nothing is wrong in the answer itself. `TRACE=<dir>` picks a folder; `TRACE=` (empty) turns it off.

## 5. When the network drops in the middle of a run

A network error does not waste runs you already paid for.

**The run starts again** when the router comes back with a failure on every tier it binds. It tries
three times, and waits 2 seconds and then 8 seconds. The run tries again only when the whole ladder failed:

- A validator failure, or a caps miss, is a *measure*, and the run keeps it.
- An empty account is for a human to fix.
- When a filter holds back an answer, the run fails with no grade, and names the filter.
- When a request is refused, the task stops with no record.

A run starts again from the start, because a chat or tool loop cannot pick up in the middle. An answer
that the upstream **breaks off** (`ai.ErrAnswerAbandoned`) starts again too. It becomes an `invalid` run
with no grade only when every try breaks off, and its scenario row counts it as `abandoned`. That means
the run reached the model, and the model could not end its answer.

**The run writes every scored run** to `.tmp/aicert/resume/` as soon as it is scored. So a new start replays
what it can (`… run(s) replayable`), and does not pay again. A stored run stands in for a new one
only when nothing it measured can have changed:

- the **same candidate binding, judge, profile, corpus version, scenario stamp, binary and run
  number**;
- and **within 6 hours**.

Edit a prompt, and the stamp of that scenario changes. Build again after you make a validator stricter,
and the binary changes, which the stamp does *not* cover. Either way the run measures those runs again.
A replayed run passes the same gates as a live one, for the model that served it and for a degraded
answer. So a record built from stored runs is the same measure.

It is on by default, and out of git, like the trace. `RESUME=<dir>` sets its folder; `RESUME=` measures
everything again. A stored file cut off in the middle of a write still replays every whole run before the
cut. One run owns a folder at a time. So runs of more than one `TASK=` at once each need their own
`RESUME=<dir>`. A run that ends early leaves a `.lock` file to delete.

## How the verdict is decided

A run **HardPasses** when three things hold. The validator of the site accepted the reply, it is the
answer the scenario expects, and it is inside its caps. The judge scores each run from 0 to 100, up
to three times, and the run takes the middle score. The band rules, the score lines, and the veto rule are in
[reference/ai-certification.md](../reference/ai-certification.md#how-the-scoring-works). Every number
is in [`thresholds.go`](../../backend/internal/compose/aicert/thresholds.go): edit it, change
`gradingRule`, and generate again.

`reliability` is the share of runs with a HardPass, and the number to watch over time. When the model
that served a run is not the same across runs or calls, the record **does not count**.

A run is not always one model call: a site may try again, fall back, or turn a tool loop. All that the
run is judged on and paid for is added up across all of those calls. One degraded call degrades the
run, and the caps, tokens, time and cost are added up for the whole run.

## Notes

- **Reasoning models think before they answer.** Gemini 2.5 and the `o-series` models spend answer tokens on
  thinking, and those count against `maxOutputTokens`. The lane gives both the candidate and the judge
  more tokens, so that long thinking does not cut the answer off at `MAX_TOKENS`. Leave tokens for it in
  a small `caps.max_tokens`.
- The lane accepts **JSON inside a Markdown code block**, and the records are files we commit.

## When certification passes but the field does not

A record measures a model against the corpus fixture. So a site certified at 1.00 can still fail on
production input. A site certified on a fixture of two lines can fail on a page of 530 KB. Run a site
against real input with [debug an AI task](debug-an-ai-task.md) (`make ai-probe`).
