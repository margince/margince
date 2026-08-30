# Connect a cloud model provider (BYOK)

Point the AI lanes at a **customer-supplied cloud key** — Anthropic, OpenAI,
Gemini, or any OpenAI-compatible vendor. Margince runs no inference of its own:
the key, the endpoint, and the DPA are yours. A provider is part of the stored
**binding**, never a binary flag — swapping one is a settings change, not a
deploy, and not even a restart. See [explanation/agent-surface.md](../explanation/agent-surface.md) for
the model runtime and [reference/configuration.md](../reference/configuration.md)
for the full provider matrix. For the no-cloud path, see
[enrich-with-a-local-llm.md](enrich-with-a-local-llm.md).

## 1. Pick a provider

| `provider` | Use it for | Key env var | `base_url` |
|---|---|---|---|
| `anthropic` | Claude (native Messages API — image input) | `ANTHROPIC_API_KEY` | optional (default `api.anthropic.com`) |
| `openai` | GPT (native Responses API — reasoning effort, prompt-cache + reasoning token usage, image/PDF input) | `OPENAI_API_KEY` | optional (default `api.openai.com`) |
| `gemini` | Gemini (native `generateContent` — thinking level, thought-signature continuity, image/PDF input) | `GEMINI_API_KEY` | optional (default `…/v1beta`) |
| `openai_compatible` | the OpenAI-wire long tail — Mistral, DeepSeek, Groq, Together, OpenRouter, a self-hosted gateway, … | `OPENAI_COMPATIBLE_API_KEY` | **required** |

A binding names only the provider — **the BYOK key lives in the key vault**, put
there under Settings → AI → Model provider keys (step 4). A stray `api_key:` in a
binding is a startup error, and secrets never touch a config file.

The variable above is a **seed**, not the home: a boot that finds one may seal it
into the vault, after which it can be unset. It stays the runtime source in two
cases — an installation with no vault configured, and the DB-less debug and
certification lanes, which open no vault at all.

Reach for a **native** adapter (`openai`/`gemini`) when you want that vendor's
reasoning/thinking knobs, itemized usage, or a PDF handed to the model whole —
`anthropic` carries images but not PDFs, and `openai_compatible` carries only
what its binding declares. Reach for
`openai_compatible` for any vendor that speaks `/v1/chat/completions` and isn't
worth a dedicated adapter — it is the correct default for everything that is not
Anthropic, OpenAI, or Gemini.

## 2. Bind a tier

The binding lives in the database. On a **running, already-bound** installation,
bind a tier under **Settings → AI** — that is the whole step, and it takes effect
without a restart, within the routing refresh interval.

Saving the FIRST binding is the one case that needs a restart afterwards: a role
that started with nothing bound wired no model path, and the watcher that would
pick up the change is built from that path. For a fresh installation, declaring it
under `seeds.ai_routing` in `margince.yaml` (a dev stack's is in
`config/margince.dev.yaml`) avoids that entirely — first boot comes up bound.

Either way it is the same shape, and **no key ever appears in it** — the key goes
in separately, in step 4. The shipped dev default binds **gemini** on
`cheap_cloud` + `premium`:

```yaml
# Native adapters — the key comes from the vault (Settings -> AI); GEMINI_API_KEY
# / OPENAI_API_KEY seed it, and remain the source with no vault and in the
# DB-less lanes:
tiers:
  cheap_cloud: { provider: gemini, model: gemini-2.5-flash }
  premium:     { provider: gemini, model: gemini-2.5-pro }

# …or any OpenAI-compatible vendor via the generic adapter. It needs a base_url
# (the key comes from OPENAI_COMPATIBLE_API_KEY):
tiers:
  cheap_cloud:
    provider: openai_compatible
    model: mistral-small-2506        # pin an explicit version — -latest aliases drift
    base_url: https://api.mistral.ai # host root, NO /v1 (see the caveat below)
```

> **`base_url` for the OpenAI-wire providers (`openai_compatible`, `openai`,
> `vllm`) is the vendor host root with _no_ version segment.** The adapter
> appends `/v1/chat/completions` (or `/v1/responses`), so a base ending in `/v1`
> doubles it — `https://api.mistral.ai/v1` becomes `…/v1/v1/chat/completions` and
> 404s. Use `https://api.mistral.ai`. `gemini` is the mirror: its default base
> keeps `/v1beta` and the paths are version-relative, so leave `base_url` unset.
>
> A seed is consumed **once**, at bootstrap, so editing it after a database
> exists changes nothing — `make dev-fresh` re-runs the bootstrap, and Settings →
> AI rebinds a stack that is already up. The shape a binding has — `profile` plus
> a `tiers` map — is schema-validated in any editor with a YAML language server
> (autocomplete, enum checks, hover docs) against `config/margince.schema.json`, which the shipped configs point at with a `# yaml-language-server:` line.
>
> **One key, every open-weight model:** bind `openai_compatible` with
> `base_url: https://openrouter.ai/api` and one `OPENAI_COMPATIBLE_API_KEY`, and
> a single OpenRouter key reaches every open-weight model. Filter candidates to
> models declaring both `structured_outputs` and `tools`. To certify one rather
> than bind it, `make e2e-ai` takes the model outright:
> `MODEL=openai_compatible:<slug> BASE_URL=https://openrouter.ai/api`.

## 3. Bind the embeddings lane separately

The embedding lane is bound apart from the chat tiers so retrieval survives a
chat-budget exhaustion. Bound apart does not mean bound elsewhere: the dev seed
points it at the same broker the chat tiers use, so a stack needs ONE key rather
than a second provider's purely for embeddings.

```yaml
# the dev default — same broker and key as the chat tiers
embeddings: { provider: openai_compatible, model: mistralai/mistral-embed-2312,
              base_url: https://openrouter.ai/api, dimensions: 1024 }
# embeddings: { provider: gemini, model: gemini-embedding-001 }  # native adapter, key from GEMINI_API_KEY
# embeddings: { provider: ollama, model: bge-m3 }                # fully-local alternative
# embeddings: { provider: fake }                                 # offline dev
```

> The retrieval store's column is an unbounded **`vector`**, and `dimensions:`
> on the embeddings binding declares the width it is populated under (default
> 1536, ceiling 2000 — pgvector's own index limit). The **native** adapters pin
> that width on the wire — `gemini` via `outputDimensionality`, `openai` via
> `dimensions` — so a cloud embedder drops in at whatever width you ask for.
> **`openai_compatible` does not**: it deliberately never sends `dimensions`,
> because a non-MRL model behind vLLM 400s on it. On that provider the
> configured width must EQUAL the model's native width. A binding that returns
> another width fails loudly.
>
> **Not every `openai_compatible` vendor serves the embeddings lane.**
> OpenRouter does — `/v1/embeddings`, with the catalog at
> `GET /api/v1/embeddings/models` — while a chat-only vendor 404s. Bind
> `embeddings:` to a vendor that has the lane (`gemini`, `openai`, Mistral,
> OpenRouter) or a local model (`ollama` `bge-m3`).

## 4. Start the stack

The key's home is the **key vault**: put it in under **Settings → AI → Model
provider keys**, one field per bound provider. It is encrypted at rest, never
read back to the screen, and rotatable without a restart.

For local dev the shortcut is still the environment. Set the key for your bound
provider in `.env.local` — `GEMINI_API_KEY`, `OPENAI_API_KEY`,
`ANTHROPIC_API_KEY`, or `OPENAI_COMPATIBLE_API_KEY`. `make dev` sources
`.env.local`, and the first boot that finds one **seals it into the vault** and
records where, so the variable can come back out afterwards. Either way the
binding stays keyless:

```sh
# .env.local:  GEMINI_API_KEY=…
make dev
```

To run without `make dev`, export the var yourself (production does the same via
its process manager):

```sh
cd backend && GEMINI_API_KEY=… go run ./cmd/api   # the stored binding applies; no routing flag
```

The api comes up on `:8080`. Exercise a lane that ladders to your tier — e.g.
open a company and **Read now** (cold-start read-back runs `cheap_cloud` →
`premium`). Set `MARGINCE_LOG_LEVEL=debug` for verbose model-runtime logs.

## The sovereign profile refuses every cloud provider

Under `profile: sovereign` (zero egress by construction) a cloud provider on any
tier — or the embeddings lane — is a **startup error**, not a runtime surprise.
The refusal is bound to the provider _name_, not a config flag, so pointing
`openai_compatible` at a localhost URL is still refused: only `ollama`, `vllm`,
and `fake` are sovereign-eligible. Use `eu_hosted` or `cloud_frontier` for a BYOK
cloud binding.

The endpoint is checked too, because a local provider name is not on its own a
local endpoint: `ollama` and `vllm` take a `base_url`, and one pointed at a
third-party host would send every call of a zero-egress deployment over the
public internet. Under this profile each binding's resolved `base_url` must be
loopback, link-local, or a private range — your own GPU box on your own network
counts; a DNS name does not, since what it resolves to can change after boot.

## Troubleshooting

| Symptom | Meaning / fix |
|---|---|
| `http 404` on `…/v1/v1/chat/completions` or `…/v1/v1/responses` | `base_url` includes a `/v1` segment — drop it (§2 caveat); the adapter adds it. |
| Boot error *"profile sovereign forbids cloud provider …"* | A cloud provider is bound under `profile: sovereign`. Switch to `eu_hosted`/`cloud_frontier`, or bind that tier to `ollama`/`vllm`. |
| Boot error *"needs an api key — set X_API_KEY …"* | The bound cloud provider's key env var is unset. Export the one the error names (e.g. `GEMINI_API_KEY`). |
| Boot error *"field api_key not found"* | You put an `api_key:` in the `seeds.ai_routing` binding — remove it; the key comes from the env var (see the table above). |
| Boot error *"needs a base_url …"* | `openai_compatible` has no `base_url`. Add the vendor host root (no `/v1`). |
| `http 404` on `/embeddings` | That `openai_compatible` vendor is chat-only. Rebind `embeddings:` to a lane-serving vendor or a local `bge-m3` (§3). |
| Embed error *"returned N vectors of width W, need 1×D"* | On `openai_compatible` the adapter never sends `dimensions`, so `dimensions:` must equal the model's NATIVE width (§3). Set it to `W`. |
| Model 404 / *"model not found"* | A drifting `-latest` alias or a wrong id. Pin an explicit versioned model, or resolve it from the vendor's `/models` endpoint. |
| Log says *"offline fake"* despite a cloud binding | Two causes, and the log line distinguishes them. Either nothing is bound — bind a tier under Settings → AI, or `make dev-fresh` to consume a `seeds.ai_routing` you just declared — or a binding EXISTS and could not be built, which with `--ai-fake` on the command line falls back to the fake and warns "the stored model binding cannot be served". That second one is almost always a bound vendor whose key is missing: supply it under Settings → AI → Model provider keys. |
