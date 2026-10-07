<!-- prose:plain -->
# config/presets: ready-made model bindings

A preset is a `seeds.ai_routing` block. You can copy it into your own `config/margince.yaml`, or
point a certification run at it with `make e2e-ai ROUTING=config/presets/<name>.yaml`.

Presets are **not** read on their own. Nothing in the product loads this folder: `MARGINCE_ENV`
picks `margince.<env>.yaml` and no more. A file that binds a cloud vendor decides where the text of
this installation goes, so an operator must choose it; it is never inherited.

| preset | binds | needs |
|---|---|---|
| [`gemini_cloud.yaml`](gemini_cloud.yaml) | every tier to Gemini, embeddings to `gemini-embedding-001` | `GEMINI_API_KEY` |
| [`gemini_vertex_eu.yaml`](gemini_vertex_eu.yaml) | every tier to Gemini on Vertex AI at `location: eu` and embeddings at `europe-west4`, under `eu_hosted` | `GEMINI_VERTEX_SA_JSON` (a service-account key) |
| [`openrouter_cloud.yaml`](openrouter_cloud.yaml) | every tier to an OpenRouter-brokered model | `OPENAI_COMPATIBLE_API_KEY` |
| [`consumer_class_brokered.yaml`](consumer_class_brokered.yaml) | every tier to a Gemma 4 an operator could self-host, brokered at fp8 | `OPENAI_COMPATIBLE_API_KEY` |
| [`openrouter_cloud_eu.yaml`](openrouter_cloud_eu.yaml) | every lane, embeddings included, to Mistral's EU-region endpoint (`only: [mistral/eu]`) | `OPENAI_COMPATIBLE_API_KEY` |
| [`gemma4_local_ollama.yaml`](gemma4_local_ollama.yaml) | every tier to `gemma4:12b` on loopback Ollama, embeddings to `bge-m3`; zero egress | Ollama with both models pulled, nothing else |
| [`qwen3_local_vllm.yaml`](qwen3_local_vllm.yaml) | every tier to Qwen3-14B (MLX 4-bit) on loopback vLLM, embeddings to `bge-m3` on a second vLLM; zero egress | two vLLM servers started with the flags in the file's header |

`gemini_cloud.yaml` is the binding a dev stack starts with today, lifted out of
`margince.dev.yaml` so it can be named and used again. The dev overlay still has its own copy,
because that file is the dev posture and has to stand alone.

`gemini_vertex_eu.yaml` binds the Flash and Flash-Lite of `gemini_cloud.yaml` at their thinking
levels, on Vertex AI at an EU location, under `eu_hosted`. Frontier takes Flash too, because Vertex
serves no Pro model in the EU. A location outside the EU member states (London and Zürich included)
is refused at save. The key is a service account that holds `roles/aiplatform.user`. The steps are
in [Gemini on Vertex AI, and EU data residency](../../docs/how-to/connect-a-cloud-model-provider.md#5-gemini-on-vertex-ai-and-eu-data-residency).

**Run a thinking-level test from an uncommitted copy.** A `gemini` tier can name
`thinking_level:` ([configuration.md](../../docs/reference/configuration.md)), and every record holds
the level it ran at. The certification page credits a preset only with records at the level of its
own rung. So a sibling preset that binds the same Flash-Lite at `low` reads as absent until it is
measured.

A record is filed by task, provider, model and profile, without the level. So to measure
it in place writes over the Flash-Lite records of `gemini_cloud.yaml` at the default level. Run it
from a copy you do not commit instead:

```sh
mkdir -p .tmp/aicert
sed 's/model: gemini-3.1-flash-lite }/model: gemini-3.1-flash-lite, thinking_level: low }/' \
  config/presets/gemini_cloud.yaml > .tmp/aicert/gemini_cloud_thinking_low.yaml
make e2e-ai ROUTING=.tmp/aicert/gemini_cloud_thinking_low.yaml
```

Commit the records only together with the same `thinking_level:` in `gemini_cloud.yaml`. If not,
throw them away with `git restore backend/internal/compose/aicert/records`.

`openrouter_cloud_eu.yaml` fixes the host first, and takes whatever Mistral weights that host serves.
`openrouter_cloud.yaml` instead picks the best model per tier. `premium` is the flagship of Mistral,
`mistral-medium-3-5`. `frontier` is `mistral-small-2603`, so the fallback of `premium` reaches some
other upstream endpoint. Every lane, embeddings included, names an EU-region endpoint in `only:`. An
unpinned lane, or a `routing: {}`, lets the broker serve the model from any region.

`openrouter_cloud.yaml` cannot serve `document_extract`. That task sends a PDF, and the wire that is
compatible with OpenAI can declare only text and image. A full certification run under it fails that
one task by design. If you need it, bind its tier to a provider whose wire carries PDFs.

`consumer_class_brokered.yaml` is a **stand-in for a posture**. The weights are ones a customer could
run on a single 24GB card, and the host that runs the model belongs to a third party. It files under
`cloud_frontier`, where the calls go. It exists to find out which open weights carry
the product before anyone buys a GPU.

The sovereign Ollama binding of the same weights is
[`gemma4_local_ollama.yaml`](gemma4_local_ollama.yaml). It is a separate measurement under a separate
profile, and the two must not share a record file name. It cannot serve `document_extract` either,
for the reason given above for `openrouter_cloud.yaml`.

`gemma4_local_ollama.yaml` and `qwen3_local_vllm.yaml` are `sovereign`. A certification run holds
the candidate to that profile. The judge is exempt: it is the lane's own grader, and it gets only the
corpus and the candidate's answer. So either preset may be certified with the default cloud judge.

The committed Gemma records used a local judge (`JUDGE=ollama:gpt-oss:20b`), so their latency
includes both models sharing the memory of one Ollama. The Gemma weights are 12b on every tier,
because that is what a 24GB machine serves wholly on the GPU. The header of the file has the numbers.

On the `routing:` block of the OpenRouter preset, and the measurements behind its defaults:
[docs/reference/openrouter.md](../../docs/reference/openrouter.md).
