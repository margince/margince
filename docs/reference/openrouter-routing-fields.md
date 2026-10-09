<!-- prose:plain -->
# OpenRouter routing fields

Every key a `routing:` value may carry, and whether it belongs to a tier or to
the connection. Why the default we ship is what it is, and the measures
behind it: [openrouter.md](openrouter.md).

The routing value is the request shape of OpenRouter itself. It has a
`provider` object (which hosts, and how) and a `reasoning` object (how hard
the model thinks). One schema holds it, `$defs/upstreamRouting` in the config schema.
`backend/gates/airoutingschema_test.go` holds it to the parser, and
`GET /ai/routing/schema` serves it to the admin screen. Each field comes with
what it does, its OpenRouter link and its placement.

| Key | Placement | What it does |
|---|---|---|
| `provider.order` | tier | Host slugs to try first, in this order. |
| `provider.sort` | tier | `price`, `throughput` or `latency`, or `{by, partition}`; `partition: none` sorts over all the models of a back-up list at once. |
| `provider.quantizations` | tier | The number formats a host may serve the model in. A hard filter. |
| `provider.require_parameters` | tier | Only hosts that support every parameter the request carries. |
| `provider.max_price` | tier | `{prompt, completion, request, image}`: the most a request may cost. Hosts above it are filtered out. |
| `provider.preferred_min_throughput` | tier | Tokens per second, one number or `{p50, p75, p90, p99}`. Soft. |
| `provider.preferred_max_latency` | tier | Seconds, one number or `{p50, p75, p90, p99}`. Soft. |
| `reasoning.effort` / `reasoning.max_tokens` | tier | The thinking budget, as a level or in tokens; one or the other. |
| `reasoning.exclude` / `reasoning.enabled` | tier | Leave the reasoning out of the answer; turn thinking on or off. |
| `provider.only` / `provider.ignore` | connection | Allow list and block list of host slugs. Hard filters, and the pin that keeps data in one region. |
| `provider.allow_fallbacks` | connection | Whether the broker may switch hosts when the first host it tries fails. |
| `provider.zdr` | connection | Zero data retention: only hosts that keep no copy of the prompt or answer. |
| `provider.data_collection` | connection | `deny` keeps every request off hosts that may store prompts or train on them. |
| `provider.enforce_distillable_text` | connection | Only models whose license allows their output to train other models. |

## Placement

`PUT /ai/routing` refuses a connection key on a tier by its path
(`tiers.cheap_cloud.routing.provider.zdr`) with `moved_to_provider`. The one
exception is a value that repeats the connection's own, which is what a client
sends when it writes back a binding it read. So a tier cannot make the privacy
of the connection less strict: there is no tier value to change. A YAML seed or a
stored row that still carries a tier pin has the pin moved to the connection
when it is read.

## Refusals

Every refusal names its path. Each of these is refused as a field error per path:

- an unknown key at any level;
- a value outside the allowed set;
- a price below zero;
- `effort` next to `max_tokens`.

`POST /ai/routing/preview` lists every one of them next
to the merged request each tier will send (`effective.tiers`). So the admin screen
shows the problems and the request together.

## Flat spelling

The flat spelling is still read. These keys parse at the top level of
`routing:` at every way in: `only`, `ignore`, `quantizations`, `sort`,
`require_parameters`, `allow_fallbacks`, `preferred_max_latency_p90` and
`reasoning_effort`. The ways in are a YAML seed, the stored value and
`MARGINCE_AICERT_UPSTREAM`.

A value the flat spelling can say is stored in it. The binding hash that every cached brief is keyed on
comes from the stored spelling. So a binding that has not changed keeps its
hash and its bytes on the wire. A value only the nested spelling can say is
stored nested.

A value that uses both forms at once is refused.
