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
Only an administrator or an operations user can change a provider. Others can
open the sheet and read it.
Also called: model vendor, AI vendor, LLM provider, API key settings.

### What does each provider need?
Each provider in Margince needs different settings:
- **Anthropic**, **OpenAI**, **Google Gemini**: a key. Nothing else.
- **Gemini on Vertex AI**: a service-account key file and a **Location**.
- **OpenAI-compatible** (OpenRouter, Mistral, Together, Groq, DeepSeek, or your
  own gateway): a key and a **Service**.
- **TypeSafe Jev**: a key. The **Service** defaults to TypeSafe's own address.
- **Jev-compatible** (a decision model served elsewhere, such as OpenRouter):
  a **Service**. The key is optional.
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
**OpenAI-compatible** sheet, add the service's key, pick it under **Service**,
and choose **Save connection**.
- A listed service — OpenRouter, Mistral, Together, Groq, DeepSeek — fills its
  own host, shown under the choice.
- For any other, choose **Other OpenAI-compatible service** and fill in **Host**
  with its address without a version at the end: `https://api.mistral.ai`, not
  `https://api.mistral.ai/v1`. **How to find your host** opens the setup guide.
Then choose **Test**. A model tier on this provider with no host set says so in
its editor, and cannot be saved until the host is set here.

### How do I keep OpenRouter's processing inside the EU?
To keep OpenRouter's processing inside the EU, choose **OpenRouter (EU)** under
**Service** on the **OpenAI-compatible** sheet. Requests then go to OpenRouter's
EU address, which processes them only inside the EU and sends them only to
providers there; it counts as EU inference under the **eu_hosted** profile.
It needs an OpenRouter Business or Enterprise plan, and only EU-eligible models
are served there.

### How do I set up Gemini on Vertex AI?
To set up Gemini on Vertex AI in Margince, give Margince a Google Cloud
service account that may call Vertex AI, then add its key:

1. In the Google Cloud project that will be billed, enable the **Vertex AI API**
   (`aiplatform.googleapis.com`).
2. Create a service account in that project.
3. Grant it the **Vertex AI User** role (`roles/aiplatform.user`) on the
   project. That one role covers everything Margince asks of Vertex AI:
   generating text, creating embeddings, checking that a location serves a
   model, and listing models and locations.
4. Create a **JSON key** for the service account and download it.
5. In Margince, open the **Gemini on Vertex AI** sheet, paste or drop the key
   file under **Connection**, choose a **Location**, choose **Save connection**,
   then choose **Test**.

The project comes from the key file itself, so there is nothing else to enter.
If **Test** says Google accepted the key but refused the call, the service
account is missing the **Vertex AI User** role or the project has not enabled
the Vertex AI API.
The **Location** is where Google processes every call: **eu**, the EU
multi-region, keeps processing in the EU and is the usual choice. Under the
**eu_hosted** profile, only EU locations can be chosen.
Changing the location asks Google whether it serves every model you have bound
on Gemini on Vertex AI. If one is not served there, the change is refused and
names the model.
The embeddings model may use a location of its own; set it in the embeddings
model's editor under **Model tiers**.
Gemini on Vertex AI serves Gemini's models, so a model its own **Prices** do not
list is priced at Google Gemini's price, marked **From Google Gemini**. Edit one
to give Vertex its own price for that model.

### How do model prices stay current?
Model prices update themselves once a day. The **Model prices** card under
Settings → AI shows when they last synced and what changed for each provider.
Anthropic, OpenAI, Google Gemini and Gemini on Vertex AI are priced from
models.dev when their key is usable; the OpenRouter models a model tier uses are
priced from OpenRouter's own list. Other providers keep the prices you set. A new
chat or embedding model your key lists is added when models.dev prices it in the
same lane. A price you set by hand is never changed by
the sync; remove it to hand the model back. Turn **Auto-sync daily** off to
stop the daily run, or choose **Refresh model prices** to run it now.
Gemini on Vertex AI lists its models only for a location a model tier uses, so
it adds new models once one of its models is bound.

### How do I add a decision model?
To add a decision model, set up its provider first, then bind it under **Model
tiers** with **Add decision model**.
- **TypeSafe Jev**: add the TypeSafe key. **Service** stays on **TypeSafe
  (default)**.
- **Jev-compatible**, served elsewhere: under **Service** choose **OpenRouter**,
  or **Other decision server** and fill in its full address. A key is sent if
  one is saved, and is not required.

### Where do I set up a local model, such as Ollama or vLLM?
A local model runs on your own machines, so it needs no key and has no sheet
under **Providers**. Where it is reached is set by whoever operates your
installation. The embeddings model may run on a server of its own: fill in
**Embeddings server** in the embeddings model's editor under **Model tiers**.

### Why does a model tier say its provider has no host?
A model tier says its provider has no host when the provider cannot be reached
until one is set: **OpenAI-compatible** and **Jev-compatible** have no address
of their own. Open that provider's sheet under **Providers**, pick its
**Service**, and choose **Save connection**; the tier can then be saved.

### How do I see whether an AI provider is working?
To see whether an AI provider is working in Margince, open **Settings**, then **System health**, and read the **AI provider status** card; **AI models** also shows a second badge on each provider under **Providers**.
The card lists only providers that are not answering normally, with why, **Started** and **Next check**. It reads "All AI providers are answering." when none is listed.
It shows the view of the server and the background worker together, learned from their calls, and only roles with access to AI diagnostics can open it.
Also called: is the AI down, AI outage, model provider health, AI provider status.

### What does "Out of credit", "Key rejected", "Unreachable" or "Degraded" mean on an AI provider?
"Out of credit" means the provider account has no credit or quota left, so top it up; "Key rejected" means the provider refused the API key, so replace it under **Providers**; "Unreachable" means the provider's host is not answering, so check its status page and host; "Degraded" means some requests fail but calls still go through.
Margince checks again by itself; a successful key **Test** clears it at once, and saving a key within about 30 seconds. The first three stop calls until then.
Also called: provider badge, provider status, credit exhausted, 401, API key refused.

### What happens to an AI task while its provider is down?
While a provider is out of credit, rejecting its key or unreachable, Margince skips it, and a task with another model on a working provider uses that one.
When every model a task can use is blocked, background work such as mail checks and enrichment waits and tries again at the provider's next check without using up its attempts, and a request someone makes in the app fails at once with a message to contact their administrator.
Under **Settings**, then **AI models**, the dot before such a task in **AI tasks** turns red; select the task's name to read what it does in this state.
Also called: AI task stuck, task waiting, deferred AI work, AI outage per task.

### What does a provider badge on AI models mean?
A badge next to a provider under **Providers** means Margince has found it not answering: **Out of credit**, **Key rejected**, **Unreachable** or **Degraded**.
Fix the cause named on **Settings**, then **System health**, in the **AI provider status** card; testing a new key under **Providers** clears it at once.
A rate-limit reply or a "no permission for this model" refusal does not mark a provider: Margince tries the next model. Only three timeouts in a row mark it **Degraded**.
Also called: provider status, provider down, API key refused, out of credit.
