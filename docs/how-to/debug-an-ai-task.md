<!-- prose:plain -->
# Debug an AI task against real input

`make ai-probe` runs one production call site against input you give it, through the same code that
production runs. It reports, as numbers, every step between that input and the verdict.

## When to reach for it

| you want to know | use |
|---|---|
| Is this model good enough for this prompt? | `make e2e-ai`: scores a fixed corpus, writes a record |
| Does this site hold up on **this** input? | **`make ai-probe`**: one site, your input, no score, no record |
| Which sites have a certification record? | `make e2e-ai-report`, or the page generated from the same three trees: [reference/ai-certification.md](../reference/ai-certification.md) |

The two answer different questions. A site can be `certified` at `reliability` 1.00 on a corpus fixture of
two lines. It can still fail every time on the page of 500 KB that production gives it. The
certification measured the fixture correctly. A green record says nothing about an input that is not in
the corpus.

The probe is cheap: `list`, `scaffold` and `fetch` cost nothing, and `run --ai-fake` costs nothing. Only
`run` against a real binding calls a model. It makes one call, with no judge and no record.

## 1. Find the site

```bash
make ai-probe ARGS='list'
```

```text
SITE                                  KIND        SCOPE            LADDER                   CORPUS
rate_extract/fx                       one_shot    full_invocation  premium,cheap_cloud      yes
agent_loop/morning_brief              agent_loop  single_turn      cheap_cloud,premium      yes
capture_classify/classify             one_shot    full_invocation  local_small,cheap_cloud  yes
```

The list comes from the census (`compose.NewTaskCensus()`, built from `tasks_gen.go`), so it cannot drift
from the contract. Read the **SCOPE** column first; see
[What a probe does not cover](#what-a-probe-does-not-cover).

`LADDER` lists where a call starts, and where it goes next on a provider or schema failure
(`ai.TaskLadder(task)`). Other rungs can answer too. Under cost limits, the router also *degrades*,
and `degrade_to` reaches rungs that the ladder never names.

For example, the ladder of `draft_reply` is `[cheap_cloud, premium]`, and `cheap_cloud` degrades to
`local_small`. So a model bound at `local_small` can end up serving `draft_reply`.
`ai.ServableTiers(task)` is the full set: the ladder, plus every rung `degrade_to` reaches, step by step,
with the ladder rungs first. `ai.LeadingTier(task)` is the first rung: what serves when nothing has failed.
To ask which of your bound models could answer a task, use the full set. The ladder by itself gives too
small an answer.

## 2. Get a starting fixture

Each site takes a fixture of a different shape: `page_text` here, `pages[].text` there, and nothing like a
web page at all for `capture_classify`. Do not read the Go types; copy the corpus scenario of the site:

```bash
make ai-probe ARGS='scaffold rate_extract/fx'
# → .tmp/aitask/rate_extract_fx.yaml
```

Edit the `fixture:` block, keep the shape, then run it:

```bash
make ai-probe ARGS='run --scenario ../.tmp/aitask/rate_extract_fx.yaml --ai-fake'
```

The files land in `.tmp/aitask/`, which git does not track. A page you fetched, or a real fixture, holds
whatever the source has. A probe must not leave customer content where a commit would pick it up.
`--out -` writes to stdout; `--out <path>` puts it where you ask.

## 3. Give it real input

`fetch` runs the production fetcher, and prints what comes out of the fetch step. That is HTML cut down by
`StripTags`, and Markdown and JSON as they are.

A site does not always get that output as it is. A route may cut it down more before it builds its
request, and `fetch` shows you the input to that step.

```bash
make ai-probe ARGS='fetch https://api.frankfurter.dev/v1/latest'
```

```text
fetched  media=application/json  bytes=214  passages=1  markdown=false  json=true
```

Read `passages=`. Passages are what `numberPassages` makes, one for each line that is not empty. A
row the model pulls out names them as evidence. A body served as one long line counts as *one* passage,
whatever its size. Then every row names `[s0]`, and the evidence gate has nothing to disagree with. A
count of bytes does not show that.

Then build a fixture and probe. `--fixture` takes JSON, so a long body never has to come through a YAML
paste:

```bash
jq -n --rawfile t .tmp/aitask/fetch-api.frankfurter.dev_v1_latest.txt \
  '{base_currency:"EUR",tracked_currencies:["USD","GBP"],page_text:$t}' > .tmp/aitask/fx.json

make ai-probe ARGS='run --site rate_extract/fx \
  --fixture ../.tmp/aitask/fx.json \
  --expect  ../.tmp/aitask/expect.json \
  --model anthropic:claude-sonnet-4-6'
```

You name the model of the probe, and **it reads no routing file**. The binding of the installation is a
stored setting. A new install gets it from `seeds.ai_routing`, and you change it under Settings → AI. This
lane opens no database to read it from.

Pass one of two flags. `--model provider:model` puts one pinned model behind the full routed pipeline.
`--ai-fake` uses the fake that needs no network.

`--model` carries a provider and a model, and has no field for a **host**. So you cannot probe an
`openai_compatible:…` model that a broker serves: that binding fails closed without a base URL. Pin a
vendor with its own client here, and use `make e2e-ai … BASE_URL=…` when the question is about the broker.

### Some sites need `--expect`

`--fixture` carries what production gets; `--expect` carries what you claim about the reply. Several
sites check the expected answer **before** they call the model. `rate_extract/fx` refuses one that is not
a map from currency to rate. `agent_loop` refuses a step name that no declared tool could reach. Those
sites need `--expect` or `--scenario`:

```text
failed    rate_extract/fx: the expected answer is not a map of currency code to its rate against the base: unexpected end of JSON input
          (no expectation was supplied; this site validates one — use --expect or --scenario)
```

That is the site's own message. The probe never makes up an expected answer to get past it.

## 4. Read the report

```text
site      rate_extract/fx   kind=one_shot   scope=full_invocation
binding   model override anthropic:claude-sonnet-4-6   ladder [premium,cheap_cloud]
caveat    company context not declared for this site
fixture   241 B

call 1
  request   system 1290 B  payload 268 B  passages 1  ~67 tok  max_tokens 1024  schema 402 B
  response  in 702 tok  out 96 tok  388 B  served=claude-sonnet-4-6  tier=premium  2.114s

evaluate  accepted
```

| line | what it tells you |
|---|---|
| `scope=` | how much of production this run covered; read it every time |
| `binding` | which model was pinned (`--model`, or the fake), and the ladder of tiers behind it. The pin is bound to every rung of that ladder, so the probe never fails as "no bound tier can serve" |
| `caveat` | company context that this lane, with no database, could not build |
| `request` | the size of the system prompt and of the payload, the **passage count**, and the output limit |
| `response` | the tokens you pay for, the model that served, the tier that answered, and the time of the call |
| `evaluate` | what the **production validator** said about the reply |

Four things to know:

- **`HIT CAP` is a guess.** `model.Response` carries no reason why the model stopped. So the probe reads it
  from `OutputTokens >= MaxTokens`. A model that stopped at the limit looks the same as one that was cut
  off. The report prints it as a flag next to the plain numbers, and makes no claim about why the provider
  stopped. A site whose answer grows with its input reaches it long before anything else fails.
- **`~N tok` is `bytes/4`.** It reads about 25% under the real count on JSON with few spaces. Use it to compare sizes
  against a context window and an output cap; it is not what you pay.
- **`served=` trusts what the provider said answered** over what the routing bound. A vendor that swaps in
  another model without saying so explains many results that look wrong.
- **`CACHED` would mean the call never happened.** The probe turns off the result cache, for the same
  reason the certification lane does, so you should never see it. If you do, a second run was served from
  the cache, and not measured.

### `invalid`, `wrong_answer` and `failed`

The report keeps three problems separate:

- **`failed`**: the *probe itself* failed, for example on a refused fixture or a model that does not
  answer. It returns a code other than 0.
- **`invalid`**: the production validator refused the reply, because its form was wrong or it named no
  evidence.
- **`wrong_answer`**: the validator accepted a reply with the right form, but it says something other than
  what you expected.

`wrong_answer` often means **your expected answer is wrong**. A run against a page priced in EUR can
come back with:

```text
evaluate  wrong_answer — "USD" is priced 0.9259259259 against the base where the scenario expects 1.08
```

The page said `1 EUR = 1.08 USD`; the sheet stores the price of one USD in EUR, 0.9259259259. The reply and
the base of the site were right. The expected answer, written by hand the way the page reads it, was
wrong. Check the source before you say the model is wrong.

## What a probe does not cover

Two limits come from the certification seam itself. The probe prints both on every run, so that no one
reads a green probe as more than it covered.

**Scope** (`aitasks.ScopeOf`, also in `make e2e-ai-report`):

- `full_invocation`: the whole production call path (`rate_extract/*`, `site_extract/profile`,
  `draft_reply/reply`, `enrich/signature`, `offer_draft/draft`, `voice_build/*`, …).
- `single_turn`: the fixture fills the chat up to one point, and one reply is graded. That is each
  `agent_loop` site, which is one scheduled agent graded on its own goal and tools. It is also each
  `cold_start` site with more than one turn.
- `single_call`: one of the many calls the site makes (`capture_classify/classify`,
  `capture_counterparty_verdict/verdict`).

**Company context is never built**, because the lane has no database. It is declared for `agent_loop`,
`draft_reply`, `offer_draft` and `summarize`. For those sites you probe without part of the real prompt,
and the `caveat` line says so.

## Tuning a prompt

1. Run with `--dump-request <dir>` to write each request, after `SecretStripper`, as JSON.
2. Edit the request builder of the site in `internal/compose/certcase_*.go`, or the production code it
   calls.
3. Run it again, and diff. `--json <path>` gives the whole result in a form a program can read.

The file from step 1 is the one you diff a prompt edit against.

```bash
make ai-probe ARGS='run --scenario ../.tmp/aitask/s.yaml --ai-fake --dump-request ../.tmp/aitask/before'
# …edit the prompt…
make ai-probe ARGS='run --scenario ../.tmp/aitask/s.yaml --ai-fake --dump-request ../.tmp/aitask/after'

# Paths inside ARGS='…' are relative to backend/ (the root Makefile delegates
# with -C backend); your own shell is in the repo root, so diff has no ../.
nonce='s/untrusted-[0-9a-f-]{36}/untrusted-NONCE/g'
diff <(sed -E "$nonce" .tmp/aitask/before/*.request.json) \
     <(sed -E "$nonce" .tmp/aitask/after/*.request.json)
```

**You must replace the nonce.** Every call makes a new `untrusted-<uuid>` mark on each side of the text
that Margince does not trust. It names that mark in both the system prompt and the payload. The mark makes a fake mark inside
a fetched page do nothing, so it must be different for each call. So two runs of the same prompt always
differ in two places. The files stay right about what was sent; the change happens in the diff.

The corpus prompts are pinned byte for byte. A change to a prompt we ship moves the stamp of every scenario
built from it. So `make e2e-ai-report` shows that site as `stale` until it is certified again. To add a
*scenario* costs less: the record stays right about what it measured, and reads `partial`. Only the new
case has to be paid for.

## Keep a finding

A scenario you probed is yours, and stays in `.tmp/`. If the build should keep measuring it, make it a
corpus scenario that you commit. The guide [Write a certification case](write-a-certification-case.md) covers the
source fields (`source`, `sanitized_by`) that the corpus requires and the probe does not.

## Flags

One set of flags serves every verb. So a verb accepts a flag it has no use for, and does nothing with
it (`list --site x` prints the whole table). The verb column shows what each flag changes.

| flag | verb | |
|---|---|---|
| `--site <task>/<variant>` | run, scaffold | which site to probe (needed with `--fixture`) |
| `--scenario <file.yaml>` | run | the fixture and the expected answer, in the form the corpus uses |
| `--fixture <file.json>` / `--expect <file.json>` | run | the two parts, each on its own |
| `--model provider:model` / `--ai-fake` | run | one of the two; `--ai-fake` is free. No routing file: only a vendor with its own client, because `--model` carries no host |
| `--json <path\|->` | run | the whole result, in a form a program can read |
| `--dump-request <dir>` | run | each request, with secrets removed |
| `--out <path\|->` | scaffold, fetch | where the file of this verb goes |
| `--work-dir <dir>` | scaffold, fetch | where files go (by default `.tmp/aitask`, which git does not track); `run` writes only to the paths that `--json` / `--dump-request` name |
| `--corpus <dir>` | list, scaffold | the corpus to read |

The probe reads the BYOK key from `.env.local` at the root of the repository, the same way `make e2e-ai`
reads it.
