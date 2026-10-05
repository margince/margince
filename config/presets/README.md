# config/presets: ready-made model bindings

A preset is a `seeds.ai_routing` block you can copy into your own
`config/margince.yaml`, or point a certification run at with
`make e2e-ai ROUTING=config/presets/<name>.yaml`.

Presets are **not** read automatically. Nothing in the product loads this
directory: `MARGINCE_ENV` selects `margince.<env>.yaml` and no more. A file that
binds a cloud vendor decides where this installation's text goes, so an operator
must choose it; it is never inherited.

| preset | binds | needs |
|---|---|---|
| [`gemini_cloud.yaml`](gemini_cloud.yaml) | every tier to Gemini, embeddings to `gemini-embedding-001` | `GEMINI_API_KEY` |
| [`gemini_vertex_eu.yaml`](gemini_vertex_eu.yaml) | every tier to Gemini on Vertex AI at `location: eu` and embeddings at `europe-west4`, under `eu_hosted` | `GEMINI_VERTEX_SA_JSON` (a service-account key) |
| [`openrouter_cloud.yaml`](openrouter_cloud.yaml) | every tier to an OpenRouter-brokered model | `OPENAI_COMPATIBLE_API_KEY` |
| [`consumer_class_brokered.yaml`](consumer_class_brokered.yaml) | every tier to a Gemma 4 an operator could self-host, brokered at fp8 | `OPENAI_COMPATIBLE_API_KEY` |
| [`openrouter_cloud_eu.yaml`](openrouter_cloud_eu.yaml) | every lane, embeddings included, to Mistral's EU-region endpoint (`only: [mistral/eu]`) | `OPENAI_COMPATIBLE_API_KEY` |
| [`gemma4_local_ollama.yaml`](gemma4_local_ollama.yaml) | every tier to `gemma4:12b` on loopback Ollama, embeddings to `bge-m3`; zero egress | Ollama with both models pulled, nothing else |
| [`qwen3_local_vllm.yaml`](qwen3_local_vllm.yaml) | every tier to Qwen3-14B (MLX 4-bit) on loopback vLLM, embeddings to `bge-m3` on a second vLLM; zero egress | two vLLM servers started with the flags in the file's header |

`gemini_cloud.yaml` is the binding a dev stack bootstraps with today, lifted out
of `margince.dev.yaml` so it can be named and reused. The dev overlay still
carries its own copy, because that file is the dev posture and has to stand alone.

`gemini_vertex_eu.yaml` binds `gemini_cloud.yaml`'s Flash and Flash-Lite at its
thinking levels, on Vertex AI at an EU location, under `eu_hosted`. Frontier takes
Flash too, because Vertex serves no Pro model in the EU. A location outside the
EU member states (London and Zürich included) is refused at save. The key is a service
account holding `roles/aiplatform.user`. The steps are in
[Gemini on Vertex AI, and EU data residency](../../docs/how-to/connect-a-cloud-model-provider.md#5-gemini-on-vertex-ai-and-eu-data-residency).

**Run a thinking-level experiment from an uncommitted copy.** A `gemini` tier can
name `thinking_level:` ([configuration.md](../../docs/reference/configuration.md)),
and every record carries the level it ran at. The certification page credits a
preset only with records at its own rung's level, so a sibling preset binding the
same Flash-Lite at `low` would read absent until measured. A record is filed by
task, provider, model and profile, without the level, so measuring it in place
writes over `gemini_cloud.yaml`'s default-level Flash-Lite records. Run it from
an uncommitted copy instead:

```sh
mkdir -p .tmp/aicert
sed 's/model: gemini-3.1-flash-lite }/model: gemini-3.1-flash-lite, thinking_level: low }/' \
  config/presets/gemini_cloud.yaml > .tmp/aicert/gemini_cloud_thinking_low.yaml
make e2e-ai ROUTING=.tmp/aicert/gemini_cloud_thinking_low.yaml
```

Commit the records only together with the same `thinking_level:` in
`gemini_cloud.yaml`; otherwise discard them with
`git restore backend/internal/compose/aicert/records`.

`openrouter_cloud_eu.yaml` fixes the host first and takes whatever Mistral
weights that host serves. `openrouter_cloud.yaml` instead picks the best model
per tier. `premium` is Mistral's flagship, `mistral-medium-3-5`. `frontier` is
`mistral-small-2603`, so premium's fallback reaches a different upstream
endpoint. Every lane, embeddings included, must name an EU-region endpoint in
`only:`, because an unpinned lane or a `routing: {}` lets the broker serve the
model from any region. `TestAResidencyPresetPinsEveryLaneToAnEURegion` enforces
this for any preset whose name ends in `_eu.yaml`.

`openrouter_cloud.yaml` cannot serve `document_extract`: that task sends a PDF,
and the OpenAI-compatible wire's declarable carriage is text and image only. A
full certification run under it fails that one task by design. If you need it,
bind its tier to a provider whose wire carries PDFs.

`consumer_class_brokered.yaml` is a **proxy for a posture**: the weights are
ones a customer could run on a single 24GB card, the inference host is a third
party's, and it files under `cloud_frontier` because that is where the calls go.
It exists to find out which open weights carry the product before anybody buys a
GPU. The sovereign Ollama binding of the same weights
([`gemma4_local_ollama.yaml`](gemma4_local_ollama.yaml)) is a different
measurement under a different profile, and the two must not share a record
filename. It cannot serve `document_extract` either, for the reason given above
for `openrouter_cloud.yaml`.

`gemma4_local_ollama.yaml` and `qwen3_local_vllm.yaml` are `sovereign`. A
certification run holds the candidate to that profile. The judge is exempt: it is
the lane's own grader and is sent only the corpus and the candidate's answer, so
either preset may be certified with the default cloud judge. The committed Gemma
records used a local judge (`JUDGE=ollama:gpt-oss:20b`), so their latency includes
both models sharing one Ollama's memory. The
Gemma weights are 12b on every tier because that is what a 24GB machine serves
entirely on the GPU; the header of the file has the numbers.

On the OpenRouter preset's `routing:` block, and the measurements behind its
defaults: [docs/reference/openrouter.md](../../docs/reference/openrouter.md).
