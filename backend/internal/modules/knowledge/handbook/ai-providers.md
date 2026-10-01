# AI providers

An AI provider is the vendor whose models Margince calls: Anthropic, OpenAI,
Google Gemini, Gemini on Vertex AI, an OpenAI-compatible service such as
OpenRouter, or a decision model. Each provider is set up once, on its own sheet,
and every kind of work bound to it uses that setup. Which model does which work
is chosen separately, under **Model tiers**.

### Where do I set up an AI provider?
To set up an AI provider in Margince, open **Settings**, then **AI models**, and
choose **Manage** on the provider's row under **Providers**.
The provider's sheet opens on the right. **Connection** holds its key and, for
the providers that need them, where it is reached; **Prices** holds what its
models cost.
Only an administrator can change a provider. Others can open the sheet and read
it.
Also called: model vendor, AI vendor, LLM provider, API key settings.

### What does each provider need?
Each provider in Margince needs different settings:
- **anthropic**, **openai**, **gemini**: a key. Nothing else.
- **gemini_vertex** (Gemini on Vertex AI): a service-account key file and a
  **Location**.
- **openai_compatible** (OpenRouter, Mistral, Groq, Together, or your own
  gateway): a key and a **Host**. On OpenRouter, optionally the **OpenRouter
  hosts** that may serve it.
- **jev** (TypeSafe's decision model): a key. The **Host** is optional.
- **jev_compatible** (a decision model served elsewhere, such as OpenRouter):
  a **Host**. The key is optional.
A provider set up here is used everywhere it is bound, so there is no host or
location to repeat on each model tier.

### How do I add or replace a provider's key?
To add or replace a provider's key in Margince, open the provider's sheet and
choose **Add** (or **Replace**) under **Connection**, paste the key, and choose
**Save key**.
The key is never shown again, to anyone: Margince keeps it sealed and only says
whether one is held. **Test** asks the vendor whether it accepts the key.
**Remove** deletes the key; every kind of work bound to that provider stops
until a new one is saved.

### How do I set up Anthropic, OpenAI or Gemini?
To set up Anthropic, OpenAI or Google Gemini in Margince, open the provider's
sheet, add the vendor's API key under **Connection**, and choose **Test**.
These three reach their vendor's own service, so they ask for nothing else.

### How do I connect OpenRouter or another OpenAI-compatible service?
To connect OpenRouter, or any service that speaks the OpenAI interface, open the
**openai_compatible** sheet, add the service's key, fill in **Host**, and choose
**Save**.
- For OpenRouter, choose **Preset: OpenRouter**; it fills the host for you.
- For another service, the host is its address without a version at the end:
  `https://api.mistral.ai`, not `https://api.mistral.ai/v1`.
Then choose **Test**. A model tier on **openai_compatible** with no host set
says so in its editor, and cannot be saved until the host is set here.

### How do I keep OpenRouter's processing inside the EU?
To keep OpenRouter's processing inside the EU, open the **openai_compatible**
sheet with OpenRouter as its host, and under **OpenRouter hosts** add the EU
endpoints to **Only these hosts** — for Mistral models, `mistral/eu`.
- **Only these hosts**: OpenRouter may serve requests from these and no others.
- **Never these hosts**: OpenRouter never uses these.
- **Fall back to other hosts**: whether OpenRouter may switch hosts when one
  fails. **OpenRouter decides** keeps its own default.
These apply to every model tier and the embeddings model on OpenRouter. A model
that no listed host serves fails, so check a model's endpoints on OpenRouter
before you pin it. How fast or how precisely one model is served is set on its
tier, not here.

### How do I set up Gemini on Vertex AI?
To set up Gemini on Vertex AI in Margince, open the **gemini_vertex** sheet,
add a Google Cloud **service-account key** file under **Connection**, choose a
**Location**, and choose **Save**.
The **Location** is where Google processes every call: **eu**, the EU
multi-region, keeps processing in the EU and is the usual choice. Under the
**eu_hosted** profile, only EU locations can be chosen.
Changing the location asks Google whether it serves every model you have bound
on Gemini on Vertex AI. If one is not served there, the change is refused and
names the model.
The embeddings model may use a location of its own; set it in the embeddings
model's editor under **Model tiers**.

### How do I add a decision model?
To add a decision model, set up its provider first, then bind it under **Model
tiers** with **Add decision model**.
- **jev**, TypeSafe's own service: add the TypeSafe key. Leave **Host** blank to
  use TypeSafe's own address.
- **jev_compatible**, served elsewhere: fill in **Host** with the full decision
  address; for OpenRouter choose **Preset: OpenRouter**. A key is sent if one is
  saved, and is not required.

### Where do I set up a local model, such as Ollama or vLLM?
A local model runs on your own machines, so it needs no key and has no sheet
under **Providers**. Where it is reached is set by whoever operates your
installation. The embeddings model may run on a server of its own: fill in
**Embeddings server** in the embeddings model's editor under **Model tiers**.

### Why does a model tier say its provider has no host?
A model tier says its provider has no host when the provider cannot be reached
until one is set: **openai_compatible** and **jev_compatible** have no address
of their own. Open that provider's sheet under **Providers**, fill in **Host**,
and choose **Save**; the tier can then be saved.
