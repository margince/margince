<!-- prose:plain -->
# AI thinking levels

`thinking` is how hard a model thinks before it answers. A site in `backend/api/ai-tasks.yaml` may set a **floor**. Unless a stronger setting wins, the request goes up to at least
that level, and never below what the adapter would send without it. The request options, a level for the task and the
level on a binding all come before the site. The full order is under [Which level wins](#which-level-wins). Each provider maps the floor to its own field on the wire, or
sends nothing when it cannot know the field or cannot send it.

The adapter default is not always the model default. Margince already sends a structured Gemini
request at `low`, which is under the default of a Gemini 3 Flash or Pro model.

## Levels

| Level | Meaning |
|---|---|
| `minimal` | The least thinking a model that thinks can do. |
| `low` | A short think before it answers. |
| `medium` | A middle think. |
| `high` | A long think. |

A site may set only the four levels above. A site that does not need thinking leaves out `thinking`, so a site
cannot set `none`: a floor of nothing asks for nothing. Margince compares a floor with the efforts and levels a
model lists on one scale. That scale also holds values only a model may list: `none` < `minimal` < `low`
< `medium` < `high` < `xhigh` < `max`.

## Which level wins

Strongest first:

1. The options on the request itself: `ProviderOptions["gemini"].thinking_level`,
   `ProviderOptions["openai"].reasoning_effort`, `ProviderOptions["ollama"].think`.
2. A level an admin sets for the task, in the task sheet under AI tasks, stored in
   `ai.task_overrides` (`thinking`). Margince sends this level as set, even below a floor.
   It holds for every site of the task, and each provider gets it in its own form:
   - OpenRouter gets it as `reasoning.effort`.
   - Gemini gets it as `thinkingLevel` on a model that takes one.
   - OpenAI gets it as `reasoning.effort` on a model that thinks.
   - Anthropic gets the budget for that level.
   - Ollama gets the least listed value that meets it.

   A model with no thinking control does not use it, and the call records no thinking block.
   It also changes the cache key for results, so Margince does not serve an answer stored at
   another level.
3. The setting on the binding: `thinking_level` (Gemini) or `routing.reasoning.effort`
   (OpenRouter). The operator set it, so it wins both up and down, even below the floor.
4. The site floor: `thinking:` on the site in `ai-tasks.yaml`.
5. The adapter default.

## What each provider gets for `low`

| Provider / model | Default without a floor | Sent for `low` | Sends nothing when |
|---|---|---|---|
| Gemini 3 Flash-Lite | thinks at `minimal` | `generationConfig.thinkingConfig.thinkingLevel: "low"` | the binding sets `thinking_level` |
| `gemini-3.1-flash-lite-image` | thinks at `minimal`; takes only `minimal` and `high` | `thinkingLevel: "high"` | the binding sets `thinking_level` |
| Gemini 3 (Flash, Pro) | `medium` or more; a structured request already gets `low` | nothing new: the structured default `low` meets it | the default meets the floor |
| Gemini 2.5 and older | no `thinkingLevel` field | nothing | always: the field is a 400 there |
| OpenRouter `openai/gpt-oss-120b` | on (always), effort `medium` | nothing | default `medium` meets `low` |
| OpenRouter `mistralai/mistral-medium-3-5` | on, effort `high` | nothing | default `high` meets `low` |
| OpenRouter `mistralai/mistral-small-2603` | off; efforts `high`, `none` | `reasoning: {"effort": "high"}` | never |
| OpenRouter `google/gemma-4-*-it` | off; no efforts listed | nothing | on or off only: no effort limits its thinking |
| OpenRouter `anthropic/claude-sonnet-4.6` | on, effort `medium` | nothing | default `medium` meets `low` |
| OpenRouter `mistralai/ministral-*` | does not think | nothing | the model lists no `reasoning` |
| OpenRouter, a model on by default that names no effort | on, effort not named | `reasoning: {"effort": "low"}` | never |
| OpenRouter, every model | the catalog `GET /api/v1/models`, read once per binding; a failed read runs again after a minute | the rule above | the request has tools, the list cannot be read, or the binding sets `routing.reasoning.effort` |
| Anthropic 4.6–4.8 (`claude-sonnet-4-6`, `claude-opus-4-6`/`-7`/`-8`) | off | `thinking: {"type": "adaptive"}` | the request has tools |
| Anthropic 4.5 and older (`claude-haiku-4-5`, `claude-sonnet-4-5`, …) | off | `thinking: {"type": "enabled", "budget_tokens": 1024}` | tools, or `max_tokens` leaves under 1024 for the answer after the budget |
| Anthropic `5.x` (Opus 5, Sonnet 5, Fable, Mythos) | on, effort `high` | nothing | always meets the floor |
| OpenAI `gpt-5.1`, `gpt-5.2`, `gpt-5.4` | effort `none` | `reasoning: {"effort": "low"}` | never |
| OpenAI `gpt-5`, `gpt-5.5`, `gpt-5.6`, `gpt-6`, the `o` models | effort `medium` | nothing | default `medium` meets `low` |
| OpenAI `o1-mini`, `o1-preview` | no `reasoning.effort` field | nothing | always: the field is a 400 there |
| OpenAI models that do not think (`gpt-4.x`, `gpt-5-chat-*`, names it does not know) | no thinking | nothing | always: the field is a 400 there |
| Ollama, on/off model (Gemma 4, `Qwen3`) | adapter sends `think: false` | `think: false` | on or off only: no effort limits its thinking |
| Ollama, model with levels (`gpt-oss`) | adapter sends the lowest level | `think: "low"` | never |
| Ollama, model that does not think | nothing | nothing | `/api/show` lists no `thinking.values` |
| vLLM (`Qwen3`, `gpt-oss`) | the flags of the server decide | nothing | always: the binding cannot tell which model the server runs |
| `openai_compatible` on every other host | host default | nothing | always |

Notes:

- Margince never sends `output_config.effort` (Anthropic) or `exclude` (OpenRouter).
  Effort changes the whole answer, and its default is already `high`.
- Budget sizes on Anthropic 4.5 and older: `minimal` and `low` 1024 (the least the API
  takes), `medium` 4096, `high` 16384.
- Margince leaves off a model that can only turn thinking on or off. A floor names a limit
  on effort, and on/off thinking has no limit. On `gemma4:12b` through Ollama, one onboarding
  turn thought for about 4,300 tokens and 7 minutes, against 110 tokens and 15 seconds with
  thinking off.
- Thinking counts against the output limit on every provider. A floor on a small
  `max_tokens` can leave less room for the answer.

## How to set it

A site floor, in `backend/api/ai-tasks.yaml`:

```yaml
sites:
  - {name: company_message, kind: multi_turn, thinking: low}
```

The level on a binding, in a preset or routing file (this comes before every site):

```yaml
cheap_cloud: { provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low }
cheap_cloud:
  provider: openai_compatible          # host on providers.openai_compatible
  model: openai/gpt-oss-120b
  routing: { provider: { sort: throughput }, reasoning: { effort: low } }
```

## How to check

- **Certification record:** `site_thinking` names the floor of each site, on a rung whose adapter maps
  a floor to its wire. It is the floor asked for, not what Margince sent on the wire.
- **Call trace:** `reasoning_tokens` on the call shows how much the model really thought.
- **Certification page:** a record row reads `(this site: thinking low)` where a site ran
  under a floor ([ai-certification.md](ai-certification.md)).
