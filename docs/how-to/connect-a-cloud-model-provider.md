<!-- prose:plain -->
# Connect a cloud model provider (BYOK)

Point the AI lanes at a **cloud key from the customer**. That can be Anthropic, OpenAI, or Gemini (on AI
Studio or on Vertex AI). It can also be any vendor that is OpenAI-compatible. Margince runs no model of its own: the key, the
endpoint and the DPA are yours. A provider is part of the stored **binding**, never a flag in the
binary. So to swap one is a change to a setting, with no new release and no restart.

See [explanation/agent-surface.md](../explanation/agent-surface.md) for the model runtime, and
[reference/configuration.md](../reference/configuration.md) for the full table of providers. For the
path with no cloud, see [enrich-with-a-local-llm.md](enrich-with-a-local-llm.md).

## 1. Pick a provider

| `provider` | Use it for | Key environment value | `base_url` |
|---|---|---|---|
| `anthropic` | Claude (its own Messages API: image input) | `ANTHROPIC_API_KEY` | you may set it (default `api.anthropic.com`) |
| `openai` | GPT (its own Responses API: how much it reasons, token use for the prompt cache and for reasoning, image and PDF input) | `OPENAI_API_KEY` | you may set it (default `api.openai.com`) |
| `gemini` | Gemini (its own `generateContent`: thinking level, signed thinking kept across turns, image and PDF input) | `GEMINI_API_KEY` | you may set it (default `…/v1beta`) |
| `gemini_vertex` | the same Gemini wire, served by Vertex AI at a `location` you choose. That is where Google runs the call (step 5) | `GEMINI_VERTEX_SA_JSON` (a key for a service account) | **refused**: the host comes from `location` |
| `openai_compatible` | the many smaller vendors on the OpenAI wire: Mistral, DeepSeek, Groq, Together, OpenRouter, a gateway you host, … | `OPENAI_COMPATIBLE_API_KEY` | **required** |

A binding names only the provider. **The BYOK key lives in the key vault.** You put it there under
Settings → AI → Model provider keys (step 4). An `api_key:` in a binding is an error at start, and
secrets never touch a config file.

The value in the table above is a **seed**. When a start finds one, it may seal it into the vault, and
after that you can remove it. It is still the source the runtime reads in two cases:

- an installation with no vault in its config;
- the debug and certification lanes, which have no database and open no vault at all.

Choose a provider with its **own** client (`openai` or `gemini`) for three things. You get the reasoning
and thinking settings of that vendor, token use split by kind, and a PDF sent to the model whole. `anthropic` carries
images but not PDF files, and `openai_compatible` carries only what its binding declares.

Choose `openai_compatible` for any vendor that serves `/v1/chat/completions` and needs no client of its
own. It is the right default for everything that is not Anthropic, OpenAI or Gemini.

## 2. Bind a tier

The binding lives in the database. On an installation that is **running and already has a binding**,
bind a tier under **Settings → AI**. The change works without a restart, within the time the routing
takes to refresh.

To save the first binding is the one case that needs a restart after it. A role that started with no
binding set up no model path. And the watcher that would pick up the change is built from that path. For
a new installation, declare the binding under `seeds.ai_routing` in `margince.yaml`. The one for a dev
stack is in `config/margince.dev.yaml`. Then the first start comes up with the binding in place.

A seed is used **once**, at the first setup, so a change to it after a database exists does nothing.
`make dev-fresh` runs the first setup again, and Settings → AI binds again a stack that is already up.

Either way it is the same shape, and **no key appears in it**. The key goes in on its own, in
step 4. The dev default we ship binds **`gemini`** on `cheap_cloud` and `premium`:

```yaml
# Native adapters — the key comes from the vault (Settings -> AI); GEMINI_API_KEY
# / OPENAI_API_KEY seed it, and remain the source with no vault and in the
# DB-less lanes:
tiers:
  cheap_cloud: { provider: gemini, model: gemini-3.1-flash-lite }
  premium:     { provider: gemini, model: gemini-3.5-flash }

# …or any OpenAI-compatible vendor via the generic adapter. It needs a host,
# set once on the provider (the key comes from OPENAI_COMPATIBLE_API_KEY):
providers:
  openai_compatible:
    base_url: https://api.mistral.ai # host root, NO /v1 (see the caveat below)
tiers:
  cheap_cloud:
    provider: openai_compatible
    model: mistral-small-2506        # pin an explicit version — -latest aliases drift
```

The shape of the binding (`profile` plus a `tiers` map) is checked against
`config/margince.schema.json` by any YAML tool that reads a schema. The tool then fills in keys,
checks the allowed values, and shows docs for each key. The config files we ship point at the schema
with a `# yaml-language-server:` line.

### Set the host

- `providers.<name>.base_url` is where every lane that binds that provider goes. In the app it is the
  **Service** on the sheet of the provider (Settings → AI models → Providers), with a Host field under
  **Other**. A lane names only its provider and model.
- The embeddings lane may carry its own `base_url`, for a separate server for embeddings. A vLLM you host
  serves one model per program.
- When a lane has a `base_url` and its provider names none, Margince moves it to the provider.
- For the providers on the OpenAI wire (`openai_compatible`, `openai`, `vllm`), `base_url` is the root of
  the vendor host, with _no_ version part. The client adds `/v1/chat/completions` (or `/v1/responses`).
  So a base that ends in `/v1` has it twice: `https://api.mistral.ai/v1` becomes
  `…/v1/v1/chat/completions`, and answers 404. Use `https://api.mistral.ai`.
- `gemini` works the other way. Its default base keeps `/v1beta`, and the paths are written from the
  version, so leave `base_url` out.

### Gemini thinking on a call with a schema

Gemini counts its thinking against the same token limit that the answer needs. So a request with a response
schema, and no `thinking_level` of its own, is sent `thinkingLevel: low`. This level is never above the
default of a model.

A Flash-Lite model already has `minimal` as its default, or no thinking at all on 2.5. So it is sent no
level, and keeps its own. A task that wants a different level names it on the request
(`ProviderOptions["gemini"].thinking_level`). A tier that needs a different level names it on the
binding (`thinking_level: low`). A request's own level still wins over it. See
[configuration.md](../reference/configuration.md).

### One key for every open-weight model

Give `openai_compatible` the host `https://openrouter.ai/api` and one `OPENAI_COMPATIBLE_API_KEY`. Then one
OpenRouter key reaches every model with open weights. Keep only the candidates that declare both
`structured_outputs` and `tools`. To certify one and not bind it, `make e2e-ai` takes the model by
name: `MODEL=openai_compatible:<slug> BASE_URL=https://openrouter.ai/api`.

## 3. Bind the embeddings lane separately

The embeddings lane has its own binding, separate from the chat tiers. So search over stored data still
works when the chat tiers run out of money. It can still use the same provider. The dev seed points it at the broker the
chat tiers use, so a stack needs one key, and no second provider for embeddings.

```yaml
# the dev default — the same broker and key as the chat tiers. The dev seed's
# openai_compatible host is OpenRouter (https://openrouter.ai/api), so the model
# is OpenRouter's vendor/model id; on Mistral's own host it is mistral-embed.
embeddings: { provider: openai_compatible, model: mistralai/mistral-embed-2312,
              dimensions: 1024 }
# embeddings: { provider: vllm, model: BAAI/bge-m3, base_url: http://localhost:8001 }  # its own server
# embeddings: { provider: gemini, model: gemini-embedding-001 }  # native adapter, key from GEMINI_API_KEY
# embeddings: { provider: ollama, model: bge-m3 }                # fully-local alternative
# embeddings: { provider: fake }                                 # offline dev
```

> The column in the search store is a **`vector`** with no fixed size. `dimensions:` on the embeddings
> binding declares the size it is filled with: 1536 by default, and at most 2000, the limit of a pgvector
> index. The providers with their **own** client pin that size on the wire: `gemini` through
> `outputDimensionality`, `openai` through `dimensions`. So a cloud embedding model works at whatever
> size you ask for.

> **`openai_compatible` does not.** It never sends `dimensions`, because a model without MRL behind vLLM
> answers 400 to it. On that provider, the size in the config must be the same as the model's own
> size. A binding that returns another size fails with an error.

> **Not every `openai_compatible` vendor serves the embeddings lane.** OpenRouter does
> (`/v1/embeddings`, with the list of models at `GET /api/v1/embeddings/models`). A vendor with chat only
> answers 404. Bind `embeddings:` to a vendor that has the lane (`gemini`, `openai`, Mistral, OpenRouter),
> or to a local model (`ollama` `bge-m3`).

## 4. Start the stack

The key lives in the **key vault**. Put it in under **Settings → AI → Model provider keys**, with
one field for each provider you bind. It is stored encrypted, the screen never shows it again, and you
can change it without a restart.

For local dev, the short way is still the environment. Set the key for the provider you bind in
`.env.local`: `GEMINI_API_KEY`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, or `OPENAI_COMPATIBLE_API_KEY`.
`make dev` reads `.env.local`. The first start that finds a key **seals it into the vault**, and records
where, so you can take the value out after that. Either way the binding has no key in it:

```sh
# .env.local:  GEMINI_API_KEY=…
make dev
```

To run without `make dev`, export the value by hand. Production does the same through the tool that
runs its programs:

```sh
cd backend && GEMINI_API_KEY=… go run ./cmd/api   # the stored binding applies; no routing flag
```

The API comes up on `:8080`. Use a lane whose ladder reaches your tier. For example, open a company and
click **Read now**: the read for a new company runs `cheap_cloud` → `premium`. Set
`MARGINCE_LOG_LEVEL=debug` for full logs from the model runtime.

## 5. Gemini on Vertex AI, and EU data residency

`gemini_vertex` sends the same requests as `gemini`, but to Vertex AI in a Google Cloud project of yours,
at a location you name. It is the provider to bind when the prompt must be handled inside the EU.

**Create the credential.** In a Google Cloud project with the **Vertex AI API** turned on:

1. Go to IAM & Admin → Service accounts → **Create service account**.
2. Give it **`roles/aiplatform.user`** (Vertex AI User) on the project. It needs nothing more.
3. On the account, go to Keys → Add key → **Create new key** → JSON. You can get the file once; it is the
   credential.
4. In Margince, open **Settings → AI → Model provider keys**. Paste the content of the file into the
   `gemini_vertex` row, or choose the file.

To save it checks the key with Google once, and Margince does not store a key that Google refuses.

`GEMINI_VERTEX_SA_JSON` is the way through the environment, as for any other key. It holds the
*content* of the file, not a path: one line, inside `'…'` in `.env.local`. The first start seals it
into the key vault. The project comes from the `project_id` in the key.

**Choose a location.** It belongs to the provider, and you set it once: the Location field on the sheet
of the `gemini_vertex` provider. `gemini_vertex` takes no `base_url`. The embeddings lane may name a
location of its own:

```yaml
profile: eu_hosted
providers:
  gemini_vertex: { location: eu }
tiers:
  premium: { provider: gemini_vertex, model: gemini-3.5-flash }
embeddings: { provider: gemini_vertex, location: europe-west4, model: gemini-embedding-001 }
```

When you change the location of the provider, Margince asks Google about every model in a binding at the
new location before it stores it. A model Google does not serve there refuses the change.

Choose `eu`, the location that covers the whole EU: Google keeps the work in EU countries, and picks
the region. An EU region pins one country, when that is what you need: `europe-west1`, `-west3`,
`-west4`, `-west8`, `-west9`, `-west10`, `-west12`, `-north1`, `-north2`, `-central2` or `-southwest1`.

`europe-west2` is London and `europe-west6` is Zürich, both outside the EU. `global` may run the call
in any country, and `us` is the US. None of those is in the EU.

The preset [`config/presets/gemini_vertex_eu.yaml`](../../config/presets/gemini_vertex_eu.yaml) binds the chat tiers
at `eu` and the embeddings lane at `europe-west4`, because `eu` serves no embedding model.

**Not every model is served at every location.** Settings → AI lists the locations the key can reach,
and the models the one you choose serves. It checks a model once you pick it. To save asks Google again
about every `gemini_vertex` binding the save adds or changes. So Margince refuses one that the location
does not serve, and does not store it.

If Margince cannot reach Google at that time, the save goes through. The API then logs a warning that
names the binding it could not check: when Google is down, you can still edit the routing.

**What this covers.** It covers **the model calls only**. Connectors, enrich work and mail reach their
own services whatever the profile says. The certification judge (`make e2e-ai` `JUDGE=`) is not checked
against the profile, because its corpus is not real data.

## The sovereign profile refuses every cloud provider

Under `profile: sovereign`, by design, no data may leave. A cloud provider on any tier, or on the
embeddings lane, is an **error at start**. The rule looks at the provider _name_. So it still refuses
`openai_compatible` when it points at a `localhost` URL. Only `ollama`, `vllm` and `fake` may run under
`sovereign`. Use `eu_hosted` or `cloud_frontier` for a BYOK cloud binding.

The endpoint is checked too, because the name of a local provider does not make a local endpoint.
`ollama` and `vllm` take a `base_url`. Say one points at the host of a third party. Then every call of a deployment that must keep all data in
would go over the public network.

Under this profile, the `base_url` of each binding, once looked up, must be loopback or a private range.
Your own GPU machine on your own network counts. A DNS name does not, because the address behind it can
change after start.

## Where a `base_url` may point, on every profile

Outside `sovereign`, a `base_url` makes no promise about data that leaves. But it is still the address
this server calls, so each provider reaches only what its lane is for. `ollama`, `vllm` and
`openai_compatible` may name loopback, a private range or a public host, over `http` or `https`.
`anthropic`, `openai` and `gemini` may name a **public host over HTTPS only**.

The call carries the model key of this installation, and Go keeps it when the call moves to another
host. So to point one at your own network would send that key there, and plain `http` would send
it in the open. Use `openai_compatible` for a gateway you host, or one served over `http`.

Neither lane may name `link-local` (`169.254.0.0/16`, `fe80::/10`), carrier-grade NAT, or the ranges kept
for documentation. Nothing serves a model from those, and the first of them is where every cloud
keeps its service for machine data. The rule is checked at the write, and again when the call
connects. So a DNS name that points at a refused address fails when it connects.

Margince refuses a `base_url` that carries a user name: a binding never carries a credential.
When the server sends the call on to another address, the same rule holds. Margince follows it when it keeps the same host and
the same `http` or `https`. It refuses it when it changes the host, or goes down to `http`.

## When it does not work

| What you see | What it means, and the fix |
|---|---|
| `http 404` on `…/v1/v1/chat/completions` or `…/v1/v1/responses` | `base_url` has a `/v1` part; drop it ([Set the host](#set-the-host)). The client adds it. |
| Error at start: `profile sovereign forbids cloud provider …` | A binding names a cloud provider under a profile that refuses it. Switch to `eu_hosted`/`cloud_frontier`, or bind that tier to `ollama`/`vllm`. |
| Error at start: `needs an api key — set X_API_KEY …` | The key value for the cloud provider in the binding is not set. Export the one the error names, such as `GEMINI_API_KEY`. |
| Error at start: `field api_key not found` | You put an `api_key:` in the `seeds.ai_routing` binding. Remove it. The key comes from the key vault (Settings → AI → Model provider keys), or from its seed value. |
| Error at start: `needs a base_url …` | `openai_compatible` has no host. Set `providers.openai_compatible.base_url` (the Host field on its provider sheet) to the root of the vendor host, with no `/v1`. |
| Error at start: `must name a public host` | A binding to a vendor with its own client (`anthropic`/`openai`/`gemini`) points inside your network. Bind `openai_compatible` for a gateway you host (see "Where a `base_url` may point"). |
| Error at start: `must be https` | A binding to a vendor with its own client uses `http://`. Use `https://`, or bind `openai_compatible` if the endpoint serves over `http`. |
| Error at start: `not an address inference is served from` | The `base_url` names a `link-local`, CGNAT or documentation address. Give the endpoint's own address (see "Where a `base_url` may point"). |
| `http 404` on `/embeddings` | That `openai_compatible` vendor has chat only. Bind `embeddings:` again to a vendor that serves the lane, or to a local `bge-m3` (step 3). |
| Error from the embeddings call: `returned N vectors of width W, need 1×D` | On `openai_compatible` the client never sends `dimensions`, so `dimensions:` must be the model's own size (step 3). Set it to `W`. |
| Model 404, or `model not found` | A `-latest` name that moves, or a wrong ID. Pin a model with a fixed version, or look it up at the `/models` endpoint of the vendor. |
| **Test** says `Google accepted the key but refused the call` | Google answered 403. The key is valid, but the service account does not have `roles/aiplatform.user` on its project, or the project has not turned on the Vertex AI API. Give the role or turn on the API; you do not need a new key. |
| Settings says `No service-account key is held yet` (`unavailable: no_key`) | Margince holds no key for a service account. Add it under Model provider keys (step 5), or set `GEMINI_VERTEX_SA_JSON` and restart. |
| 422 `the service-account key was not accepted by Google: … invalid_grant` | Someone revoked the key or deleted the account, or the clock of the machine is wrong. Create a new JSON key on the account. |
| 422 `invalid service account key: …` | It is not a JSON key file for a service account; the message names the field. Paste the whole file you saved. |
| 422 `gemini_vertex does not serve model … in location …` | That location has no endpoint for the model (`no_endpoint`; Settings shows `Not served in …`). Pick a model the location lists, or another location. |
| Log warning `routing saved with a gemini_vertex model unchecked` | The check on save could not run (`unreachable`). Either Margince could not reach Google, or the key does not have `roles/aiplatform.user`, or the project does not have the Vertex AI API. The binding is stored. Fix the role, then pick the model again in Settings to check it. |
| 422 `… the service-account key was refused by Google … — replace the key under Provider keys` | To save a Vertex lane asked Google, and Google refused the key: it was revoked, or the account was deleted. Create a new JSON key on the account, and replace the old one. |
| 422 `the stored gemini_vertex service-account key is not usable` | Margince cannot read the key file it holds. Replace it under Model provider keys (step 5). |
| 422 `… under profile eu_hosted: gemini_vertex location … is not an EU location` | London, Zürich, `global` and `us` are outside the EU. Choose `eu` or an EU region (step 5), or declare `cloud_frontier`. |
| Provider badge `Out of credit` (Settings → AI models) | The vendor refused because the account has run out of money or is over its limit: a 402, or a `4xx` whose text says so, such as `credit balance is too low`. Pay into the account at the vendor. Margince checks again within 15 minutes. Work waits in that time, and you pay nothing: not for the first call to the empty account, and not for any later check that failed. |
| Provider badge `Key rejected` | The vendor answered 401, or a 400 or 403 whose text says the key is not valid. A plain `does not have permission` never counts. Replace the key under Model provider keys (step 5). A **Test** that passes clears the badge at once, and a save clears it within about 30 seconds. |
| Provider badge `Unreachable` | The host did not answer, or three calls in a row returned a `5xx`. Check the `base_url` and the status of the vendor. Margince checks again after 30 seconds, and waits twice as long each time, up to 5 minutes. |
| Provider badge `Degraded` | Three calls in a row did not answer in time. Calls still go out. If it goes on, check the network path or the status of the vendor. |
| Users see `The AI provider has no credit left`, `refused the configured credential` or `is not answering right now` | The provider was in one of these states when a user asked. The request fails at once with a 503, and does not wait for a time-out. Read Settings → System health and fix the cause. Then see [recover-after-a-provider-outage.md](recover-after-a-provider-outage.md) for the work that waited. |
| Log says `offline fake`, and no binding exists | Bind a tier under Settings → AI, or run `make dev-fresh` to use a `seeds.ai_routing` you declared after the last setup. |
| Log says `offline fake`, and warns `the stored model binding cannot be served` | A binding exists but could not be built, so `--ai-fake` on the command line has it fall back to `fake`. Most of the time, the key of a vendor in a binding is missing: add it under Settings → AI → Model provider keys. |
