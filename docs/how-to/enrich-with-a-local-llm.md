<!-- prose:plain -->
# Enrich a company with a local LLM (Ollama)

Run the AI lanes (company **enrich**, and the read for a new company) against an
[Ollama](https://ollama.com) on your machine, or on a server you host. You use it in place of a cloud
model, with no Anthropic key. To get the data is the job of the app. It fetches the page behind an SSRF guard, then
asks the model only to *pull out* facts that the page backs up. The `enrich` task goes to the
`local_small` tier first, so a local model serves it.

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
[ollama-self-hosting.md](../reference/ollama-self-hosting.md).

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

On a running stack, do it under **Settings → AI**, which works at once. To have a *new* stack come up this
way, edit the seed and run `make dev-fresh`. The shape is the same either way:

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
