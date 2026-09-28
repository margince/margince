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
| [`openrouter_cloud.yaml`](openrouter_cloud.yaml) | every tier to an OpenRouter-brokered model | `OPENAI_COMPATIBLE_API_KEY` |
| [`consumer_class_brokered.yaml`](consumer_class_brokered.yaml) | every tier to a Gemma 4 an operator could self-host, brokered at fp8 | `OPENAI_COMPATIBLE_API_KEY` |
| [`openrouter_cloud_eu.yaml`](openrouter_cloud_eu.yaml) | every lane, embeddings included, to Mistral's EU-region endpoint (`only: [mistral/eu]`) | `OPENAI_COMPATIBLE_API_KEY` |
| [`gemma4_local_ollama.yaml`](gemma4_local_ollama.yaml) | every tier to `gemma4:12b` on loopback Ollama, embeddings to `bge-m3` — zero egress | Ollama with both models pulled, nothing else |
| [`qwen3_local_vllm.yaml`](qwen3_local_vllm.yaml) | every tier to Qwen3-14B (MLX 4-bit) on loopback vLLM, embeddings to `bge-m3` on a second vLLM — zero egress | two vLLM servers started with the flags in the file's header |

`gemini_cloud.yaml` is the binding a dev stack bootstraps with today, lifted out
of `margince.dev.yaml` so it can be named and reused. The dev overlay still
carries its own copy — that file is the dev posture and has to stand alone.

**A thinking-level experiment is not a preset.** A `gemini` tier can name
`thinking_level:` ([configuration.md](../../docs/reference/configuration.md)),
and every record carries the level it ran at; the certification page credits a
preset only with records at its own rung's level. A sibling preset binding the
same Flash-Lite at `low` would therefore read absent until measured. Measuring
it is the catch: a record is filed by task, provider, model and profile, not
level, so the run writes over `gemini_cloud.yaml`'s default-level Flash-Lite
records. Run it from an uncommitted copy instead:

```sh
mkdir -p .tmp/aicert
sed 's/model: gemini-3.1-flash-lite }/model: gemini-3.1-flash-lite, thinking_level: low }/' \
  config/presets/gemini_cloud.yaml > .tmp/aicert/gemini_cloud_thinking_low.yaml
make e2e-ai ROUTING=.tmp/aicert/gemini_cloud_thinking_low.yaml
```

Commit the records only together with the same `thinking_level:` in
`gemini_cloud.yaml`; otherwise discard them with
`git restore backend/internal/compose/aicert/records`.

`openrouter_cloud_eu.yaml` fixes the HOST first and takes whatever Mistral
weights that host serves, which is a different question from
`openrouter_cloud.yaml`'s "best model per tier". Two consequences a reader
should meet before the file: `mistral-medium-3-5` has no EU endpoint, so it
cannot be bound there and `premium` and `frontier` both land on
`mistral-small-2603`, sharing one upstream endpoint; and every lane, the
embeddings lane included, must name an EU-region endpoint in `only:` — a preset
whose name ends in `_eu.yaml` is held to that by
`TestAResidencyPresetPinsEveryLaneToAnEURegion`, because an unpinned lane or a
`routing: {}` lets the broker serve the model from any region.

`openrouter_cloud.yaml` cannot serve `document_extract`: that task sends a PDF,
and the OpenAI-compatible wire's declarable carriage is text and image only. A
full certification run under it fails that one task by design — bind its tier to
a provider whose wire carries PDFs if you need it.

`consumer_class_brokered.yaml` is a **proxy, not a posture**: the weights are
ones a customer could run on a single 24GB card, the inference host is a third
party's, and it files under `cloud_frontier` because that is where the calls go.
It exists to find out which open weights carry the product before anybody buys a
GPU — the sovereign Ollama binding of the same weights
([`gemma4_local_ollama.yaml`](gemma4_local_ollama.yaml)) is a different
measurement under a different profile, and the two must not share a record
filename. It cannot serve `document_extract` either, for the reason below.

`gemma4_local_ollama.yaml` and `qwen3_local_vllm.yaml` are `sovereign`. A
certification run holds the candidate to that profile and not the judge, which is
the lane's own grader and is sent only the corpus and the candidate's answer, so
either may be certified with the default cloud judge. The Gemma records were
taken with a local one (`JUDGE=ollama:gpt-oss:20b`) before that was allowed, and
their latency includes the two models taking turns in one Ollama's memory. The
Gemma weights are 12b on every tier because that is what a 24GB machine serves
entirely on the GPU; the header of the file has the numbers.

On the OpenRouter preset's `routing:` block, and the measurements behind its
defaults: [docs/reference/openrouter.md](../../docs/reference/openrouter.md).
