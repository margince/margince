<!-- prose:plain -->
# Enrich a company with a local LLM (Ollama or vLLM)

Run the AI lanes (company **enrich**, and the read for a new company) against an
[Ollama](https://ollama.com) on your machine, or on a server you host. You use it in place of a cloud
model, with no Anthropic key. To get the data is the job of the app. It fetches the page behind an SSRF guard, then
asks the model only to *pull out* facts that the page backs up. The `enrich` task goes to the
`local_small` tier first, so a local model serves it. To serve the model with vLLM, follow
[Serve the model with vLLM instead](#serve-the-model-with-vllm-instead) in place of step 1.

See [explanation/agent-surface.md](../explanation/agent-surface.md) for the model runtime, and
[reference/configuration.md](../reference/configuration.md) for the flags and environment values below.

## 1. Run Ollama and pull the models

```sh
ollama serve                 # default http://localhost:11434
ollama pull gemma3           # a small local model to bind local_small to
ollama pull bge-m3           # only if you exercise search/retrieval (embeddings lane)
```

`mistral` follows the JSON schema for the facts more often than `gemma3` does. Pull it too
(`ollama pull mistral`) if `enrich` finds too few facts on the page.

For Gemma 4, pull `gemma4:12b`, and bind it with
[`config/presets/gemma4_local_ollama.yaml`](../../config/presets/gemma4_local_ollama.yaml). What we
measured, and what not to pull in its place, is in
[ollama-self-hosting.md](../explanation/ollama-self-hosting.md).

Gemma 4 reasons before it answers, and Ollama turns that on by default. The client asks Ollama once per
model what it accepts (`/api/show`). It sends `think: false` to a model that can turn thinking off. It
sends the smallest level to one that cannot, and nothing to one that does not think.

When thinking stays on, a short JSON answer takes many seconds. A small output limit goes to the
reasoning before the answer starts, so the reply comes back empty with `done_reason: "length"`.

A proxy between the client and Ollama that refuses `/api/show` gets no `think` field. So Gemma 4 keeps its default
thinking there. Give the client direct access to `/api/show`, or expect slow answers, and at times empty
ones.

## 2. Point the AI lanes at Ollama

A dev stack binds every tier and the embeddings lane to `gemini` (`seeds.ai_routing` in
`config/margince.dev.yaml`). **Enrich itself needs one change to its binding**: point `local_small` at
Ollama. That is the first rung of the ladder of `enrich` (`local_small` → `cheap_cloud`). Step 3 says what
else must move for a stack with no cloud key.

On a running stack, do it under **Settings → AI**, which every process picks up within 30 seconds, with no
restart. To have a *new* stack come up this way, edit the seed and run `make dev-fresh`. The shape is the same either way:

```yaml
local_small: { provider: ollama, model: gemma3 }   # no base_url ⇒ localhost:11434
```

Edit the other tiers to do more:

- **Use an Ollama you host on another machine**: set the `base_url` of the provider once, with no `/` at the
  end. The client adds `/api/chat`, and every Ollama lane uses it:
  ```yaml
  providers:
    ollama: { base_url: https://ollama.internal:11434 }
  tiers:
    local_small: { provider: ollama, model: mistral }
  ```
- **Run more lanes on your machine**: the read for a new company and offer drafts have the ladder
  `cheap_cloud` → `premium`, which is cloud by default. Bind those tiers to `ollama` too.

> Seeds apply once, at the first setup; see
> [connect-a-cloud-model-provider.md](connect-a-cloud-model-provider.md#2-bind-a-tier).

## 3. Start the stack

`scripts/dev.sh` (`make dev`) scans the `seeds.ai_routing` that will bind this stack. It falls back to the
`fake` that needs no network, unless the key of **every bound cloud provider** is set. The keys are:

- `anthropic` → `ANTHROPIC_API_KEY`;
- `openai` → `OPENAI_API_KEY`;
- `gemini` → `GEMINI_API_KEY`;
- `gemini_vertex` → `GEMINI_VERTEX_SA_JSON`;
- `openai_compatible` → `OPENAI_COMPATIBLE_API_KEY`;
- `jev` → `TYPESAFE_API_KEY`.

Local providers (`ollama`/`vllm`/`fake`) need no key.

The scan counts every binding, not only the tiers you use: a tier you do not use, and the `embeddings`
lane, count too. If any key is missing, the stack runs on the `fake`. That fakes the *Ollama* call too,
because the fake stands in for the whole binding, not only for the tier with no key.

The fake is only a fall back. Once the binding can serve, it wins over the flag, so a stack that can reach
its models never answers with fixed text.

So when you first start, you must do one of two things. One is to set `GEMINI_API_KEY`. The other is to
bind every cloud binding (all four tiers and `embeddings`) to `ollama` (step 2). That gives a stack that
is all local and needs no key:

```sh
make dev   # look for: "dev: the stored model binding serves the cold-start read-back (providers bound by …)"
```

> Keep keys in `.env.local` if you like; git does not track it, and `make dev` reads it.

> ⚠️ `enrich` goes on to `cheap_cloud` on a provider error or a schema failure, and the read for a new
> company starts on `cheap_cloud`. For a run that stays local, bind every tier and the embeddings lane to
> Ollama.

`make dev` starts the app on `:8080`, with the API behind it, and with no data yet. There is only the
company and admin from the first setup, and no records. Open **http://localhost:8080**, and log in as
`admin@demo.test` / `demo-password-123`. The full first-run steps are in
[tutorials/getting-started.md](../tutorials/getting-started.md).

## 4. Add a company and enrich it

1. Go to **Companies** (`#/companies`) → **New company**. Give it a domain the fetcher can read, such as `stripe.com`.
2. Open the company, and click **Read now** on the *Read from the website* card.
3. **What you should see:** a staged 🟡 enrich proposal, with an **Open inbox** button.

The fetcher sends `User-Agent: margince-siteread/1.0`. Sites that block bots (such as `tesla.com`) answer
**403**. Sites it can read: `stripe.com`, `go.dev`, `ollama.com`, `news.ycombinator.com`, `sqlite.org`.

The proposal asks you to confirm first, and shows a confidence and evidence for each field. Nothing writes to the company until you accept it in the Inbox.

The model must return the JSON shape of the facts as it writes (the `format` of Ollama). So a small model
returns JSON of the right shape, and does not break the JSON reader. The model still decides how good the
evidence is. The evidence gate drops any field whose `evidence_snippet` is not a word-for-word copy of
text on the page. That check is the guard against facts that are not on the page.

## Serve the model with vLLM instead

vLLM is the server most GPU installations run. Serve the model with it, then bind
the tiers to the `vllm` provider where step 2 binds `ollama`.
[`config/presets/qwen3_local_vllm.yaml`](../../config/presets/qwen3_local_vllm.yaml)
binds every tier this way. What we measured, and which models to serve, is in
[vllm-self-hosting.md](../explanation/vllm-self-hosting.md).

### On a Mac, install vllm-metal

vLLM runs on Apple chips through the
[vllm-metal](https://github.com/vllm-project/vllm-metal) plugin, which serves MLX
model files on the GPU:

```bash
curl -fsSL https://raw.githubusercontent.com/vllm-project/vllm-metal/main/install.sh | bash
source ~/.venv-vllm-metal/bin/activate
```

It installs vLLM next to the plugin, in its own Python `venv` (Python 3.12,
`arm64`). Serve MLX builds (`mlx-community/...-4bit`): a GGUF file from Ollama
does not load.

### Start the server

The `vllm` binding sends a plain request in the OpenAI wire format: no key, no
switches for one model. All that depends on which model you serve is the job of
the server, and belongs on its command line:

```bash
vllm serve <model> \
  --max-model-len 40960 \
  --default-chat-template-kwargs '{"enable_thinking": false}' \
  --reasoning-parser <parser> \
  --enable-prompt-tokens-details
```

| flag | why |
|---|---|
| `--max-model-len 40960` | The agent loop plans its prompt against a window of 32,768 tokens and asks for output on top. A server started with less refuses those calls with a 400 (`max_tokens=… cannot be greater than max_model_len`). 40,960 is what the Ollama adapter asks for, so the two serve the same prompts. |
| `--default-chat-template-kwargs '{"enable_thinking": false}'` | Qwen3 and Gemma 4 think by default. Measured on `Qwen3-14B` with a budget of 300 tokens and a JSON schema: 299 tokens of thinking, 29 seconds, and no answer (`content: null`, `finish_reason: length`). With this flag: the answer in 22 tokens and 2.4 seconds. A template that has no such value (`gpt-oss`, Mistral, Gemma 3) does not use it. |
| `--reasoning-parser <parser>` | `qwen3`, `gemma4`, `openai_gptoss`, … Moves any thinking out of the answer into its own field. Without it, the thinking stays in the text that the product parses as JSON. Leave it out for a model that does not think. |
| `--enable-prompt-tokens-details` | Reports cached prompt tokens, which the product records per call. Without it the count is always 0. |

The binding in the routing config then needs only the model id and, if it is not
`http://localhost:8000`, the host root (no `/v1`). The id is the name the server
answers to: what `vllm serve` was given, unless `--served-model-name` renames it.
A server on another host carries every prompt and answer over the network. Start
it with `--ssl-certfile` and `--ssl-keyfile` (or put it behind a TLS proxy), and
bind it by `https`:

```yaml
providers:
  vllm: { base_url: https://gpu-box.internal:8000 }
tiers:
  local_small: { provider: vllm, model: "mlx-community/Qwen3-14B-4bit" }
```

Two more things the server decides, and the product cannot see:

- **Sampling.** The product sends no `temperature`, so every call gets the
  defaults of the server. The server takes them from the
  `generation_config.json` of the model when the repository has one. It then logs `Default vLLM sampling parameters have been overridden`. Else it samples at `temperature` 1.0 with no
  `top_k`. Many MLX builds ship with no such file.
  - Of the ones we served, `mlx-community/Qwen3-14B-4bit`,
    `Mistral-Nemo-Instruct-2407-4bit` and `Ministral-8B-Instruct-2410-4bit` have
    none. The Gemma 3 file has no sampling values in it.
  - Qwen3 then gives far more random output than its model card asks for. Set
    the values of the card with
    `--override-generation-config '{"temperature": 0.7, "top_p": 0.8, "top_k": 20}'`
    (Qwen3 with no thinking). Section 4 of
    [vllm-self-hosting.md](../explanation/vllm-self-hosting.md) shows what it changes.
- **Memory.** vLLM takes `--gpu-memory-utilization` (0.92 by default) of the
  machine at start. It fills what the model files leave with the `KV` cache.
  Nothing else that needs the GPU fits next to it (a second model, an embedding
  server, a local judge) unless you lower that number.

**You cannot set `gpt-oss` thinking on the server.** A `reasoning_effort` level
sets its thinking. vLLM takes that only per request, and has no server default
for it; `enable_thinking` does not reach it. Served as above, it thinks at
`medium`: 188 tokens and 14 seconds on the request above, against 96 tokens and 3
seconds at `low`. The answers were correct both ways. `reasoning_effort: "none"`
is refused with a 400.

**Never publish the port.** The `vllm` binding sends no key, so only the hosts of
the product may reach the server: the local machine or a private network. The
`sovereign` profile checks only that `base_url` names such a host. It does not
stop other hosts from reaching the server, so the network around it must do
that. The own `--api-key` of
vLLM guards only the `/v1`, `/v2`, `/inference` and `/cohere` routes; `/metrics`
and `/health` stay open.

## When it does not work

| What you see | What it means, and the fix |
|---|---|
| Log says `binds provider(s) whose key is not set … offline fake` | A tier is still bound to a cloud provider whose key is missing. Bind that tier to `ollama` under **Settings → AI**. That works on the running stack, within the time the routing takes to refresh, with no restart. If you set the named key in `.env.local`, you need `make dev` again, because a program reads the environment only when it starts. |
| `Couldn't read enough from this company's site.` | The fetch failed. Either the `fake` is on (see above), a domain that blocks bots answered **403**, or the page has few words. Use a domain the fetcher can read. |
| `no field survived the no-guess evidence gate` | The model returned JSON, but no `evidence_snippet` was on the page word for word, or each confidence was ≤ 0. Expect it from small models and short pages. Try a page with more text, or `mistral` over `gemma3`. |
| A 500 that names `cannot unmarshal … into … string` | The model did not follow the schema, and returned a field of the wrong type. Switch to `mistral`. |
| Logged out right after you sign in | The API cannot be reached at the `/v1` proxy target. Check that `make dev` is running (it starts both), and use the URL it printed. |

Set `MARGINCE_LOG_LEVEL=debug` (in `.env.local`, or through `--log-level`) for full logs from the model
runtime. Small local models pass the strict evidence gate only some of the time. A cloud model (a real
provider key, with tiers on `gemini` / `anthropic` / `openai`) finds evidence more often. Ollama is good
for testing the pipeline from end to end.
