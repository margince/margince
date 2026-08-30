# Enrich a company with a local LLM (Ollama)

Run the AI lanes (company **enrich**, cold-start read-back) against a local or
self-hosted [Ollama](https://ollama.com) instead of a cloud model — no
Anthropic key needed. Retrieval is the app's job (it fetches the page under an
SSRF guard, then asks the model only to *extract* grounded facts), and the
`enrich` task routes to the `local_small` tier first, so a local model serves
it. See [explanation/agent-surface.md](../explanation/agent-surface.md) for the
model runtime and [reference/configuration.md](../reference/configuration.md)
for the flags/env below.

## 1. Run Ollama and pull the models

```sh
ollama serve                 # default http://localhost:11434
ollama pull gemma3           # a small local model to bind local_small to
ollama pull bge-m3           # only if you exercise search/retrieval (embeddings lane)
```

`mistral` follows the extraction JSON schema more reliably than `gemma3`; pull
it too (`ollama pull mistral`) if enrich grounding is weak.

## 2. Point the AI lanes at Ollama

A dev stack is bound to one broker over `openai_compatible` on every tier and on
the embed lane (`seeds.ai_routing` in `config/margince.dev.yaml`), so **enrich
needs one rebind**: point `local_small`
— the first rung of `enrich`'s ladder (`local_small` → `cheap_cloud`) — at Ollama.

On a running stack do it under **Settings → AI**, which takes effect immediately.
To have a *fresh* stack come up this way, edit the seed and `make dev-fresh`; the
shape is the same either way:

```yaml
local_small: { provider: ollama, model: gemma3 }   # no base_url ⇒ localhost:11434
```

Edit the other tiers to:

- **use a remote/self-hosted Ollama** — add a `base_url` (no trailing slash; the
  adapter appends `/api/chat`):
  ```yaml
  local_small: { provider: ollama, model: mistral, base_url: https://ollama.internal:11434 }
  ```
- **run cold-start / offer-draft locally too** (they ladder `cheap_cloud` →
  `premium`, cloud by default) — rebind those tiers to `ollama` as well.

> A seed is consumed once, at bootstrap, so editing it under a database that
> already exists changes nothing until `make dev-fresh`. Settings → AI is the
> path that works on a stack already running — and the one a real operator has.

## 3. Start the stack

`scripts/dev.sh` (`make dev`) scans the `seeds.ai_routing` that will bind this
stack and drops to the offline fake unless **every bound cloud provider's key is
set** (`anthropic` → `ANTHROPIC_API_KEY`, `openai` → `OPENAI_API_KEY`, `gemini` →
`GEMINI_API_KEY`, `openai_compatible` → `OPENAI_COMPATIBLE_API_KEY`); local
providers (`ollama`/`vllm`/`fake`) need no key.

If any key is missing the stack runs on the offline fake — and that fakes the
*Ollama* call too, because the fake stands in for the whole binding rather than
for the unkeyed tier. So the fix is to rebind the tiers you exercise to a local
provider, not to leave one cloud tier unkeyed. Note the fake is a fallback: once
the binding is servable it outranks the flag, so a stack that CAN reach its
models never answers with canned text.

The shipped dev routing binds every tier AND the embed lane to one broker over
the `openai_compatible` adapter, so out of the box you must either set
`OPENAI_COMPATIBLE_API_KEY`, or rebind the tiers you exercise to `ollama` (§2)
for a fully local, no-key stack:

```sh
make dev   # look for: "dev: the stored model binding serves the cold-start read-back (providers bound by …)"
```

> Persist keys in `.env.local` if you prefer — it is git-ignored and `make dev`
> reads it.
>
> ⚠️ Ladders can leave the box: enrich *starts* on `local_small` (Ollama once
> §2 rebinds it) but its ladder is `local_small` → `cheap_cloud`, so a provider error or
> schema failure escalates to the cloud tier — and flows that start
> cloud-bound (the cold-start read-back runs `cheap_cloud` → `premium`)
> call it immediately. For a guaranteed-local run, rebind every tier you
> exercise to Ollama (§2). And remember the all-or-nothing gate above: a
> missing key for *any* bound cloud provider puts the whole stack on the
> offline fake — Ollama included.

`make dev` brings up the app on `:8080` (the api behind it), cold —
the bootstrap organization and admin, no records. Open
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
3. **Expected:** a staged 🟡 enrichment proposal — a confirm-first banner with
   per-field confidence and evidence chips, and an **Open inbox** button.
   Nothing writes to the company until you accept it in the Inbox.

The model is constrained to emit the extraction JSON shape at generation
(Ollama's `format`), so a small model returns a well-formed object rather than
failing the parser. Grounding is still model-dependent: the evidence gate drops
any field whose snippet isn't a verbatim quote from the page — that refusal is
the anti-hallucination guarantee, not a bug.

## Troubleshooting

| Symptom | Meaning / fix |
|---|---|
| Log says *"binds provider(s) whose key is not set … offline fake"* | A tier is still bound to a cloud provider whose key is missing. Rebind that tier to `ollama` under **Settings → AI** — that takes effect on the running stack, within the routing refresh interval, with no restart. Setting the named key in `.env.local` instead does need `make dev` again, because a process reads the environment only at boot. |
| *"Couldn't read enough from this company's site."* | The fetch failed: the offline fake is active (see above), a **403** from a bot-protected domain, or a genuinely thin page. Use a crawlable domain. |
| *"no field survived the no-guess evidence gate"* | The model returned JSON but no `evidence_snippet` was verbatim on the page (or confidences ≤ 0). Expected for weak models / thin pages — try a content-rich page, or `mistral` over `gemma3`. |
| A 500 mentioning *"cannot unmarshal … into … string"* | The model ignored the schema and emitted a wrong-typed field. Switch to `mistral`. |
| Logged out immediately after login | The api isn't reachable at the `/v1` proxy target — make sure `make dev` is running (it starts both) and use the URL it printed. |

Set `MARGINCE_LOG_LEVEL=debug` (in `.env.local` or via `--log-level`) for verbose
model-runtime logs. Small local models are hit-or-miss against the strict
evidence gate — a cloud model (a real provider key, tiers on `gemini` /
`anthropic` / `openai`) grounds more reliably; Ollama is ideal for exercising
the pipeline end to end.
