# OpenRouter routing fields

Every key a `routing:` value may carry, and whether it belongs to a tier or to
the connection. Why the shipped default is what it is, and the measurements
behind it: [openrouter.md](openrouter.md).

The routing value is OpenRouter's own request shape: a `provider` object (which
hosts, and how) and a `reasoning` object (how hard the model thinks). It is held
by one schema, `$defs/upstreamRouting` in the configuration schema, which
`backend/gates/airoutingschema_test.go` holds to the parser and
`GET /ai/routing/schema` serves to the admin screen, each field with its
description, its OpenRouter link and its placement.

| Key | Placement | What it does |
|---|---|---|
| `provider.order` | tier | Host slugs to try first, in this order. |
| `provider.sort` | tier | `price`, `throughput` or `latency`, or `{by, partition}`; `partition: none` sorts across a fallback list's models at once. |
| `provider.quantizations` | tier | Serving precisions a host may use. A hard filter. |
| `provider.require_parameters` | tier | Only hosts that support every parameter the request carries. |
| `provider.max_price` | tier | `{prompt, completion, request, image}`: the most a request may cost. Hosts above it are filtered out. |
| `provider.preferred_min_throughput` | tier | Tokens per second, one number or `{p50, p75, p90, p99}`. Soft. |
| `provider.preferred_max_latency` | tier | Seconds, one number or `{p50, p75, p90, p99}`. Soft. |
| `reasoning.effort` / `reasoning.max_tokens` | tier | The thinking budget, as a level or in tokens; one or the other. |
| `reasoning.exclude` / `reasoning.enabled` | tier | Leave the reasoning out of the answer; turn thinking on or off. |
| `provider.only` / `provider.ignore` | connection | Allowlist and blocklist of host slugs. Hard filters, and the residency pin. |
| `provider.allow_fallbacks` | connection | Whether the broker may switch hosts when the chosen one fails. |
| `provider.zdr` | connection | Zero data retention: only hosts that keep no copy of the prompt or answer. |
| `provider.data_collection` | connection | `deny` keeps every request off hosts that may store or train on prompts. |
| `provider.enforce_distillable_text` | connection | Only models whose license allows their output to train other models. |

**A connection key on a tier is refused** by `PUT /ai/routing`, by its path
(`tiers.cheap_cloud.routing.provider.zdr`) with `moved_to_provider`, unless it
repeats the connection's own value, which is what a client writing back a
resolved binding sends. That is what makes loosening a tier's privacy
impossible: there is no tier value to loosen. A YAML seed or a stored row that
still carries a tier pin has it lifted onto the connection when it is read.

**Every refusal names its path.** An unknown key at any depth, a value outside a
vocabulary, a negative price or `effort` beside `max_tokens` is refused as a
field fault per path, and `POST /ai/routing/preview` lists every one of them
beside the merged request each tier will send (`effective.tiers`), so the editor
shows the problems and the request together.

**The flat spelling is still read.** `only`, `ignore`, `quantizations`, `sort`,
`require_parameters`, `allow_fallbacks`, `preferred_max_latency_p90` and
`reasoning_effort` at the top level of `routing:` parse at every door (a YAML
seed, the stored value, `MARGINCE_AICERT_UPSTREAM`), and a value the flat
spelling can say is stored in it. The binding digest every cached brief is keyed
on is computed over the stored spelling, so an unchanged binding keeps its
digest and its wire bytes; a value only the nested spelling can say is stored
nested. Mixing the two in one value is refused.
