# Enrich a company with a local LLM (Ollama)

Run the AI lanes (company **enrich**, cold-start read-back) against a local or
self-hosted [Ollama](https://ollama.com) instead of a cloud model, with no
Anthropic key. Retrieval is the app's job (it fetches the page under an SSRF
guard, then asks the model only to *extract* grounded facts), and the `enrich`
task routes to the `local_small` tier first, so a local model serves it. See
[explanation/agent-surface.md](../explanation/agent-surface.md) for the model
runtime and [reference/configuration.md](../reference/configuration.md) for the
flags/env below.

## 1. Run Ollama and pull the models

```sh
ollama serve                 # default http://localhost:11434
ollama pull gemma3           # a small local model to bind local_small to
ollama pull bge-m3           # only if you exercise search/retrieval (embeddings lane)
```

`mistral` follows the extraction JSON schema more reliably than `gemma3`; pull
it too (`ollama pull mistral`) if enrich grounding is weak.

For Gemma 4, pull `gemma4:12b` (what we measured, and what not to pull instead:
[ollama-self-hosting.md](../reference/ollama-self-hosting.md)) and bind it with
[`config/presets/gemma4_local_ollama.yaml`](../../config/presets/gemma4_local_ollama.yaml).
Gemma 4 reasons before it answers, which Ollama turns on by default. The adapter
asks Ollama once per model what it accepts (`/api/show`) and sends `think: false`
to a model that can turn thinking off (the lowest level to one that cannot, and
nothing to one that does not think). Left on, a short JSON answer takes tens of
seconds and a small output budget is spent on the reasoning before the answer
starts, so the reply comes back empty with `done_reason: "length"`. A proxy in
front of Ollama that refuses `/api/show` gets no `think` field, so Gemma 4 keeps
its default thinking there: give the adapter direct access to `/api/show`, or
expect slow, sometimes empty answers.

## 2. Point the AI lanes at Ollama

A dev stack binds every tier and the embed lane to `gemini` (`seeds.ai_routing`
in `config/margince.dev.yaml`). **Enrich itself needs one rebind**: point
`local_small`, the first rung of `enrich`'s ladder (`local_small` →
`cheap_cloud`), at Ollama. Step 3 says what else must move for a stack with no
cloud key.

On a running stack do it under **Settings → AI**, which takes effect immediately.
To have a *fresh* stack come up this way, edit the seed and `make dev-fresh`; the
shape is the same either way:

```yaml
local_small: { provider: ollama, model: gemma3 }   # no base_url ⇒ localhost:11434
```

Edit the other tiers to:

- **use a remote/self-hosted Ollama**: set the provider's `base_url` once (no
  trailing slash; the adapter appends `/api/chat`), and every Ollama lane uses it:
  ```yaml
  providers:
    ollama: { base_url: https://ollama.internal:11434 }
  tiers:
    local_small: { provider: ollama, model: mistral }
  ```
- **run cold-start / offer-draft locally too** (they ladder `cheap_cloud` →
  `premium`, cloud by default): rebind those tiers to `ollama` as well.

> Seeds apply once, at bootstrap; see
> [connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md#2-bind-a-tier).

## 3. Start the stack

`scripts/dev.sh` (`make dev`) scans the `seeds.ai_routing` that will bind this
stack and drops to the offline fake unless **every bound cloud provider's key is
set**. The keys are `anthropic` → `ANTHROPIC_API_KEY`, `openai` → `OPENAI_API_KEY`,
`gemini` → `GEMINI_API_KEY`, `gemini_vertex` → `GEMINI_VERTEX_SA_JSON`,
`openai_compatible` → `OPENAI_COMPATIBLE_API_KEY` and `jev` → `TYPESAFE_API_KEY`.
Local providers (`ollama`/`vllm`/`fake`) need no key.

The scan counts every binding, not only the tiers you exercise: an unused tier
and the `embeddings` lane count too. If any key is missing, the stack runs on
the offline fake, and that fakes the *Ollama* call too, because the fake stands
in for the whole binding instead of for the unkeyed tier. The fake is a
fallback: once the binding is servable it outranks the flag, so a stack that can
reach its models never answers with canned text.

So out of the box you must either set `GEMINI_API_KEY`, or rebind every cloud
binding (all four tiers and `embeddings`) to `ollama` (step 2) for a fully
local, no-key stack:

```sh
make dev   # look for: "dev: the stored model binding serves the cold-start read-back (providers bound by …)"
```

> Persist keys in `.env.local` if you prefer; it is git-ignored and `make dev`
> reads it.
>
> ⚠️ enrich escalates to `cheap_cloud` on a provider error or schema failure, and
> the cold-start read-back starts on `cheap_cloud`. For a local-only run, bind
> every tier and the embed lane to Ollama.

`make dev` brings up the app on `:8080` (the api behind it), cold: the bootstrap
company and admin, no records. Open
**http://localhost:8080** and log in as `admin@demo.test` /
`demo-password-123`.
Full first-run details:
[tutorials/getting-started.md](../tutorials/getting-started.md).

## 4. Add a company and enrich it

1. Go to **Companies** (`#/companies`) → **New company**. Give it a **crawlable**
   domain, e.g. `stripe.com`.
   > The fetcher sends `User-Agent: margince-siteread/1.0`; bot-protected sites
   > (e.g. `tesla.com`) answer **403**. Known-crawlable: `stripe.com`, `go.dev`,
   > `ollama.com`, `news.ycombinator.com`, `sqlite.org`.
2. Open the company → **Read now** on the *Read from the website* card.
3. **Expected:** a staged 🟡 enrichment proposal: a confirm-first banner with
   per-field confidence and evidence chips, and an **Open inbox** button.
   Nothing writes to the company until you accept it in the Inbox.

The model is constrained to emit the extraction JSON shape at generation
(Ollama's `format`), so a small model returns a well-formed object instead of
failing the parser. Grounding still depends on the model: the evidence gate drops
any field whose snippet is not a verbatim quote from the page. That refusal is
the guard against invented facts.

## Troubleshooting

| Symptom | Meaning / fix |
|---|---|
| Log says *"binds provider(s) whose key is not set … offline fake"* | A tier is still bound to a cloud provider whose key is missing. Rebind that tier to `ollama` under **Settings → AI**; that takes effect on the running stack, within the routing refresh interval, with no restart. Setting the named key in `.env.local` instead needs `make dev` again, because a process reads the environment only at boot. |
| *"Couldn't read enough from this company's site."* | The fetch failed: the offline fake is active (see above), a **403** from a bot-protected domain, or a thin page. Use a crawlable domain. |
| *"no field survived the no-guess evidence gate"* | The model returned JSON but no `evidence_snippet` was verbatim on the page (or confidences ≤ 0). Expected for weak models / thin pages; try a content-rich page, or `mistral` over `gemma3`. |
| A 500 mentioning *"cannot unmarshal … into … string"* | The model ignored the schema and emitted a wrong-typed field. Switch to `mistral`. |
| Logged out immediately after login | The api isn't reachable at the `/v1` proxy target. Make sure `make dev` is running (it starts both) and use the URL it printed. |

Set `MARGINCE_LOG_LEVEL=debug` (in `.env.local` or via `--log-level`) for verbose
model-runtime logs. Small local models are hit-or-miss against the strict
evidence gate. A cloud model (a real provider key, tiers on `gemini` /
`anthropic` / `openai`) grounds more reliably; Ollama suits exercising the
pipeline end to end.
