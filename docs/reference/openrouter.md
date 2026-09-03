# OpenRouter: upstream selection

**Tested 2026-09-02** against `openai/gpt-oss-120b` and
`mistralai/mistral-large-2512` through this tree's own certification lane, on
commit `b63dc2c60`. Every figure below is a measurement from that day, not an
estimate. Re-measure before trusting the numbers: OpenRouter's host roster,
their prices and their speeds all move.

Related: [configuration.md](configuration.md) for the environment variables,
[connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md) for
credentials, [config/presets/](../../config/presets/README.md) for a
ready-made binding.

## 1. What the broker does, and why it needs configuring

OpenRouter is not a vendor. It is a gateway in front of many inference hosts,
and a model id names a *set* of them. On the day of the test
`openai/gpt-oss-120b` had **21 endpoints**.

They are not interchangeable:

- **quantization spans fp4 → bf16** — different answer quality
- **`max_completion_tokens` spans 8,192 → 117,964** — a long generation
  truncates on some and not others
- **30-minute uptime spanned 35.1% → 100%**

Its default choice among them is documented as: skip hosts with an outage in
the last 30 seconds, then weight by the **inverse square of price** (a $1/M host
is 9× more likely than a $3/M one). The choice is remade **per request**.

So an unconfigured binding gets a different serving stack per call, and the
product cannot see which. That is what this page exists to fix.

## 2. What that cost us, measured

One certification run of `draft_reply`, 38 candidate calls, no preferences:

| | value |
|---|---|
| upstreams reached | **8** — DeepInfra ×9, AkashML ×6, CoreWeave ×6, SiliconFlow ×5, Novita ×4, BaseTen ×3, Nebius ×3, Google ×2 |
| latency p50 / p90 / p99 | 19.0s / 38.0s / **304.2s** |
| scenarios whose 3 repeats were split across hosts | **8 of 9** |

The last row is the serious one. `RUNS=3` exists to sample one thing three
times and take the median. One scenario —
`first_message_from_an_intent_alone` — was served by **five** different hosts
across its three repeats. A median over five serving stacks at two precisions is
not a median of anything, and the record reported it as one number.

**The control that proves it is the judge.** `mistral-large-2512` ran in the
same runs, through the same adapter and the same HTTP client, at p90 2.8s
against the candidate's 38s. It has **two** endpoints and both are first-party
Mistral. There was never a lottery to lose.

## 3. The default this product ships

A binding whose `base_url` is an OpenRouter host and which declares no
`routing:` block inherits:

```yaml
routing:
  sort: throughput
  quantizations: [fp16, bf16]
  require_parameters: true
```

Reliability over price — deliberately the **inverse** of the broker's own
default. Measured against the same corpus, same night:

| `draft_reply` | baseline | with the default |
|---|---|---|
| latency p50 | 19,041ms | **1,102ms** |
| latency p90 | 38,038ms | **1,992ms** |
| latency p99 | 304,203ms | **3,697ms** |
| upstreams reached | 8 | **1** (Cerebras) |
| repeats split across hosts | 8 of 9 | **0 of 9** |
| mean output tokens | 670 | 701 |
| whole run, wall clock | 1,268s | **156s** |

17× at p50, 82× at p99, and the reproducibility defect gone. Mean output tokens
went slightly *up*, so it is not faster for having answered less.

It generalizes. `cold_start`, run the same way:

| `cold_start` | baseline | with the default |
|---|---|---|
| latency p50 / p90 | 10,191ms / 22,204ms | **788ms / 1,107ms** |
| upstreams reached | 8 | **1** |
| repeats split | 7 of 9 | **0 of 9** |
| wall clock | 463s | **119s** |

The certification record for `draft_reply` also moved: reliability
`0.963 → 1.0`, `reported_invalid 1 → 0`, `score_p50 75 → 85`. Treat the score
gain as suggestive rather than settled — it is one run of 27 against a record
from a different day.

**Cost: roughly 2.4× per call.** Taken from OpenRouter's own `usage.cost`, not
from our `est_cost_microusd` — see §7.

### Why these three

- **`sort: throughput` is the lever.** It is what collapses the tail. With it
  set, the other two change almost nothing measurable.
- **`quantizations` buys comparability, not speed.** Pinning precision is what
  makes repeated calls comparable at all. It earns its place as a *guardrail*:
  the day the fastest host drops out, the sort would otherwise fall to an fp4
  host and answer quality would shift with nothing to show it.
- **`require_parameters` makes a soft preference a rule.** OpenRouter already
  prefers hosts supporting `response_format`; this stops it being a preference.

### Opting out

Three states, and the last two are different:

| written | means |
|---|---|
| no `routing:` key | the default above |
| `routing: {}` | **explicitly nothing** — the broker's own price-weighted routing |
| `routing: {…}` | exactly what is written |

The distinction survives the settings store, because an omitted key and a
written `{}` are different JSON.

## 3b. Validated through the config path

The figures in §3 were taken with the preferences injected by hand. They were
then re-taken through `ROUTING=config/presets/openrouter_cloud.yaml`, where the
default is *inherited from config* rather than supplied — which is what a
deployment actually does:

| | `draft_reply` | | `cold_start` | |
|---|---|---|---|---|
| | baseline | via config | baseline | via config |
| p50 | 19,041ms | **1,016ms** | 10,191ms | **743ms** |
| p90 | 38,038ms | **1,413ms** | 22,204ms | **1,421ms** |
| p99 | 304,203ms | 76,902ms | 39,464ms | **5,857ms** |
| upstreams | 8 | **1** | 8 | **1** |
| split repeats | 8 of 9 | **0 of 9** | 7 of 9 | **0 of 9** |

The `draft_reply` p99 is a single 76.9-second call — on the pinned host, with
`finish_reason: stop` and 718 tokens, so a normal response the gateway sat on.
Excluding it: p50 972ms, p90 1,380ms, max 2,632ms. That is §6's second tail,
which this default does not address and is not meant to.

Note the judge for these runs was `gemini-3.5-flash`, not the `mistral-large`
the earlier records used: the preset binds `mistral-large` at `premium`, and the
runner refuses a judge that leads a task it would also grade. Latency and
upstream attribution are judge-independent; **scores across the two are not
comparable.**

## 4. Hard filters versus soft preferences

**This is the distinction that matters most.** A hard filter removes a host from
the candidate set. A soft preference only reorders it. **Only a hard filter can
bound a tail.**

| field | effect | hard? |
|---|---|---|
| `only` / `ignore` | allow/blocklist by slug | **hard** |
| `quantizations` | serving precision | **hard** |
| `require_parameters` | hosts supporting every parameter sent | **hard** |
| `max_price` | `{prompt, completion}` $/M; *blocks the request* if nothing qualifies | **hard** |
| `sort` | `throughput` \| `price` \| `latency`; **disables load balancing** | reorders |
| `preferred_max_latency_p90` | percentile cutoff, rolling 5-min window | **soft** |
| `allow_fallbacks` | host switching on failure; default true | — |

Not academic. In the A/B, the arm that set **only** the soft latency
preference (`preferred_max_latency_p90: 8`) was the **worst arm measured**:
p90 43.7s against the baseline's 38s, with one call taking **231 seconds**.
Asking politely for speed was worse than asking for nothing.

## 5. What was tried and rejected

Two interleaved A/B rounds, 8 arms × 20 samples each, 320 samples total. Arms
were round-robined rather than run one at a time, so the broker's own load
change through the night could not be attributed to whichever arm was running.
Round 2, with `A_baseline` and `G_nitro` repeated from round 1 as drift
controls:

| arm | p50 | p90 | p99 | $/1k calls | upstreams |
|---|---|---|---|---|---|
| baseline *(control)* | 4,683 | 21,188 | 31,841 | 0.661 | 7 hosts |
| `:nitro` suffix *(control)* | 1,437 | 5,306 | 7,174 | 1.272 | Cerebras ×20 |
| `only: [cerebras]` | 1,422 | 1,897 | 2,550 | 1.336 | Cerebras ×20 |
| **the shipped default** | 1,392 | **1,861** | **2,180** | 1.351 | Cerebras ×20 |
| default + `reasoning_effort: low` | **1,071** | **1,677** | **1,794** | **0.866** | Cerebras ×20 |
| default + `session_id` | 1,476 | 2,881 | 6,660 | 1.289 | Cerebras ×20 |
| `only: [cerebras, groq]` | 2,609 | 3,513 | 3,857 | 0.971 | Groq ×14, Cerebras ×6 |
| `sort` + `max_price` | 2,702 | **141,484** | **386,985** | 1.068 | Groq ×11, Cerebras ×9 |

**`max_price` is excluded from the default.** Its p99 was **387 seconds**, the
worst arm of either round: a price ceiling cannot exclude what the sort then
prefers.

**`only`/`ignore` are excluded.** They reach the same host as the sort while
throwing away the failover breadth a sort leaves intact.

**Prefer `sort: throughput` over the `:nitro` suffix.** All three of `:nitro`,
`only: [cerebras]` and the default landed on Cerebras ×20, yet `:nitro`'s p90
was ~3× the other two. It also carries priority-service-tier eligibility. And
the suffix form **fails open on a typo**: `gpt-oss-120b:baseten` was silently
ignored and routed to CoreWeave — no error. (`@baseten` and `/baseten` at least
return a 400.)

**`reasoning_effort: low` is an operator knob, not a default.** It won every
latency percentile *and* cost 36% less — and cost **20 points of certification
score** (`score_p50` 85 → 65) with mean output falling 961 → 259 tokens.
Structural reliability stayed 1.0: the answers still parse, validate and pass
their caps. They are simply worse answers.

> This is the methodological warning worth keeping. On latency and cost alone
> that arm won everything, and shipping from that evidence would have introduced
> a 20-point quality regression **invisibly** — no error, no failed cap, no
> changed verdict count. Only the graded lane could see it. A latency benchmark
> cannot choose a model configuration for a quality-sensitive task.

**`session_id` pins without making anything faster.** A stable key is
OpenRouter's sticky-routing key and it works exactly as documented — 20/20
samples on one host in both rounds. But it pins to whatever host it landed on
*first*: in round 1 that was Parasail, a mid-latency fp4 host, and p50 stayed
8.9s. It is a reproducibility instrument, and a candidate for getting
reproducibility *without* narrowing the candidate set. This tree does not send
it yet.

## 6. Gateway hangs are a separate tail

2 of 56 samples (3.6%) hung past a 400-second client deadline — one on the
baseline, **one on the fastest arm**. Both returned bodies of pure whitespace:
the gateway pads the connection with newlines to hold it open, then never
answers.

So there are two independent tails. Upstream choice explains the 30–90s band and
the preferences above collapse it. A gateway stall is arm-independent, and only
a client deadline plus a retry bounds it — `ai.CallCeiling` (300s) and the
certification lane's 3-attempt re-drive are what carried the runs through.

OpenRouter's own guidance agrees: provider-layer failover is automatic, but a
gateway-level incident needs client-side retry.

## 7. Do not judge the cost from `est_cost_microusd`

Between the two `draft_reply` runs our own figure moved only **264 → 268
microUSD**, which reads as "free". It is priced from `ai_model_rate`, which keys
on **model** — and the entire price difference here is **per upstream**
(Cerebras $0.35/M prompt against CoreWeave $0.03/M, ~11×).

OpenRouter returns the true figure as `usage.cost` on every response and this
tree does not read it. Until it does, the honest cost of this default is the
A/B's own column: **~2.4×**.

## 8. What the trace records

Since the change that added this page's subject, `ai_call` carries:

- **`served_provider`** — the upstream that actually served, from the response's
  `provider` field. Empty on a direct vendor, which reports none.
- **`finish_reason`** — so a truncated answer and a complete one are different
  rows.

`served_identity_source` deliberately stays `'echo'` for this wire: the
broker's `model` field is our own request reflected back, so we now know **who**
served without knowing **what** they served. Collapsing the two would launder an
echo into a report.

The certification lane's payload trace carries the same two per call, which is
what makes a run attributable at all.

## 9. Postmortem: why this took so long to find

It presented as flaky model behaviour for months and was none of those things.

1. **The evidence was never recorded.** `openAICompatChatResponse` decoded
   `model`, `content` and `usage` and nothing else. Eight hosts therefore looked
   like one. Every symptom — a 300-second call, a score that moved between runs,
   an occasional empty answer — was real, attributable, and had nowhere to be
   attributed *to*.
2. **The honest label was already there and was not enough.** `servedSource`
   correctly tagged this wire `echo`, and the adapter's own comment said the
   model field "merely reflects back the requested model id". The tree knew the
   field was untrustworthy; nobody had added the field that *is*.
3. **The stable control was mistaken for a well-behaved model.** The judge's p90
   of 2.8s was read as "the judge is fine". It was two endpoints versus 21.
4. **Aggregates hid it.** Pooling all models across a run put the fast Gemini
   tiers next to the broker calls and dragged the median down by an order of
   magnitude. The first honest comparison needed filtering to one model *and*
   one task.
5. **A latency fix nearly became a quality regression.** See the block quote in
   §5.
6. **Populating a field exposed a check that would have silently passed.**
   Reading `reasoning_tokens` for the first time revealed that some hosts report
   more reasoning tokens than completion tokens (DeepInfra 817 vs 787, Parasail
   1117 vs 1069, AkashML 1234 vs 1121). The certification lane grades an answer
   as `TokensOut - ReasoningTokens`; that goes negative, and a negative answer
   count can never exceed a `max_tokens` cap. The ceiling would have looked
   present and never fired. The adapter now bounds the reported value by the
   completion it breaks down. Other hosts err the other way — BaseTen reported
   0 reasoning tokens on a response whose reasoning text was plainly there — so
   the field is bounded, never invented.
7. **A paid call could return silence.** A reasoning model spends its output
   budget thinking before the answer starts, both charged to the same cap, so a
   cap that binds mid-thought returns `content: null` with every generated token
   under `reasoning`. Reading `content` alone returned empty text with **no
   error** — so no retry, no log. Confirmed once in 225 calls, at the one call
   site in the tree that opts out of `ai.ReasoningOutputMaxTokens`, whose comment
   describes this exact failure.

The through-line: **every one of these was invisible rather than wrong.** No
exception, no failing test, no red check. The lesson worth carrying is the one
in `AGENTS.md` about a census that can fail short — a measurement that cannot
see a defect reports success, and success is indistinguishable from correctness
until somebody records the missing field.

## 10. Re-evaluating later

The raw data, the harness and the working notes are not in this tree — they were
session artefacts under `.tmp/openrouter-stability/`, which is deliberately not
committed. What is reproducible from here:

```sh
# One task, baseline then tuned, back to back.
make e2e-ai TASK=draft_reply RUNS=3 \
  MODEL=openai_compatible:openai/gpt-oss-120b \
  BASE_URL=https://openrouter.ai/api \
  JUDGE=openai_compatible:mistralai/mistral-large-2512 \
  JUDGE_BASE_URL=https://openrouter.ai/api \
  TRACE=.tmp/aicert RESUME=

# The same, with preferences, via UPSTREAM= (JSON of one ai.OpenRouterRouting):
make e2e-ai TASK=draft_reply RUNS=3 ... \
  UPSTREAM='{"sort":"throughput","quantizations":["fp16","bf16"],"require_parameters":true}'
```

Then compare `served_provider` and `latency_ms` across the two traces, per task
and per model — pooling either one hides the effect (§9.4).

The host roster is worth re-reading before each round:

```sh
curl -s https://openrouter.ai/api/v1/models/openai/gpt-oss-120b/endpoints \
  | jq '.data.endpoints[] | {provider_name, quantization, max_completion_tokens, uptime_last_30m}'
```

Two things to re-check specifically, because both would change the default:
whether Cerebras is still the throughput leader, and whether it still serves at
fp16 — the `quantizations` clause admits it today, and a re-quantized host would
silently fall out of the candidate set.
