<!-- prose:plain -->
# Model binding

How the model binding names providers, and where the key of each provider comes from. Then what a
binding may carry and reach, and what to do after a binding change. How to bind a model is in
[connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md).

## Model credentials and the binding

Model credentials (BYOK cloud tiers) live in the **key vault**. An admin puts them there under
Settings → AI → AI models → Providers. No binary flag holds one, and the binding does not either: a
binding names providers and never a credential.

The normal environment variable of a provider sets its key at boot. It wins over a key saved in
Settings and locks it: a save there gets a 409. Margince reads it where it is and never seals it
into the vault. The debug and certification lanes, which run with no database, read only the
variable.

The **binding** is a stored setting; there is no routing file. A new installation declares it under
`seeds.ai_routing` (a file in `config/presets/` is one to copy); you bind a running one again from Settings
→ AI. A dev stack is bound by `seeds.ai_routing` in `config/margince.dev.yaml`.

The lanes that test a binding without opening a database get their model from the command line.
`worker siteread` and `worker aitask` take `--model provider:model` or `--ai-fake`, and
`make e2e-ai` takes `MODEL=` and `JUDGE=` (see [Other environment variables](programs.md#other-environment-variables)). The shape of a
binding (`profile` plus a `tiers` map) is under `$defs.aiRouting` in
[`config/margince.schema.json`](../../config/margince.schema.json). An editor checks a
`seeds.ai_routing` block against it.

The key of a cloud provider lives in the **key vault**. An admin puts one there at Settings → AI →
Models, on the Providers card (`PUT /v1/ai/provider-keys/{provider}`). Test on the same card asks
the vendor whether the stored key works; what it calls for each provider is in
[ai-provider-key-test.md](ai-provider-key-test.md).

The environment variable in the table below sets the key at boot, and then wins over the key saved
in the vault. Both routes fail closed. Margince refuses a bound provider with a key from no route
when it builds the adapter, and names what is missing.

| provider | key env var | `base_url` | notes |
|---|---|---|---|
| `fake` | (none) | (none) | a fixed stub that runs offline (dev/test) |
| `ollama` | (none) | can be skipped (default `localhost:11434`) | local; may run under `sovereign` |
| `vllm` | (none) | can be skipped (default `localhost:8000`) | local; may run under `sovereign` |
| `anthropic` | `ANTHROPIC_API_KEY` | can be skipped (default `api.anthropic.com`) | BYOK cloud |
| `openai_compatible` | `OPENAI_COMPATIBLE_API_KEY` | **required** | BYOK cloud, the general OpenAI wire (OpenAI, Mistral, DeepSeek, Groq, Together, OpenRouter, …) |
| `openai` | `OPENAI_API_KEY` | can be skipped (default `api.openai.com`) | BYOK cloud, its own Responses API |
| `gemini` | `GEMINI_API_KEY` | can be skipped (default `generativelanguage.googleapis.com/v1beta`) | BYOK cloud, its own `generateContent` |
| `gemini_vertex` | `GEMINI_VERTEX_SA_JSON` (the JSON of the key file of the service account) | **refused**: the host comes from `location` | BYOK cloud, the `gemini` wire served by Vertex AI; **`location` required** |
| `jev` | `TYPESAFE_API_KEY` | can be skipped (default `https://api.typesafe.ai/v1/systemone`, the full endpoint) | decisions lane only; the TypeSafe API |
| `jev_compatible` | `JEV_COMPATIBLE_API_KEY` (**not needed**: sent when stored, never asked for) | **required**, the full endpoint | decisions lane only; any server on the Jev wire: OpenRouter (`https://openrouter.ai/api/alpha/decisions`, key = your OpenRouter key) or a server you run on your own (`http://127.0.0.1:8767/v1/systemone`, most often with no key) |

`base_url` and `location` belong to the provider, set once under `providers:`. That is
`providers.<name>.base_url`, `providers.gemini_vertex.location`, and for an OpenRouter host
`providers.openai_compatible.upstream` (`only`, `ignore`, `allow_fallbacks`). Every lane that binds
that provider reads them. In the app they are the fields on the provider sheet, or
`PUT /v1/ai/provider-settings/{provider}`. Only the embeddings lane may carry its own `base_url`,
`location` or upstream pins, which then come before those of the provider for that lane.

A tier or decisions lane that still writes one (the older form) has it moved to its provider when
the provider names none. A write that does not agree with the provider gets 422 `moved_to_provider`
from `PUT /ai/routing`.

The `base_url` of a decision provider is the whole endpoint URL, and Margince sends to it as
written; it adds nothing.

`base_url` for the providers whose adapter adds `/v1` (`openai_compatible`, `openai`, `vllm` and
`anthropic`) is the **root of the vendor host**. The adapter adds `/v1/chat/completions` (or
`/v1/responses`, `/v1/messages`). A base that ends in `/v1` is stored and dialled as its root, so
`https://api.mistral.ai/v1` and `https://api.mistral.ai` reach the same place. `gemini` is the other
way round: its default base keeps the `/v1beta` part, and the paths are written from that version.

`location` belongs to `gemini_vertex` only. It is set on the provider (and may be set again on
`embeddings:`), and refused on every other provider.

- It names the Vertex AI location that serves the call and works on the prompt: `eu`, `us`,
  `global`, or a region such as `europe-west4`. The API host comes from it, so Margince accepts no
  `base_url`.
- `eu` and the EU regions keep the work in the EU; London `europe-west2`, Zürich `europe-west6`,
  `global` and `us` do not. `eu_hosted` and `cloud_frontier` let in every location; `sovereign`
  refuses `gemini_vertex` at any location.
- When an admin stores a `gemini_vertex` binding, Margince asks Google whether the location serves
  the model, and refuses it with a 422 if not. A change to the location of the provider does the
  same for every bound model.
- The key is the JSON key file of a service account that holds `roles/aiplatform.user`.
  `GEMINI_VERTEX_SA_JSON` carries what is in the file, not a path.
  [how-to/connect-a-cloud-model-provider.md](../how-to/connect-a-cloud-model-provider.md#5-gemini-on-vertex-ai-and-eu-data-residency)
  walks through it.

### What a binding can be handed (documents, scans, images of forms)

`document_extract` reads a file by handing it to the bound model when the model takes that media
type. When the model does not, it uses the text lane. What each provider carries comes from its
**wire**, and is fixed in the adapter:

| provider | carries |
|---|---|
| `anthropic`, `gemini`, `openai` | `image/*` and `application/pdf`: all three wires take a document part as their own. On Anthropic that is the `document` block of the Messages API, which every live model accepts and which needs no beta header |
| `ollama` | `image/*`, as the `images` array per message of the chat API; that wire has no document part. A model that cannot see images, pulled into the binding, fails at the runner, where you can see it |
| `openai_compatible`, `vllm` | what the binding declares; see `input:` below |
| `fake` | `image/*` and `application/pdf` (the offline stub copies the wires above) |

A media type outside what a binding carries is **refused, never dropped**. The run says which file
it could not read, and does not answer about a document the model never had. An `ollama` binding
takes bytes in the request only. `anthropic` takes bytes in the request, or an `http(s)` URL it gets
itself.

**What it means to carry a document.** Know these limits before you turn on an attachment lane:

- **How the file entered decides the lane.** The carry check looks at the kind. The AI lane reads
  the content type the file is stored with, and adds no second source it trusts. What that type
  means turns on how the file entered.

  A **captured** attachment carries the type read from its bytes. If the sender claims another type,
  Margince records that claim and does not use it. So someone outside can change the lane only
  through the bytes they sent. A file **uploaded through the API** carries the type its uploader
  declared, or the one its file name gives (see [uploads](feature-settings.md#uploads)). It is not read from the bytes.

  Before the bytes become a wire part, that type has to hold up. A file that claims a kind with a
  clear signature (PNG, JPEG, GIF, WebP, BMP, PDF, HEIC, HEIF) must carry that signature. A file
  that claims any other image type is refused when its bytes are text. `image/svg+xml` is refused on
  every wire, no matter how its bytes look. It matches `image/*` by its first part on a binding that
  declares that, and no model that sees images reads it as an image.

  When the check refuses here, that is a problem of the file, not a limit of what the binding
  carries. So a retry on another binding would read the same bytes the same way. The check decides
  nothing else. The stored type stays the source every other reader trusts. A kind whose bytes carry
  no signature this build can name goes through with no check. `input: [image]` still says what
  Margince will carry, not what an image shows.
- **The secret stripper skips what is inside attachments.** It runs over the outbound payload, but
  an attachment goes in that payload **as `base64`**, and the rules match the plain text of a
  secret. The rules cannot see a credential inside an attachment in that form. The same file sent as
  text is cleaned.

To keep secrets out of an attachment, narrow the tier with `input: [text]`, or pick a `profile:`
that sends less. The stripper does not read inside attachments.

### `input:`: what the model of a binding can take

A chat tier may declare the kinds of input its model accepts:

```yaml
premium:
  provider: openai_compatible        # host on providers.openai_compatible
  model: mistralai/mistral-large-2512
  input: [text, image]
```

The field does two separate jobs, and which one turns on the provider under it.

**It decides for `openai_compatible` and `vllm`.** Only there does the code not know the answer.
They are one adapter pointed at an endpoint the operator picks, so whether an image may go out turns
on which model was bound. Without it there, it means text only.

**On every other provider it narrows.** What they carry is fixed in the wire, and a declaration
means *at most this*: the binding carries only what both let through. So this keeps scanned invoices
off a model that sends data out, and keeps that model for text:

```yaml
premium:
  provider: gemini
  model: gemini-3.1-flash-lite
  input: [text]        # this tier is sent no attachment at all
```

A declaration can only remove. It never gives a provider a lane its wire does not have.
`input: [text, image]` on a binding whose wire has no image part still carries no image. Without it,
it means *what that provider carries*.

This governs what Margince sends. As with `profile:`, it makes no claim about what the endpoint you
picked does with what it gets.

- **Leave it out** to take the answer of the provider; write `input: [text]` to send a tier no
  attachments at all. An `openai_compatible`/`vllm` binding that declares nothing carries no
  attachment parts, and *refuses* an attachment, not drops it.
- **The values it accepts are `text` and `image`**, and `text` must be there. A kind it does not
  know is a startup error that names the accepted set. So a wrong letter cannot turn off the feature
  it should turn on.
- `pdf` is not accepted. On one gateway a PDF goes in a vendor extension to the request. On an
  endpoint you run on your own it goes in nothing at all. So the word would mean another thing per
  vendor. On a binding whose wire has no document part, the document lane reads the text the PDF
  already holds, and sends that. A scan has no text to read, since its pages are images, and the
  reading says so and does not guess.
- **The `embeddings:` binding does not take it**, because that lane sends no attachments.
- **Margince does not check a declaration.** A binding that claims more than its model serves fails
  on the wire, where you can see it.

**The ladder cuts both ways.** A task carries only what all its bound rungs carry. The budget guard
can move a call down to a lower rung at any time. So to turn on a lane, every rung needs the
declaration: one `openai_compatible` rung that declares nothing blocks it for the whole task. To
narrow one rung narrows the whole task, which is right when the narrowing is a privacy decision.
Look up the own answer of a model before you declare it; on OpenRouter:

```sh
curl -s https://openrouter.ai/api/v1/models \
  | jq '.data[] | select(.id=="<slug>") | .architecture.input_modalities'
```

### `thinking_level:`: how hard a Gemini tier thinks

A `gemini` tier may name the thinking level Margince sends with its requests, when the request names
none of its own:

```yaml
cheap_cloud: { provider: gemini, model: gemini-3.1-flash-lite, thinking_level: low }
```

- **Without it, the adapter decides.** A structured request thinks at `low`, and a Flash-Lite keeps
  its own lower default (`minimal`), so it gets no level at all. So to name `low` on a Flash-Lite
  *raises* its thinking.
- **It comes before the floor of a site.** A site in `backend/api/ai-tasks.yaml` may declare
  `thinking:` (the two company conversations of `cold_start` declare `low`). That is a floor: at
  least this much, and never less than the adapter would send without it. It applies only where the
  binding names no level of its own. On a structured request the adapter already sends `low`, under
  the own default of a Gemini 3 Flash or Pro model. A `low` floor does not undo that.
- **The level on the request wins over both** (`ProviderOptions["gemini"].thinking_level`).
  Strongest first: the level on the request, the one on the binding, the site floor, the adapter
  default. What every provider gets for a floor, with the `routing.reasoning_effort` of the broker:
  [ai-thinking.md](ai-thinking.md).
- **It accepts** `minimal`, `low`, `medium` and `high`. Anything else is a startup
  error. So is the field on a provider other than `gemini`, on the `embeddings:` lane, or on a
  Gemini 2.5 model (which answers the field with a 400). Which levels one Gemini 3 model takes is
  for the vendor to say: `gemini-3.1-pro-preview` refuses `minimal`.
- **To clear it through the API, send `default`.** A routing write that leaves out `thinking_level`
  keeps the stored level, while provider, host and model stay the same. One that sends
  `thinking_level: default` clears it, and `default` itself is never stored.
- **Thinking is output.** Gemini counts it against the same `maxOutputTokens` as the answer. The
  adapter reports it as reasoning tokens inside the output count, so it is counted and priced. A
  structured answer whose thinking used up the limit runs again with more room.
- Settings → AI has no field for it. To store a tier bound to the same model again keeps the stored
  level, and to point the tier at another model drops it.

### Egress rules for a binding

Margince refuses a cloud binding at startup under `profile: sovereign` (no egress, by design). It
also refuses a **local provider on a host someone else runs**. Without that, the provider
name alone would let a deployment declare no egress, and send every call over the public internet.

- Under that profile, the `base_url` of each binding must name an address on systems you control. A
  binding that leaves it out gets the provider default, which is loopback.
- A binding may name: loopback or a private range (`10.x`, `172.16–31.x`, `192.168.x`, or an
  `IPv6 unique-local` address). A host in a private range on another machine counts: your own GPU
  box is your own system.
- A **DNS name is refused**, even when it looks internal. To look it up at boot says only where it
  pointed at boot, and the answer can change an hour later. Use the IP, or `localhost`.

Two egress rules bind every profile. Margince checks them when the binding is written, and again on
the socket the call opens. So a name that points (or later points again) to a refused address is
stopped at connect time:

- `ollama`, `vllm`, `openai_compatible` and `jev_compatible` may reach loopback, a private range, or
  a public host. That covers the local model, the GPU box, and the gateway you run.
- `anthropic`, `openai`, `gemini` and `jev` may reach a **public host, over https only**. Their
  `base_url` takes the place of the own API host of a vendor. The call carries the model key of this
  installation in a header (`x-api-key`, `x-goog-api-key`) that Go does not remove across hosts. To
  reach a gateway on your own network, or one served over http, bind `openai_compatible` instead.

No lane may reach the ranges that serve nobody. They are link-local (`169.254.0.0/16`, `fe80::/10`),
carrier-grade NAT, the documentation ranges, and the forms that carry another address inside them.
Link-local is where the metadata service of every cloud lives. Margince refuses a `base_url` that
carries user data (`http://user:token@host`), because a binding never carries a credential.

A redirect follows the same rule as the binding. The outbound client follows a redirect that stays
on the same host and keeps its scheme. It refuses one that changes host or moves from https to http.
Either would carry the model key to a place the binding never named.

An editor with a YAML language server picks up
[`config/margince.schema.json`](../../config/margince.schema.json) (named in the first line of the
shipped config files). It gives completion, enum checks and help when you point at a key, across the
whole file. At runtime the parser is still the only source Margince trusts.

The `embeddings:` binding also takes `dimensions`, the vector width the provider is asked to send
back. The default is `1536` (the width Gemini names as best). `0`, or no value, means the default. A
value an operator sets must be within `[1, 2000]` (`ai.ParseRouting`); outside that range it is a
boot error, never a runtime one.

To change `dimensions` (or the provider or model) needs **no migration**. The embedding column is
`vector` with no fixed width, so a config edit + restart (`make dev`) applies at once. The next file
that comes in, and the next query, both use the new width. Rows already there keep the mark of the
old identity until a new embed covers them; see below.

## Embedding binding changes & reindex

Margince marks every embedding row with the identity (`provider/model@dimensions`) it was written
under. On boot, the seed step sets the `embed_store_binding` marker of the deployment at the
identity in the config. The store may already hold rows under **another** one, because an operator
changed the binding since the last boot. Then Margince logs that the two do not match at **error**
level, so an admin sees it, and boot still works.

Search stays available the whole time. Vector ranking filters to the **current** identity: rows with
an old identity are dropped, not queried at the wrong width. Text (`FTS`) search and any rows already
current keep answering. To reindex to the new identity is an ops action that boot never makes run.

The gap shows up in two places. One is the `embed:` line of `/readyz`. The other is a banner for
admin and ops only in the frontend shell. The `embed:` line reads `active` | `needs_reindex` |
`reembedding` | `unknown`. It reads `unknown` when no embed lane is bound, or when the marker read
fails. It never makes `/readyz` return 503.

To set it right, use three **human-only** routes (`x-agent-access: human-only`; a passport or agent
principal never reaches them):

- `GET /embeddings/reindex/status`: the binding marker, plus a live scan per workspace of the
  records still waiting. Admin and ops only, through the `read` grant of the `embedding_reindex`
  object. Manager, rep and read_only hold no grant and get 403, as the banner that reads it is also
  for ops only.
- `GET /embeddings/reindex/preview`: the scope before the spend. It gives counts of waiting records
  for all workspaces and per workspace, and a cost guess. It also shows how much of the advisory
  budget of each workspace the reindex would use.

  The guess is always `heuristic`: a token number from the shape of the work, never priced from the
  `ai_call` history. Admin and ops only, with the same `read` grant as the status route. The embed
  lane itself does not count against the budget (routing never queues it or moves it down). So this
  only tells, and never blocks.
- `POST /embeddings/reindex`: admin and ops only (the `update` grant of the `embedding_reindex`
  object). It claims the binding marker (`idle` → `reembedding`), and queues one embed job for all
  workspaces. The job can go on after a stop. It compares a content hash and the identity, so a row
  already current costs nothing to pass again. One live reindex at a time (`409 reindex_running`).

Right answers never wait for a reindex to be done. Search filters to the current identity, so search
hides rows still under an old identity until a new embed covers them. It never serves them as if
they were current.

Two things an operator should watch, checked against the current vendor docs:

1. **Not every `openai_compatible` vendor serves the embeddings lane.** OpenRouter does
   (`/v1/embeddings`, with the catalog at `GET /api/v1/embeddings/models`). Vendors with only chat,
   such as Groq and DeepSeek, answer 404.

   Bind `embeddings:` to a vendor that has the lane (`gemini`, `openai`, Mistral, OpenRouter) or a
   local model (`ollama` `bge-m3`). On `openai_compatible` the adapter never sends `dimensions`. So
   the width in the config must be the same as the own width of the model. A binding that returns
   another width fails with a clear error.
2. **Vendor `-latest` model names drift.** Some are on their way out (for example at Mistral). Pin a
   model ID with a version, or look it up through the `/models` endpoint of the vendor. Do not type
   a moving name into the config.
