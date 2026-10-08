<!-- prose:plain -->
# OpenRouter: choosing the upstream

**Tested 2026-09-02** against `openai/gpt-oss-120b` and
`mistralai/mistral-large-2512` through the certification lane of this tree, on
commit `b63dc2c60`. Every number below is a measure from that day. Measure again
before you trust the numbers: the list of OpenRouter hosts, their prices and their
speeds all change.

See also: [configuration.md](configuration.md) for the process settings,
[connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md)
for credentials, and [config/presets/](../../config/presets/README.md) for a
binding that is ready to use.

## 1. What the broker does, and why it needs config

OpenRouter is a gateway in front of many model hosts, and a model id names a
*set* of them. On the day of the test, 21 hosts served `openai/gpt-oss-120b`.

One host is not the same as another:

- **quantization goes from `fp4` to `bf16`**, so answer quality is different
- **`max_completion_tokens` goes from 8,192 to 117,964**, so a long answer ends early on
  some hosts and not on others
- **uptime over 30 minutes went from 35.1% to 100%**

Its documents describe its default choice. First it skips hosts that were down in
the last 30 seconds. Then it chooses by price: one over the price times the price. A
host at `$1/M` is 9× more likely than one at `$3/M`. The choice is made again for
each request.

So a binding with no config gets a different serving stack per call, and the
product cannot see which. The routing settings on this page fix that.

## 2. What that cost us, measured

One certification run of `draft_reply`, 38 candidate calls, no preferences:

| | value |
|---|---|
| upstreams reached | **8**: DeepInfra ×9, AkashML ×6, CoreWeave ×6, SiliconFlow ×5, Novita ×4, BaseTen ×3, Nebius ×3, Google ×2 |
| latency `p50` / `p90` / `p99` | `19.0s` / `38.0s` / **`304.2s`** |
| scenarios whose 3 repeats were split over hosts | **8 of 9** |

The last row is the serious one. `RUNS=3` exists to sample one thing three times
and take the median. One scenario, `first_message_from_an_intent_alone`, was
served by five different hosts over its three repeats. The record reported one
median over five serving stacks at two number formats.

The judge is the control. `mistral-large-2512` ran in the same runs, through the
same adapter and the same HTTP client, at `p90` `2.8s` against `38s` for the
candidate. It has two endpoints, and both are run by Mistral itself, so routing
could not change.

## 3. The default this product ships

An `openai_compatible` tier on an OpenRouter provider host with no `routing:`
gets this by default:

```yaml
routing:
  provider: {sort: throughput, quantizations: [fp16, bf16], require_parameters: true}
```

It puts uptime and speed over price, the other way round from the broker's own
default. Measured against the same corpus, the same night:

| `draft_reply` | baseline | with the default |
|---|---|---|
| latency `p50` | `19,041ms` | **`1,102ms`** |
| latency `p90` | `38,038ms` | **`1,992ms`** |
| latency `p99` | `304,203ms` | **`3,697ms`** |
| upstreams reached | 8 | **1** (Cerebras) |
| repeats split over hosts | 8 of 9 | **0 of 9** |
| mean output tokens | 670 | 701 |
| whole run, real time | `1,268s` | **`156s`** |

17× at `p50`, 82× at `p99`, and no split repeats. Mean output tokens went *up*, not
down, so the speed does not come from shorter answers.

`cold_start`, run the same way, shows the same effect:

| `cold_start` | baseline | with the default |
|---|---|---|
| latency `p50` / `p90` | `10,191ms` / `22,204ms` | **`788ms` / `1,107ms`** |
| upstreams reached | 8 | **1** |
| repeats split | 7 of 9 | **0 of 9** |
| real time | `463s` | **`119s`** |

The certification record for `draft_reply` also moved: `reliability` `0.963 → 1.0`,
`reported_invalid 1 → 0`, `judge_score_p50 75 → 85`. Do not read too much into
the higher score: it is one run of 27, against a record from a different day.

**Cost: about 2.4× per call.** This comes from the own `usage.cost` of OpenRouter, not
from our `est_cost_microusd`; see section 7.

### Why these three

- **`sort: throughput` is the setting that counts.** It removes the tail. With it set, the
  other two change close to nothing we can measure.
- **`quantizations` makes calls easy to compare.** Pinning the number format is
  what makes repeated calls possible to compare at all. It is a guard. On the day the fastest host
  drops out, the sort would else move to an `fp4` host. Answer quality would
  then change with nothing to show it.
- **`require_parameters` makes a soft preference a rule.** OpenRouter already
  prefers hosts that support `response_format`; this makes it a requirement.

### Turning it off

Three states, and the last two are different:

| written | means |
|---|---|
| no `routing:` key | the default above |
| `routing: {}` | **nothing, by choice**: the broker's own routing, which goes for low prices |
| `routing: {…}` | what is written, and nothing else |

### Pinning a region

The default and `{}` both say nothing about *where* a call is served. The broker
lists the endpoints of each model by slug, and a region is part of the slug.
`mistral/eu` is the EU endpoint of Mistral. The base slug `mistral` matches every
region Mistral serves from. A slug such as `mistral/zdr` names a retention
policy, not a place.

Only `only: [<provider>/<region>]` keeps a call in a region. OpenRouter answers
404 when no endpoint matches, and does not move to another place. Read the
endpoints of a model at
`https://openrouter.ai/api/v1/models/<model id>/endpoints` before you bind it. A
model with no EU endpoint cannot be pinned to the EU at all. And the list
changes: `mistral-medium-3-5` had none until the broker added `mistral/eu`.

**Connection keys and tier keys.** Where a request is served belongs to the
connection, and how a model is served belongs to the tier. `only`, `ignore`,
`allow_fallbacks`, `zdr`, `data_collection` and `enforce_distillable_text` are
set once, as `providers.openai_compatible.upstream` (the OpenRouter settings part
of its provider sheet). They reach every lane on it; the embeddings lane may state
its own. The serving keys stay on the `routing:` of each tier, because two models
behind one broker need different answers.

[openrouter-routing-fields.md](openrouter-routing-fields.md) lists every field.
The EU address of OpenRouter, `https://eu.openrouter.ai/api` (Business or
Enterprise plan), keeps every request in the EU and needs no pin. On the default
address, pin `only` to a slug for an EU region on the connection.

### Checked through the config path

We took the numbers in section 3 with the preferences put in by hand. We then
took them again through `ROUTING=config/presets/openrouter_cloud.yaml`, where the
default comes from config, as in a real installation:

| | `draft_reply` | | `cold_start` | |
|---|---|---|---|---|
| | baseline | through config | baseline | through config |
| `p50` | `19,041ms` | **`1,016ms`** | `10,191ms` | **`743ms`** |
| `p90` | `38,038ms` | **`1,413ms`** | `22,204ms` | **`1,421ms`** |
| `p99` | `304,203ms` | `76,902ms` | `39,464ms` | **`5,857ms`** |
| upstreams | 8 | **1** | 8 | **1** |
| split repeats | 8 of 9 | **0 of 9** | 7 of 9 | **0 of 9** |

The `draft_reply` `p99` is a single call of 76.9 seconds on the pinned host, with
`finish_reason: stop` and 718 tokens: a normal response that the gateway kept
back. Without it: `p50` `972ms`, `p90` `1,380ms`, highest `2,632ms`. That is the second
tail described in section 6, which this default does not address.

The judge for these runs was `gemini-3.5-flash`, not the `mistral-large` the
earlier records used. The preset binds `mistral-large` at `premium`, and the
runner refuses a judge that leads a task it would also grade. Latency, and which
upstream served a call, do not depend on the judge. You cannot compare scores
from the two judges.

## 4. Hard filters and soft preferences

A hard filter removes a host from the candidate set. A soft preference only
changes the order. Only a hard filter puts a limit on a tail.

| field | effect | hard? |
|---|---|---|
| `only` / `ignore` | allow list or block list by slug | **hard** |
| `quantizations` | serving number format | **hard** |
| `require_parameters` | hosts that support every parameter sent | **hard** |
| `max_price` † | `{prompt, completion}` `$/M`; *blocks the request* if no host is under the price | **hard** |
| `sort` | `throughput` \| `price` \| `latency`; **turns off load sharing** | changes the order |
| `preferred_max_latency_p90` | a `p90` limit over a moving window of 5 minutes | **soft** |
| `allow_fallbacks` | switch hosts on failure; default true | `n/a` |

† **A tier can set `max_price`.** It goes under `routing.provider` on an
OpenRouter binding: `OpenRouterRouting` has a member for it, and the config
schema has a field. It is left out of the default because section 5 measured
its `p99` at 387 seconds.

The soft preference on its own did harm. In the `A/B` test, the arm that set only
`preferred_max_latency_p90: 8` was the slowest arm we measured. Its `p90` was
`43.7s` against `38s` for the baseline, with one call that took 231 seconds. It did
worse than setting nothing.

## 5. What we tried and did not use

Two `A/B` rounds, 8 arms × 20 samples each, 320 samples in all. The arms took
turns and did not run one at a time. So a change in the broker's own load over
the night could not show up as the effect of one arm. Round 2, with `A_baseline`
and `G_nitro` repeated from round 1 as controls for change over time:

| arm | `p50` | `p90` | `p99` | `$` per 1,000 calls | upstreams |
|---|---|---|---|---|---|
| baseline *(control)* | 4,683 | 21,188 | 31,841 | 0.661 | 7 hosts |
| `:nitro` ending *(control)* | 1,437 | 5,306 | 7,174 | 1.272 | Cerebras ×20 |
| `only: [cerebras]` | 1,422 | 1,897 | 2,550 | 1.336 | Cerebras ×20 |
| **the shipped default** | 1,392 | **1,861** | **2,180** | 1.351 | Cerebras ×20 |
| default + `reasoning_effort: low` | **1,071** | **1,677** | **1,794** | **0.866** | Cerebras ×20 |
| default + `session_id` | 1,476 | 2,881 | 6,660 | 1.289 | Cerebras ×20 |
| `only: [cerebras, groq]` | 2,609 | 3,513 | 3,857 | 0.971 | Groq ×14, Cerebras ×6 |
| `sort` + `max_price` | 2,702 | **141,484** | **386,985** | 1.068 | Groq ×11, Cerebras ×9 |

**`max_price` is not used.** It is not in the default, but a tier may set it (see
the note in section 4). Its `p99` was 387 seconds, the slowest arm of either round: a
price limit cannot remove what the sort then prefers.

**`only` and `ignore` are not used.** They reach the same host as the sort, but
drop the many back-up hosts that a sort keeps.

**Use `sort: throughput`, not the `:nitro` ending.** `:nitro`, `only: [cerebras]`
and the default all reached Cerebras ×20, yet the `p90` of `:nitro` was about 3×
the other two. It also carries a right to a higher service tier. And the ending
form fails open on a typing error: `gpt-oss-120b:baseten` was skipped with no
error and routed to CoreWeave. The forms `@baseten` and `/baseten` at least
return a 400.

**`reasoning_effort: low` is an operator setting, not a default.** It was best at every
latency point (`p50`, `p90`, `p99`) and cost 36% less. It also cost 20 points of certification
score (`judge_score_p50` 85 → 65), with mean output down from 961 to 259 tokens.
Structure stayed at 1.0: the answers still parse, pass the validator and stay
under their caps. They are worse answers. On latency and cost only, we would have
used this arm; only the graded lane showed the 20-point drop in quality.

**`session_id` pins without making any call faster.** A fixed key is the OpenRouter key that keeps
routing on one host, and it works as documented: 20/20 samples on one host in
both rounds. But it pins to the host it reached *first*, which can be any host.

In round 1 that was
Parasail, an `fp4` host that was far from the fastest, and `p50` stayed at `8.9s`. It is a
tool for getting the same result twice. It could do that *without* making the
candidate set smaller. This tree does not send it yet.

## 6. A stopped gateway is a separate tail

2 of 56 samples (3.6%) went past a client time limit of 400 seconds: one on the
baseline, one on the fastest arm. Both returned bodies of only spaces and new
lines. The gateway fills the connection with new lines to hold it open, then
never answers.

So there are two tails that do not depend on each other. The choice of upstream
explains the times from 30 to 90 seconds, and the preferences above remove
them. A gateway stop does not depend on the arm, and only a client time limit plus a retry
puts a limit on it. `ai.CallCeiling` (300 seconds) and the 3 tries of the
certification lane carried the runs through.

OpenRouter says the same. It moves to a back-up provider on its own, but an
error in the gateway itself needs a retry on the client side.

## 7. Do not judge the cost from `est_cost_microusd`

Between the two `draft_reply` runs, our own number moved only from 264 to 268
`microUSD`, which reads as no cost. It is priced from `ai_model_rate`, which keys on
the model. The whole price difference here is per upstream (Cerebras `$0.35/M`
prompt against CoreWeave `$0.03/M`, about 11×).

OpenRouter returns the true number as `usage.cost` on every response, and this
tree does not read it. Until it does, use the cost column of the `A/B` test:
`$1.351` against `$0.661` per 1,000 calls, about 2×.

## 8. What the trace records

`ai_call` carries:

- **`served_provider`**: the upstream that served the call, from the `provider`
  field of the response. Empty on a direct vendor, which reports none.
- **`finish_reason`**, so an answer that ended early and a complete one are different
  rows.
- **`cached_tokens` and `cache_write_tokens`**, read from the
  `usage.prompt_tokens_details` of the response, both already within
  `prompt_tokens`. So an upstream that caches prices at its cache rates,
  not as plain input. (Anthropic through the broker is the one that asks a price
  for the cache write.)

`served_identity_source` stays `'echo'` for this wire. The `model` field of the
broker is a copy of our own request. So we know who served the call, but not
which model they served. Reporting that copy as the served model would be wrong.

The payload trace of the certification lane carries the same two per call, so we
can tie every run to a host.

## 9. What hosts report that needs care

- **Reasoning tokens can be more than completion tokens.** Some hosts report more
  reasoning tokens than completion tokens (DeepInfra 817 to 787, Parasail 1117 to
  1069, AkashML 1234 to 1121). The certification lane grades an answer as
  `TokensOut - ReasoningTokens`. With no limit that goes below zero. A count below
  zero never passes a `max_tokens` cap, so the cap would never act. The adapter
  caps the reported value at the completion count.
  - Other hosts are wrong the other way. BaseTen reported 0 reasoning tokens on a
    response whose reasoning text was there to see. So the field has a limit,
    and is never made up.
- **A cap reached while thinking returns `content: null`.** A reasoning
  model uses its output budget thinking before the answer starts, and both
  count against the same cap. When the cap is reached part way through thinking,
  every generated token is under `reasoning`. Reading only `content` returns
  empty text with no error, so no retry and no log. Read `reasoning` too.
  - We found this once in 225 calls, at the one call site in the tree that does
    not use `ai.ReasoningOutputMaxTokens`. The comment on that value describes
    this failure.

## 10. Measuring again later

The raw data, the test tools and the working notes were files of one session
under `.tmp/openrouter-stability/`, which is not committed. What you can run again
from here:

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

Then compare `served_provider` and `latency_ms` over the two traces, per task and
per model. Putting models or tasks together hides the effect: fast tiers of another model
pull the median far down.

Read the list of hosts again before each round:

```sh
curl -s https://openrouter.ai/api/v1/models/openai/gpt-oss-120b/endpoints \
  | jq '.data.endpoints[] | {provider_name, quantization, max_completion_tokens, uptime_last_30m}'
```

Check two things again, because both would change the default: whether Cerebras
is still the fastest host, and whether it still serves at `fp16`. The
`quantizations` clause admits it today. A host that moves to a smaller number
format would drop out of the candidate set with no error.

## 11. The decisions endpoint

OpenRouter also serves TypeSafe Jev at
`POST https://openrouter.ai/api/alpha/decisions`, priced on input tokens only. It
is one server on the Jev wire, so it binds the `decisions:` lane of the routing
config under `jev_compatible`. That provider host
(`providers.jev_compatible.base_url`) is the full endpoint; nothing is added to it.
**Preset: OpenRouter** fills it, and `openrouter_cloud.yaml` carries a block in
comments whose lane endpoint is moved to the provider:

```yaml
    decisions:
      provider: jev_compatible
      model: typesafe/jev-1.13
      base_url: https://openrouter.ai/api/alpha/decisions
```

`JEV_COMPATIBLE_API_KEY` carries the OpenRouter key (the
`OPENAI_COMPATIBLE_API_KEY` value). It takes no `routing:` preferences. A bound
lane answers only the sites certified for it; the rest go to the tier order of the task
([ai-runtime.md](../explanation/ai-runtime.md#the-decision-lane)).
