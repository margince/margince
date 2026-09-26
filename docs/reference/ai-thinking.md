# AI thinking levels

`thinking` is how hard a model reasons before it answers. A site in
`backend/api/ai-tasks.yaml` may set a level, and that level is a **floor**: the
model thinks at least this much, and never less than its own default. A binding
can also set its own level, and that outranks the site. Each provider maps the
floor to its own wire field, or sends nothing when it cannot know.

## Levels

| Level | Meaning |
|---|---|
| `minimal` | The least thinking a model that thinks can do. |
| `low` | A short think before answering. |
| `medium` | A moderate think. |
| `high` | A long think. |

A site that should not think omits `thinking`. There is no `none` floor: a floor
of nothing asks for nothing. Floors, efforts and levels are compared on one
scale: `none` < `minimal` < `low` < `medium` < `high` < `xhigh` < `max`.

## Precedence

Strongest first:

1. The request's own options: `ProviderOptions["gemini"].thinking_level`,
   `ProviderOptions["openai"].reasoning_effort`, `ProviderOptions["ollama"].think`.
2. The binding's explicit setting: `thinking_level` (Gemini) or
   `routing.reasoning_effort` (OpenRouter). The operator chose it, so it wins in
   both directions, even below the floor.
3. The site floor: `thinking:` on the site in `ai-tasks.yaml`.
4. The adapter's default.

## What each provider is sent for `low`

| Provider / model family | Default without a floor | Sent for `low` | Sent nothing when |
|---|---|---|---|
| Gemini 3 Flash-Lite | thinks at `minimal` | `generationConfig.thinkingConfig.thinkingLevel: "low"` | the binding sets `thinking_level` |
| Gemini 3 (Flash, Pro) | `medium` or deeper; a structured request already gets `low` | nothing new: the structured default `low` meets it | the default meets the floor |
| Gemini 2.5 and earlier | no `thinkingLevel` field | nothing | always: the field is a 400 there |
| OpenRouter `openai/gpt-oss-120b` | on (mandatory), effort `medium` | nothing | default `medium` meets `low` |
| OpenRouter `mistralai/mistral-medium-3-5` | on, effort `high` | nothing | default `high` meets `low` |
| OpenRouter `mistralai/mistral-small-2603` | off; efforts `high`, `none` | `reasoning: {"effort": "high"}` | — |
| OpenRouter `google/gemma-4-*-it` | off; no efforts listed | `reasoning: {"enabled": true}` | — |
| OpenRouter `anthropic/claude-sonnet-4.6` | on, effort `medium` | nothing | default `medium` meets `low` |
| OpenRouter `mistralai/ministral-*` | does not reason | nothing | the model lists no `reasoning` |
| OpenRouter, any model | read from `GET /api/v1/models` once per model | the rule above | the list is unreadable, or the binding sets `routing.reasoning_effort` |
| Anthropic 4.6–4.8 (`claude-sonnet-4-6`, `claude-opus-4-6`/`-7`/`-8`) | off | `thinking: {"type": "adaptive"}` | the request carries tools |
| Anthropic 4.5 and earlier (`claude-haiku-4-5`, `claude-sonnet-4-5`, …) | off | `thinking: {"type": "enabled", "budget_tokens": 1024}` | tools, or `max_tokens` ≤ the budget |
| Anthropic 5.x (Opus 5, Sonnet 5, Fable, Mythos) | on, effort `high` | nothing | always meets the floor |
| OpenAI `gpt-5.1`, `gpt-5.2`, `gpt-5.4` | effort `none` | `reasoning: {"effort": "low"}` | — |
| OpenAI `gpt-5`, `gpt-5.5`, `gpt-5.6`, `gpt-6`, o-series | effort `medium` | nothing | default `medium` meets `low` |
| OpenAI non-reasoning (`gpt-4.x`, `gpt-5-chat-*`, unknown ids) | no reasoning | nothing | always: the field is a 400 there |
| Ollama, boolean model (Gemma 4, Qwen3) | adapter sends `think: false` | `think: true` | — |
| Ollama, graded model (gpt-oss) | adapter sends the lowest level | `think: "low"` | — |
| Ollama, model that does not think | nothing | nothing | `/api/show` lists no `thinking.values` |
| vLLM (Qwen3, gpt-oss) | the server's own flags decide | nothing | always: the binding cannot tell which model it serves |
| `openai_compatible` on any other host | host default | nothing | always |

Notes:

- `output_config.effort` (Anthropic) and `exclude` (OpenRouter) are never sent.
  Effort shapes the whole answer, and its default is already `high`.
- Budget sizes on Anthropic 4.5 and earlier: `minimal` and `low` 1024 (the API
  minimum), `medium` 4096, `high` 16384.
- Thinking counts against the output ceiling on every provider. A floor on a
  small `max_tokens` can leave less room for the answer.

## How to set it

A site floor, in `backend/api/ai-tasks.yaml`:

```yaml
sites:
  - {name: company_message, kind: multi_turn, thinking: low}
```

A binding's own level, in a preset or routing file (this outranks every site):

```yaml
cheap_cloud: { provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low }
cheap_cloud:
  provider: openai_compatible
  model: openai/gpt-oss-120b
  base_url: https://openrouter.ai/api
  routing: { sort: throughput, reasoning_effort: low }
```

## How to check

- **Certification record:** `site_thinking` names each site's floor on a rung
  that maps one. It is the floor asked, not the wire sent.
- **Call trace:** `reasoning_tokens` on the call shows how much the model
  actually thought.
- **Certification page:** a record row reads `(this site: thinking low)` where
  a site ran under a floor ([ai-certification.md](ai-certification.md)).
