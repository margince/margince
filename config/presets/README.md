# config/presets — ready-made model bindings

A preset is a `seeds.ai_routing` block you can copy into your own
`config/margince.yaml`, or point a certification run at with
`make e2e-ai ROUTING=config/presets/<name>.yaml`.

Presets are **not** read automatically. Nothing in the product loads this
directory: `MARGINCE_ENV` selects `margince.<env>.yaml` and no more. That is
deliberate — a file that binds a cloud vendor must be chosen, not inherited,
because the choice decides where this installation's text goes.

| preset | binds | needs |
|---|---|---|
| [`gemini_cloud.yaml`](gemini_cloud.yaml) | every tier to Gemini, embeddings to `gemini-embedding-001` | `GEMINI_API_KEY` |
| [`gemini_vertex_eu.yaml`](gemini_vertex_eu.yaml) | every tier and embeddings to Gemini on Vertex AI at `location: eu`, under the enforced `eu_resident` profile | `GEMINI_VERTEX_SA_JSON` (a service-account key) |
| [`openrouter_cloud.yaml`](openrouter_cloud.yaml) | every tier to an OpenRouter-brokered model | `OPENAI_COMPATIBLE_API_KEY` |
| [`consumer_class_brokered.yaml`](consumer_class_brokered.yaml) | every tier to a Gemma 4 an operator could self-host, brokered at fp8 | `OPENAI_COMPATIBLE_API_KEY` |
| [`openrouter_cloud_eu.yaml`](openrouter_cloud_eu.yaml) | every chat tier to Mistral's EU-region endpoint (`only: [mistral/eu]`) | `OPENAI_COMPATIBLE_API_KEY` |

`gemini_cloud.yaml` is the binding a dev stack bootstraps with today, lifted out
of `margince.dev.yaml` so it can be named and reused. The dev overlay still
carries its own copy — that file is the dev posture and has to stand alone.

`gemini_vertex_eu.yaml` is the one preset that is **EU-resident rather than
EU-flavoured**: its profile is enforced, so every lane must process inside the EU
member states — the UK and Switzerland are outside it — and a binding that could
not is refused at save. It binds the same models as `gemini_cloud.yaml`. The key
is a service account holding `roles/aiplatform.user`; the steps are in
[docs/how-to/connect-a-cloud-model-provider.md](../../docs/how-to/connect-a-cloud-model-provider.md) §5.

`openrouter_cloud_eu.yaml` fixes the HOST first and takes whatever Mistral
weights that host serves, which is a different question from
`openrouter_cloud.yaml`'s "best model per tier". Two consequences a reader
should meet before the file: `mistral-medium-3-5` has no EU endpoint, so
`premium` and `frontier` both land on `mistral-small-2603`; and the embeddings lane cannot carry
the pin at all, because `routing:` is not a field on an embeddings binding.
Both are stated in the file.

`openrouter_cloud.yaml` cannot serve `document_extract`: that task sends a PDF,
and the OpenAI-compatible wire's declarable carriage is text and image only. A
full certification run under it fails that one task by design — bind its tier to
a provider whose wire carries PDFs if you need it.

`consumer_class_brokered.yaml` is a **proxy, not a posture**: the weights are
ones a customer could run on a single 24GB card, the inference host is a third
party's, and it files under `cloud_frontier` because that is where the calls go.
It exists to find out which open weights carry the product before anybody buys a
GPU — the sovereign Ollama binding of the same weights is a different
measurement under a different profile, and the two must not share a record
filename. It cannot serve `document_extract` either, for the reason below.

On the OpenRouter preset's `routing:` block, and the measurements behind its
defaults: [docs/reference/openrouter.md](../../docs/reference/openrouter.md).
