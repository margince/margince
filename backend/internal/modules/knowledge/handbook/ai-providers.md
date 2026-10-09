<!-- prose:plain -->
# AI providers

An AI provider is the company whose models Margince calls. It is Anthropic,
OpenAI, Google Gemini, Gemini on Vertex AI, an OpenAI-compatible service such
as OpenRouter, or a decision model. Each provider is set up once, on its own sheet,
and every kind of work tied to it reads that sheet. You choose which model does
which work in another place, under **Model tiers**.

### Where do I set up an AI provider?
To set up an AI provider in Margince, open **Settings**, then **AI models**, and
choose **Edit** on the provider's row under **Providers**.
The provider's sheet opens on the right. **Connection** holds its key and, for
the providers that need them, where it is reached; **Prices** holds what its
models cost.
Only an administrator or an operations user can change a provider. Others can
open the sheet and read it.
Also called: model vendor, AI vendor, LLM provider, API key settings.

### What does each provider need?
Each provider in Margince needs different settings:
- **Anthropic**, **OpenAI**, **Google Gemini**: a key. Nothing else.
- **Gemini on Vertex AI**: a service account key file and a **Location**.
- **OpenAI-compatible** (OpenRouter, Mistral, Together, Groq, DeepSeek, or your
  own gateway): a key and a **Service**.
- **TypeSafe Jev**: a key. The **Service** is the TypeSafe address by default.
- **Jev-compatible** (a decision model served somewhere else, such as
  OpenRouter): a **Service**. The key is optional.
A provider set up here is used in every place it is tied. So there is no host
or location to set again on each model tier.

### How do I add or replace a provider's key?
To add or replace a provider's key in Margince, open the provider's sheet.
Choose **Add** (or **Replace**) under **Connection**, paste the key, and choose
**Save key**.
The key is never shown again, to anyone. Margince keeps it sealed and only says
whether one is held. **Test** asks the provider whether it accepts the key.
**Remove** deletes the key; every kind of work tied to that provider stops
until a new one is saved.

### How do I set up Anthropic, OpenAI or Gemini?
To set up Anthropic, OpenAI or Google Gemini in Margince, open the provider's
sheet, add the provider's API key under **Connection**, and choose **Test**.
These three reach their own service, so they ask for nothing else.

### How do I connect OpenRouter or another OpenAI-compatible service?
To connect OpenRouter, or any service that works the OpenAI way, open the
**OpenAI-compatible** sheet. Add the service's key, pick it under **Service**,
and choose **Save connection**.
- A listed service (OpenRouter, Mistral, Together, Groq, DeepSeek) fills its
  own host, shown under the choice.
- For any other, choose **Other OpenAI-compatible service**. Fill in **Host**
  with its address without a version at the end: `https://api.mistral.ai`, not
  `https://api.mistral.ai/v1`. **How to find your host** opens help for this.
Then choose **Test**. A model tier on this provider with no host set says so in
its editor, and cannot be saved until the host is set here.

### How do I keep OpenRouter processing inside the EU?
To keep OpenRouter processing inside the EU, choose **OpenRouter (EU)** under
**Service** on the **OpenAI-compatible** sheet. Requests then go to the EU
address of OpenRouter. It handles them only inside the EU and sends them only
to providers there. It counts as EU processing when your installation requires
EU hosting, which the screen calls the **eu_hosted** profile.
It needs an OpenRouter Business or Enterprise plan, and only models that
OpenRouter offers in the EU are served there.

### How do I set up Gemini on Vertex AI?
To set up Gemini on Vertex AI in Margince, give Margince a Google Cloud
service account that may call Vertex AI, then add its key:

1. In the Google Cloud project that will pay, turn on the **Vertex AI API**
   (`aiplatform.googleapis.com`).
2. Create a service account in that project.
3. Grant it the **Vertex AI User** role (`roles/aiplatform.user`) on the
   project. That one role covers everything Margince asks of Vertex AI.
   That is writing text, making embeddings, checking that a location serves a
   model, and listing models and locations.
4. Create a **JSON key** for the service account and download it.
5. In Margince, open the **Gemini on Vertex AI** sheet. Paste or drop the key
   file under **Connection** and choose a **Location**.
   Choose **Save connection**, then choose **Test**.

The project comes from the key file itself, so there is nothing else to enter.

**Test** may say Google accepted the key but refused the call. Then the service
account is missing the **Vertex AI User** role, or the project has not turned
on the Vertex AI API.
The **Location** is where Google handles every call. **eu** covers many places
in the EU, keeps processing in the EU, and is the usual choice.
Changing the location asks Google whether it serves every model you have tied
to Gemini on Vertex AI. If one is not served there, the change is refused and
names the model.

The embeddings model may use a location of its own; set it in the embeddings
model's editor under **Model tiers**.
Gemini on Vertex AI serves the Gemini models. So a model that its own
**Prices** do not list gets the Google Gemini price, marked **From Google
Gemini**. Edit one to give Vertex its own price for that model.

### How do model prices stay current?
Model prices update on their own once a day. The **Model prices** card under
Settings → AI shows when they last synced and what changed for each provider.
Anthropic, OpenAI, Google Gemini and Gemini on Vertex AI get prices from
models.dev when their key works. The OpenRouter models that a model tier uses
get prices from the OpenRouter list. Other providers keep the prices you set.
A new chat or embedding model your key lists is added when models.dev prices it
as the same kind of model.

A price you set by hand is never changed by the
sync; remove it to hand the model back.
Turn **Auto-sync daily** off to stop the daily run, or choose **Refresh model
prices** to run it now.
Gemini on Vertex AI lists its models only for a location that a model tier
uses. So it adds new models once one of its models is tied.

### How do I add a decision model?
To add a decision model, set up its provider first, then tie it under **Model
tiers** with **Add decision model**.
- **TypeSafe Jev**: add the TypeSafe key. **Service** stays on **TypeSafe
  (default)**.
- **Jev-compatible**, served somewhere else: under **Service** choose
  **OpenRouter**, or **Other decision server** and fill in its full address. A
  key is sent if one is saved, and is not required.

### Where do I set up a local model, such as Ollama or vLLM?
A local model runs on your own servers, so it needs no key and has no sheet
under **Providers**. Where it is reached is set by whoever runs your
installation. The embeddings model may run on a server of its own: fill in
**Embeddings server** in the embeddings model's editor under **Model tiers**.

### Why does a model tier say its provider has no host?
A model tier says its provider has no host when the provider cannot be reached
until one is set. **OpenAI-compatible** and **Jev-compatible** have no address
of their own. Open that provider's sheet under **Providers**, pick its
**Service**, and choose **Save connection**; the tier can then be saved.

### How do I see how many model calls failed, how long they took and what they cost?
To read the model calls of your installation, open **Settings**, then **AI models**.
You need **AI diagnostics read** to see the figures.
- **Providers**: each row reads like "7 d: 98 calls · 11 failed (4 timeouts)".
  **Edit** opens the sheet, and **Recent calls** there splits the calls **By host**, **By model** or **By tier**.
- **Recent calls** covers **24 h**, **7 d** or **30 d**, with **p50**, **p95** and **Cost**.
  Open a row to see only the calls that ended there.
- **Model tiers**: the mark next to a tier opens its health for the last 7 days.
  It counts only the calls of the model the tier has now.
- **AI tasks**: **Edit** opens the task's sheet. **Recent calls** there shows "How calls got an answer", step by step.
  It also says why a step passed a call on, such as "timed out", "failed" or "not sure enough".
  Under "How long calls take, against the timeout", **p50** and **p95** sit beside the timeout that stops a call.

Also called: AI call log, model latency, AI cost per task, failed AI calls.

### How do I choose which OpenRouter hosts serve a model tier?
To choose how OpenRouter picks a host, open **Model tiers**, choose **Edit** on the tier, and fill **Serving**.
Host routing applies only when the tier's provider is **OpenAI-compatible** and its **Service** is OpenRouter.
- Leave **Serving JSON** empty for the shipped default: sort by throughput, fp16 or bf16, require parameters.
- Write `{}` to let OpenRouter route on its own.
- To speed up the slowest calls, sort by `throughput`. Sorting by `latency` reached slower hosts in our tests.
- `preferred_max_latency` and `preferred_min_throughput` only change the order of hosts. Use them with a sort or a filter.
- `max_price` leaves out the hosts that cost more. With a sort, it can leave out the host the sort would choose.
The form checks with the server as you type, and lists each problem with its line.
**What OpenRouter will be asked for** shows the whole request in plain words.
Each line says where it comes from: **Margince default**, **Connection**, **You set here** or **Each task**.
Also called: OpenRouter routing, provider routing, host sort, OpenRouter throughput.

### How do I stop OpenRouter hosts from keeping or training on our data?
To set privacy for OpenRouter, open the **OpenAI-compatible** sheet under **Providers**.
While its **Service** is OpenRouter, the sheet shows **OpenRouter settings**.
- **Zero data retention**: only hosts that keep nothing.
- **Refuse hosts that train on prompts**.
- **Distillable models only**: only models whose license allows reusing output.
- **Allow fallbacks**: try another host when the preferred one fails.
- **Use only these hosts** and **Never use**: lists of OpenRouter host names.
These apply to every tier on the connection, and no tier can loosen them.
Saving a tier with a value that disagrees with the connection is refused.
Settings in your OpenRouter account also apply, and this sheet does not show them.
Also called: OpenRouter privacy, data retention, ZDR, data collection.

### How do I set a task's thinking level and timeouts?
To change how one AI task calls its model, open **AI tasks** under **Settings**, then **AI models**, and choose **Edit** on the task.
The same users who may change a provider may change these.
- **Thinking level** is sent to every provider through its own thinking setting.
  A model with no thinking control ignores it. **Default (the binding and each prompt decide)** sends nothing of its own.
- **Decision model timeout** shows only on a task that asks a decision model first.
  It runs from 5 to 60 seconds, 15 by default. After it, the task falls back to its tier model.
  If **p95** is close to the line and the fallback answers, give it a little more time.
- **Model call timeout** runs from 10 to 300 seconds, 300 by default. A call that runs longer is stopped, and the next tier gets a try.
  Lower it when a host stops answering and the tier above it works.

**Save settings** applies to the whole installation within a minute. **Reset to defaults** removes the task's own values.
To check the change, come back to **Recent calls** after some calls. A call's own page shows the **Request settings sent** with it.
Also called: AI timeout, reasoning effort, thinking budget, slow AI task.

### How do I see whether an AI provider is working?
To see whether an AI provider is working in Margince, open **Settings**, then **System health**, and read the **AI provider status** card.
**AI models** also shows a second badge on each provider under **Providers**.
The card lists only providers that are not answering as normal, with why, **Started** and **Next check**. With none listed, it reads "All AI providers are answering."
It shows what the server and the background worker found in their calls, and only roles with **AI diagnostics read** can open it.
Also called: is the AI down, AI outage, model provider health, AI provider status.

### What does "Out of credit", "Key rejected", "Unreachable" or "Degraded" mean on an AI provider?
- "Out of credit" means the provider account has no credit or quota left: top it up.
- "Key rejected" means the provider refused the API key: replace it under **Providers**.
- "Unreachable" means the provider's host is not answering: check its status page.
- "Degraded" means some requests fail but calls still go through.
Margince checks again by itself; a key **Test** that works clears it at once, and saving a key clears it within about 30 seconds. The first three stop calls until then.
Also called: provider badge, provider status, credit exhausted, 401, API key refused.

### What happens to an AI task while its provider is down?
While a provider is out of credit, rejecting its key or unreachable, Margince skips it. Three server errors in a row also count as unreachable.
A task with another model on a working provider uses that one. Timeouts only mark a provider **Degraded**, which skips nothing.

When every model a task can use is blocked, background work such as mail checks and enrichment waits. It tries again at the provider's next check without using up its tries.
A request someone makes in the app fails at once, with a message to contact their administrator. Search indexing is the exception: it is refused and tries again on its own schedule.

Under **Settings**, then **AI models**, the dot before such a task in **AI tasks** turns red. Select the task's name to read what it does in this state.
Also called: AI task stuck, task waiting, deferred AI work, AI outage per task.

### What does a provider badge on AI models mean?
A badge next to a provider under **Providers** means Margince has found it not answering: **Out of credit**, **Key rejected**, **Unreachable** or **Degraded**.
Fix the cause named on **Settings**, then **System health**, in the **AI provider status** card. Testing a new key under **Providers** clears it at once.
A rate limit reply or a "no permission for this model" refusal does not mark a provider: Margince tries the next model. Only three timeouts in a row mark it **Degraded**.
Also called: provider status, provider down, API key refused, out of credit.
