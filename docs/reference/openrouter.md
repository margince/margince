# OpenRouter: upstream selection

**Tested 2026-09-02** against `openai/gpt-oss-120b` and
`mistralai/mistral-large-2512` through this tree's own certification lane, on
commit `b63dc2c60`. Every figure below is a measurement from that day.
Re-measure before trusting the numbers: OpenRouter's host roster, their prices
and their speeds all move.

Related: [configuration.md](configuration.md) for the environment variables,
[connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md) for
credentials, [config/presets/](../../config/presets/README.md) for a
ready-made binding.

## 1. What the broker does, and why it needs configuring

OpenRouter is a gateway in front of many inference hosts, and a model id names
a *set* of them. On the day of the test, 21 hosts served `openai/gpt-oss-120b`.

They are not interchangeable:

- **quantization spans fp4 → bf16**, so answer quality differs
- **`max_completion_tokens` spans 8,192 → 117,964**, so a long generation
  truncates on some and not others
- **30-minute uptime spanned 35.1% → 100%**

Its documented default choice among them: skip hosts with an outage in the last
30 seconds, then weight by the inverse square of price (a $1/M host is 9× more
likely than a $3/M one). The choice is remade per request.

So an unconfigured binding gets a different serving stack per call, and the
product cannot see which. The routing settings on this page fix that.

## 2. What that cost us, measured

One certification run of `draft_reply`, 38 candidate calls, no preferences:

| | value |
|---|---|
| upstreams reached | **8**: DeepInfra ×9, AkashML ×6, CoreWeave ×6, SiliconFlow ×5, Novita ×4, BaseTen ×3, Nebius ×3, Google ×2 |
| latency p50 / p90 / p99 | 19.0s / 38.0s / **304.2s** |
| scenarios whose 3 repeats were split across hosts | **8 of 9** |

The last row is the serious one. `RUNS=3` exists to sample one thing three
times and take the median. One scenario, `first_message_from_an_intent_alone`,
was served by five different hosts across its three repeats. The record
reported one median over five serving stacks at two precisions.

The judge is the control. `mistral-large-2512` ran in the same runs, through the
same adapter and the same HTTP client, at p90 2.8s against the candidate's 38s.
It has two endpoints and both are first-party Mistral, so routing could not
vary.

## 3. The default this product ships

An `openai_compatible` tier on an OpenRouter provider host with no `routing:` inherits:

```yaml
routing:
  provider: {sort: throughput, quantizations: [fp16, bf16], require_parameters: true}
```

It favours reliability over price, the inverse of the broker's own default.
Measured against the same corpus, same night:

| `draft_reply` | baseline | with the default |
|---|---|---|
| latency p50 | 19,041ms | **1,102ms** |
| latency p90 | 38,038ms | **1,992ms** |
| latency p99 | 304,203ms | **3,697ms** |
| upstreams reached | 8 | **1** (Cerebras) |
| repeats split across hosts | 8 of 9 | **0 of 9** |
| mean output tokens | 670 | 701 |
| whole run, wall clock | 1,268s | **156s** |

17× at p50, 82× at p99, and no split repeats. Mean output tokens went slightly
*up*, so the speed does not come from shorter answers.

`cold_start`, run the same way, shows the same effect:

| `cold_start` | baseline | with the default |
|---|---|---|
| latency p50 / p90 | 10,191ms / 22,204ms | **788ms / 1,107ms** |
| upstreams reached | 8 | **1** |
| repeats split | 7 of 9 | **0 of 9** |
| wall clock | 463s | **119s** |

The certification record for `draft_reply` also moved: reliability
`0.963 → 1.0`, `reported_invalid 1 → 0`, `judge_score_p50 75 → 85`. Treat the
score gain as suggestive: it is one run of 27 against a record from a different
day.

**Cost: roughly 2.4× per call.** Taken from OpenRouter's own `usage.cost`, not
from our `est_cost_microusd`; see section 7.

### Why these three

- **`sort: throughput` is the lever.** It collapses the tail. With it set, the
  other two change almost nothing measurable.
- **`quantizations` buys comparability.** Pinning precision makes repeated
  calls comparable at all. It is a guardrail: the day the fastest host drops
  out, the sort would otherwise fall to an fp4 host and answer quality would
  shift with nothing to show it.
- **`require_parameters` makes a soft preference a rule.** OpenRouter already
  prefers hosts supporting `response_format`; this makes it a requirement.

### Opting out

Three states, and the last two are different:

| written | means |
|---|---|
| no `routing:` key | the default above |
| `routing: {}` | **explicitly nothing**: the broker's own price-weighted routing |
| `routing: {…}` | what is written, and nothing else |

### Pinning a region

Neither the default nor `{}` says anything about *where* a call is served. The
broker lists each model's endpoints by slug, and a region is part of the slug.
`mistral/eu` is Mistral's EU endpoint. The base slug `mistral` matches every
region Mistral serves from, and a variant such as `mistral/zdr` names a
retention policy, not a place. Only `only: [<provider>/<region>]` keeps a call
in a region, and OpenRouter answers 404 when no endpoint matches instead of
falling back elsewhere. Read a model's endpoints at
`https://openrouter.ai/api/v1/models/<model id>/endpoints` before binding it. A
model with no EU endpoint cannot be pinned to the EU at all, and the list
changes: `mistral-medium-3-5` had none until the broker added `mistral/eu`.

**Connection keys and tier keys.** Where a request is served belongs to the
connection, and how a model is served belongs to the tier. `only`,
`ignore`, `allow_fallbacks`, `zdr`, `data_collection` and
`enforce_distillable_text` are set once, as `providers.openai_compatible.upstream`
(the OpenRouter settings section of its provider sheet). They reach every lane on
it; the embeddings lane may state its own. The serving keys stay on each tier's
`routing:`, because two models behind one broker need different answers.
[openrouter-routing-fields.md](openrouter-routing-fields.md) lists every field.
OpenRouter's EU address, `https://eu.openrouter.ai/api` (Business or Enterprise
plan), keeps every request in the EU and needs no pin. On the global address,
pin `only` to an EU-region slug on the connection.

## 3b. Validated through the config path

The figures in section 3 were taken with the preferences injected by hand. They
were then re-taken through `ROUTING=config/presets/openrouter_cloud.yaml`, where
the default is inherited from config, as in a deployment:

| | `draft_reply` | | `cold_start` | |
|---|---|---|---|---|
| | baseline | via config | baseline | via config |
| p50 | 19,041ms | **1,016ms** | 10,191ms | **743ms** |
| p90 | 38,038ms | **1,413ms** | 22,204ms | **1,421ms** |
| p99 | 304,203ms | 76,902ms | 39,464ms | **5,857ms** |
| upstreams | 8 | **1** | 8 | **1** |
| split repeats | 8 of 9 | **0 of 9** | 7 of 9 | **0 of 9** |

The `draft_reply` p99 is a single 76.9-second call on the pinned host, with
`finish_reason: stop` and 718 tokens: a normal response the gateway sat on.
Excluding it: p50 972ms, p90 1,380ms, max 2,632ms. That is the second tail
described in section 6, which this default does not address.

The judge for these runs was `gemini-3.5-flash`, not the `mistral-large` the
earlier records used. The preset binds `mistral-large` at `premium`, and the
runner refuses a judge that leads a task it would also grade. Latency and
upstream attribution are judge-independent; scores across the two judges are
not comparable.

## 4. Hard filters versus soft preferences

A hard filter removes a host from the candidate set. A soft preference only
reorders it. Only a hard filter bounds a tail.

| field | effect | hard? |
|---|---|---|
| `only` / `ignore` | allow/blocklist by slug | **hard** |
| `quantizations` | serving precision | **hard** |
| `require_parameters` | hosts supporting every parameter sent | **hard** |
| `max_price` † | `{prompt, completion}` $/M; *blocks the request* if nothing qualifies | **hard** |
| `sort` | `throughput` \| `price` \| `latency`; **disables load balancing** | reorders |
| `preferred_max_latency_p90` | percentile cutoff, rolling 5-min window | **soft** |
| `allow_fallbacks` | host switching on failure; default true | n/a |

† **`max_price` is not settable in this tree.** The table is the broker's field
set, and `OpenRouterRouting` has no member for this row. The routing config is
parsed with `KnownFields(true)`, so an operator who copies it into a `routing:`
block gets a parse error at boot instead of a price ceiling. It is left out
because section 5 measured its p99 at 387 seconds. Adding it would mean a
struct member, a schema property and a parity case.

The soft preference alone did harm. In the A/B, the arm that set only
`preferred_max_latency_p90: 8` was the worst arm measured: p90 43.7s against
the baseline's 38s, with one call taking 231 seconds. It did worse than setting
nothing.

## 5. What was tried and rejected

Two interleaved A/B rounds, 8 arms × 20 samples each, 320 samples total. Arms
were round-robined, not run one at a time, so the broker's own load change
through the night could not be attributed to whichever arm was running. Round
2, with `A_baseline` and `G_nitro` repeated from round 1 as drift controls:

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

**`max_price` is excluded.** It is in neither the default nor the tree (see the
footnote in section 4). Its p99 was 387 seconds, the worst arm of either round:
a price ceiling cannot exclude what the sort then prefers.

**`only`/`ignore` are excluded.** They reach the same host as the sort while
throwing away the failover breadth a sort leaves intact.

**Prefer `sort: throughput` over the `:nitro` suffix.** `:nitro`,
`only: [cerebras]` and the default all landed on Cerebras ×20, yet `:nitro`'s
p90 was ~3× the other two. It also carries priority-service-tier eligibility.
And the suffix form fails open on a typo: `gpt-oss-120b:baseten` was ignored
without an error and routed to CoreWeave. (`@baseten` and `/baseten` at least
return a 400.)

**`reasoning_effort: low` is an operator knob, not a default.** It won every
latency percentile and cost 36% less. It also cost 20 points of certification
score (`judge_score_p50` 85 → 65), with mean output falling 961 → 259 tokens.
Structural reliability stayed 1.0: the answers still parse, validate and pass
their caps. They are worse answers. Latency and cost alone would have chosen
this arm; only the graded lane showed the 20-point quality drop.

**`session_id` pins without making anything faster.** A stable key is
OpenRouter's sticky-routing key and it works as documented: 20/20 samples on
one host in both rounds. But it pins to whatever host it landed on *first*. In
round 1 that was Parasail, a mid-latency fp4 host, and p50 stayed 8.9s. It is a
reproducibility instrument, and a candidate for getting reproducibility
*without* narrowing the candidate set. This tree does not send it yet.

## 6. Gateway hangs are a separate tail

2 of 56 samples (3.6%) hung past a 400-second client deadline: one on the
baseline, one on the fastest arm. Both returned bodies of pure whitespace. The
gateway pads the connection with newlines to hold it open, then never answers.

So there are two independent tails. Upstream choice explains the 30–90s band,
and the preferences above collapse it. A gateway stall is arm-independent, and
only a client deadline plus a retry bounds it. `ai.CallCeiling` (300s) and the
certification lane's 3-attempt re-drive carried the runs through.

OpenRouter's own guidance agrees: provider-layer failover is automatic, but a
gateway-level incident needs client-side retry.

## 7. Do not judge the cost from `est_cost_microusd`

Between the two `draft_reply` runs our own figure moved only 264 → 268
microUSD, which reads as "free". It is priced from `ai_model_rate`, which keys
on model, and the entire price difference here is per upstream (Cerebras
$0.35/M prompt against CoreWeave $0.03/M, ~11×).

OpenRouter returns the true figure as `usage.cost` on every response, and this
tree does not read it. Until it does, use the A/B's own cost column: about
2.4×.

## 8. What the trace records

`ai_call` carries:

- **`served_provider`**: the upstream that served the call, from the response's
  `provider` field. Empty on a direct vendor, which reports none.
- **`finish_reason`**, so a truncated answer and a complete one are different
  rows.
- **`cached_tokens` and `cache_write_tokens`**, read from the response's
  `usage.prompt_tokens_details`, both already inside `prompt_tokens`. An
  upstream that caches (Anthropic through the broker is the one that charges for
  the write) therefore prices at its cache rates, not as plain input.

`served_identity_source` stays `'echo'` for this wire. The broker's `model`
field is our own request reflected back, so we know who served without knowing
what model they served. Reporting the echo as the served model would be false.

The certification lane's payload trace carries the same two per call, so every
run is attributable to a host.

## 9. What hosts report that needs care

- **Reasoning tokens can exceed completion tokens.** Some hosts report more
  reasoning tokens than completion tokens (DeepInfra 817 vs 787, Parasail 1117
  vs 1069, AkashML 1234 vs 1121). The certification lane grades an answer as
  `TokensOut - ReasoningTokens`. Unbounded, that goes negative, and a negative
  answer count never exceeds a `max_tokens` cap, so the cap would never fire.
  The adapter caps the reported value at the completion count. Other hosts err
  the other way (BaseTen reported 0 reasoning tokens on a response whose
  reasoning text was plainly there), so the field is bounded, never invented.
- **A cap that binds mid-thought returns `content: null`.** A reasoning model
  spends its output budget thinking before the answer starts, and both count
  against the same cap. When the cap binds mid-thought, every generated token
  is under `reasoning`, and reading `content` alone returns empty text with no
  error, so no retry and no log. Read `reasoning` too. This was seen once in 225
  calls, at the one call site in the tree that opts out of
  `ai.ReasoningOutputMaxTokens`, whose comment describes this failure.

## 10. Re-evaluating later

The raw data, the harness and the working notes were session artefacts under
`.tmp/openrouter-stability/`, which is not committed.
What is reproducible from here:

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
and per model. Pooling models or tasks hides the effect: fast tiers of another
model drag the median down by an order of magnitude.

The host roster is worth re-reading before each round:

```sh
curl -s https://openrouter.ai/api/v1/models/openai/gpt-oss-120b/endpoints \
  | jq '.data.endpoints[] | {provider_name, quantization, max_completion_tokens, uptime_last_30m}'
```

Re-check two things, because both would change the default: whether Cerebras
is still the throughput leader, and whether it still serves at fp16. The
`quantizations` clause admits it today, and a re-quantized host would drop out
of the candidate set without any error.

## 11. The decisions endpoint

OpenRouter also brokers TypeSafe Jev at
`POST https://openrouter.ai/api/alpha/decisions`, billed on input tokens only.
It is one server on the Jev wire, so it binds the routing config's `decisions:`
lane under `jev_compatible`. That provider host
(`providers.jev_compatible.base_url`) is the full endpoint; nothing is appended.
**Preset: OpenRouter** fills it, and `openrouter_cloud.yaml` carries a commented
block whose lane endpoint is lifted:

```yaml
    decisions:
      provider: jev_compatible
      model: typesafe/jev-1.13
      base_url: https://openrouter.ai/api/alpha/decisions
```

`JEV_COMPATIBLE_API_KEY` carries the OpenRouter key (the `OPENAI_COMPATIBLE_API_KEY`
value). It takes no `routing:` preferences. A bound lane
answers only the sites certified for it; the rest go to the ladder
([ai-runtime.md](../explanation/ai-runtime.md#the-decision-lane)).
