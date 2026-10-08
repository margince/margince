<!-- prose:plain -->
# Self-hosting models with vLLM: what we measured

**Tested 2026-09-24** on the same machine as
[ollama-self-hosting.md](ollama-self-hosting.md) (`Mac mini M4`, 24 GB of memory
shared by CPU and GPU, about 17.8 GiB of it usable by the GPU). We used **vLLM
0.30.0** through the Apple plugin **`vllm-metal` `0.30.0.dev20260924`**, and the
certification lane of this tree. Every number is a measure from that night. vLLM
ships a release about every two weeks: measure again before you trust a number
here.

See also:

- [certify-an-ai-model.md](../how-to/certify-an-ai-model.md) for how to run the lane;
- [configuration.md](configuration.md) for the `vllm` binding;
- [system-requirements.md](system-requirements.md) for the size of a GPU host.

## The short answer

On a 24 GB Apple machine, if it must be vLLM, serve **`Qwen3-14B`** (`4-bit`
MLX). Of five models it was first or tied on 6 of 7 tasks, at 10 to 13 seconds
for a verdict. It is the preset
[`qwen3_local_vllm.yaml`](../../config/presets/qwen3_local_vllm.yaml), measured
on every task (section 5): 8 of 28 certified and 1 usable with care. The Ollama
Gemma 4 preset certifies 8 of 28, and 6 more with care.

If it need not be vLLM, use **Ollama with Gemma 4 `12B`**
([ollama-self-hosting.md](ollama-self-hosting.md)). On this machine vLLM is not
faster for one user. Gemma 4 `12B` gives answers that make no sense through it (section 2),
and the structured output of Gemma 3 `12B` loops (section 4).

The Mistral models cannot replace either here. Mistral Nemo `12B` and
Ministral `8B` score below `Qwen3-14B` on every task but `enrich`, where all five
tie.

For any model you serve, start vLLM with the flags in section 1. Without them, Qwen3
and Gemma 4 think before every answer, and a short output budget comes back with
no answer at all. And the prompt of the agent loop does not fit.

## 1. Start the server

The `vllm` binding sends a plain request in the OpenAI wire format: no key, no
switches for one model. All that depends on which model you serve is the job of
the server, and belongs on its command line:

```bash
vllm serve <model> \
  --max-model-len 40960 \
  --default-chat-template-kwargs '{"enable_thinking": false}' \
  --reasoning-parser <parser> \
  --enable-prompt-tokens-details
```

| flag | why |
|---|---|
| `--max-model-len 40960` | The agent loop plans its prompt against a window of 32,768 tokens and asks for output on top. A server started with less refuses those calls with a 400 (`max_tokens=… cannot be greater than max_model_len`). 40,960 is what the Ollama adapter asks for, so the two serve the same prompts. |
| `--default-chat-template-kwargs '{"enable_thinking": false}'` | Qwen3 and Gemma 4 think by default. Measured on `Qwen3-14B` with a budget of 300 tokens and a JSON schema: 299 tokens of thinking, 29 seconds, and no answer (`content: null`, `finish_reason: length`). With this flag: the answer in 22 tokens and 2.4 seconds. A template that has no such value (`gpt-oss`, Mistral, Gemma 3) does not use it. |
| `--reasoning-parser <parser>` | `qwen3`, `gemma4`, `openai_gptoss`, … Moves any thinking out of the answer into its own field. Without it, the thinking stays in the text that the product parses as JSON. Leave it out for a model that does not think. |
| `--enable-prompt-tokens-details` | Reports cached prompt tokens, which the product records per call. Without it the count is always 0. |

The binding in the routing config then needs only the model id and, if it is not
`http://localhost:8000`, the host root (no `/v1`). The id is the name the server
answers to: what `vllm serve` was given, unless `--served-model-name` renames it.
A server on another host carries every prompt and answer over the network. Start
it with `--ssl-certfile` and `--ssl-keyfile` (or put it behind a TLS proxy), and
bind it by `https`:

```yaml
providers:
  vllm: { base_url: https://gpu-box.internal:8000 }
tiers:
  local_small: { provider: vllm, model: "mlx-community/Qwen3-14B-4bit" }
```

Two more things the server decides, and the product cannot see:

- **Sampling.** The product sends no `temperature`, so every call gets the
  defaults of the server. The server takes them from the
  `generation_config.json` of the model when the repository has one. It then logs `Default vLLM sampling parameters have been overridden`. Else it samples at `temperature` 1.0 with no
  `top_k`. Many MLX builds ship with no such file.
  - Of the ones we served, `mlx-community/Qwen3-14B-4bit`,
    `Mistral-Nemo-Instruct-2407-4bit` and `Ministral-8B-Instruct-2410-4bit` have
    none. The Gemma 3 file has no sampling values in it.
  - Qwen3 then gives far more random output than its model card asks for. Set
    the values of the card with
    `--override-generation-config '{"temperature": 0.7, "top_p": 0.8, "top_k": 20}'`
    (Qwen3 with no thinking); section 4 shows what it changes.
- **Memory.** vLLM takes `--gpu-memory-utilization` (0.92 by default) of the
  machine at start. It fills what the model files leave with the `KV` cache.
  Nothing else that needs the GPU fits next to it (a second model, an embedding
  server, a local judge) unless you lower that number.

**You cannot set `gpt-oss` thinking on the server.** A `reasoning_effort` level
sets its thinking. vLLM takes that only per request, and has no server default
for it; `enable_thinking` does not reach it. Served as above, it thinks at
`medium`: 188 tokens and 14 seconds on the request above, against 96 tokens and 3
seconds at `low`. The answers were correct both ways. `reasoning_effort: "none"`
is refused with a 400.

**Never publish the port.** The `vllm` binding sends no key, so only the hosts of
the product may reach the server: the local machine or a private network. The
`sovereign` profile checks only that `base_url` names such a host. It does not
stop other hosts from reaching the server, so the network around it must do
that. The own `--api-key` of
vLLM guards only the `/v1`, `/v2`, `/inference` and `/cohere` routes; `/metrics`
and `/health` stay open.

## 2. On a Mac: vllm-metal

vLLM runs on Apple chips through the
[vllm-metal](https://github.com/vllm-project/vllm-metal) plugin, which serves MLX
model files on the GPU:

```bash
curl -fsSL https://raw.githubusercontent.com/vllm-project/vllm-metal/main/install.sh | bash
source ~/.venv-vllm-metal/bin/activate
```

It installs vLLM next to the plugin, in its own Python `venv` (Python 3.12,
`arm64`). Serve MLX builds (`mlx-community/...-4bit`): a GGUF file from Ollama
does not load.

What we found when we served the models below with it:

| model (MLX build) | on disk | result |
|---|---|---|
| `mlx-community/Qwen3-14B-4bit` | 8.3 GB | Served the full 40,960 window. Measured in section 4 |
| `mlx-community/gpt-oss-20b-MXFP4-Q4` | 11.2 GB | Served the full window. Measured in section 4 |
| `mlx-community/Mistral-Nemo-Instruct-2407-4bit` | 6.9 GB | Served the full window. Measured in section 4 |
| `mlx-community/Ministral-8B-Instruct-2410-4bit` | 4.5 GB | **Refuses a 40,960 window.** Its config declares 32,768 token places. vLLM will not go past that unless you force it, and then the model can write text with no sense past the length it was trained on. At 32,768 it cannot carry the prompt of the agent loop and an answer. Measured at 32,768 in section 4 |
| `mlx-community/gemma-3-12b-it-4bit` | 8.0 GB | **Refuses a 40,960 window.** It needs 15 GiB of `KV` cache, and 8.5 GiB is left after the model files. The largest window that starts is about 23,000 tokens, which is below the 32,768 of the agent loop. Measured at 20,480 in section 4 |
| `mlx-community/gemma-4-12B-it-4bit` | 6.7 GB | **Serves text with no sense.** It starts and answers, and every answer is random tokens in many languages, at `temperature` 0 too. The same files through `mlx_lm` directly answer correctly (14 tokens a second). The error is in the `vllm-metal` path for the model shape of this build (`Gemma4UnifiedForConditionalGeneration`), not in the model files |

Gemma 4 `12B` also needs `--language-model-only` to start at all. Without it, vLLM
loads the part that reads images and video, and the MLX build carries no config
for that part.

On this machine vLLM reported the GPU budget Ollama did (17.8 GB fixed limit). It
started a model in 30 to 55 seconds, and filled all that the model files left
with `KV` cache (8 to 9 GiB).

## 3. How quality was measured

The same way as the Ollama page, so you can read the two side by side. The
certification lane runs the corpus written by hand through the model. It applies
the own validator of each site, and has a second model grade the answers
([certify-an-ai-model.md](../how-to/certify-an-ai-model.md)). Seven tasks, three
runs per scenario, and the judge `openai/gpt-oss-120b` in the cloud. So the
latency below is only the candidate on the GPU. The server was started with the
flags in section 1.

The runs in section 4 that compare models were written under a test `eu_hosted`
profile, and are not committed. The run of the preset itself (section 5) is
`sovereign` and committed. A certification run holds only the candidate to the
profile. The judge gets the corpus and nothing from an installation, so it may be
the default cloud one. No local judge would fit next to a vLLM server on 24 GB
(see `Memory` in section 1).

A word of care on the numbers: three runs per scenario, one machine, one
quantization. A change of about 0.15 in pass rate on 15 runs is within the noise.

## 4. Results

Pass rate, then the median time of one model call in seconds. Seven tasks, three
runs per scenario, which is the `runs` column. Every model ran at the sampling of
its model card, set with `--override-generation-config` (section 1). The one
exception is `gpt-oss`, whose repository carries its own.

| task | runs | `Qwen3-14B` | `gpt-oss-20b` | Gemma 3 `12B`¹ | Mistral Nemo `12B` | Ministral `8B`² |
|---|---|---|---|---|---|---|
| `capture_classify` | 15 | **0.80** · 13³ | 0.67 · 30 | **0.80** · 10 | 0.47 · 11 | 0.33 · 8 |
| `enrich` | 3 | **1.00** · 27 | **1.00** · 36 | **1.00** · 28 | **1.00** · 31 | **1.00** · 20 |
| `capture_counterparty_verdict` | 57 | **0.79** · 11 | 0.74 · 34 | loop⁴ | 0.53 · 12 | 0.67 · 9 |
| `cold_start` | 27 | **0.85** · 10 | 0.59 · 24 | loop⁴ | 0.52 · 13 | 0.44 · 10 |
| `corpus_ask` | 15 | **0.87** · 19 | **0.87** · 40 | `n/r` | 0.60 · 19 | 0.60 · 11 |
| `site_extract` | 15 | 0.60 · 29 | **0.73** · 34 | `n/r` | 0.40 · 21 | 0.27 · 20 |
| `summarize` | 27 | **0.41** · 41 | 0.33 · 57 | `n/r` | 0.37 · 56 | 0.30 · 45 |

¹ At a window of 20,480 tokens (section 2). `n/r` is a task not run. After the two loops below, no config was left that we could use.

² At a window of 32,768 tokens: its config declares no more.

³ With the id `enum` in section 6. At the default sampling of vLLM it scored 0.40 without the `enum` and 0.87 with it.

⁴ Never answered; see the paragraph after this list.

- **Serve `Qwen3-14B` through vLLM here.** It is first or tied on 6 of 7 tasks
  at the speed of a small model (10 to 13 seconds for a verdict). Its sampling
  made no change we could measure. At the vLLM default of `temperature` 1.0, it
  scored within 0.07 of the values of the card on every task.
- **`gpt-oss-20b` is slow for no reason.** It runs two to three times slower than
  it could, because the server cannot turn its thinking down (section 1). It wrote
  about 1,000 output tokens a call where Qwen3 wrote about 100 on the same tasks,
  and the difference is thinking. It is the best at `site_extract`.
- **No Mistral model can replace Qwen3 or Gemma.** Mistral Nemo lost 6 of its
  15 `capture_classify` runs to a `confidence` written as a share of 100 (`90`, `100`)
  where the schema wants 0 to 1
  ([#6165](https://github.com/margince/margince/issues/6165)). Ministral `8B` is
  the fastest and the weakest.
- Against the same models on Ollama ([ollama-self-hosting.md](ollama-self-hosting.md)
  section 5): `Qwen3-14B` scored within the noise of its Ollama numbers with the
  id `enum` in place. It ran at about the same speed. vLLM on a Mac is not faster
  than Ollama for one user. What it adds is the server that most GPU
  installations run.

**Gemma 3 looped in the JSON grammar.** With the default structured-output
settings of vLLM, two of its tasks never answered. The model kept writing spaces
in the JSON until the output budget of 8,192 tokens ran out. At 9 tokens a
second, that takes longer than the time limit of the client. All three retries did the
same.

Start the server with
`--structured-outputs-config.backend xgrammar --structured-outputs-config.disable_any_whitespace true`
to stop it. The flag is refused unless the backend is named, and the `auto`
default names none. It stops the loop and breaks the answer.

With it, Gemma 3
answered every `capture_counterparty_verdict` run. But 56 of 57 carried a
`confidence` outside the range of the schema (`2`, `-1`, `-5`, `23`). That is a
pass rate of 0.02 at 39 seconds a call. With spaces not allowed, the model cannot write the
tokens it would write, and the number is where that shows. Through `vllm-metal`,
the structured output of Gemma 3 is not usable either way.

## 5. The preset, certified

[`config/presets/qwen3_local_vllm.yaml`](../../config/presets/qwen3_local_vllm.yaml)
binds every tier to `Qwen3-14B` on vLLM, and embeddings to `bge-m3` on a second
vLLM, under `sovereign`. Its header carries the two `vllm serve` command lines.
With both servers up (0.87 and 0.1 of memory), about 9% of the machine is left
free. We ran every task we ship that this text-only binding can carry (all but
`document_extract`): three runs per scenario, judged by the default cloud judge.
The records are committed, and the page that shows what is ready reads them.

| task | verdict | pass rate | runs | median (seconds) | `p95` (seconds) | output tokens |
|---|---|---|---|---|---|---|
| `account_scan` | certified | 1.00 | 12 | 19.7 | 29.2 | 115 |
| `agent_loop` | certified | 1.00 | 18 | 6.2 | 29.0 | 18 |
| `brief_ranking` | certified | 1.00 | 9 | 8.1 | 12.9 | 71 |
| `cert_judge` | certified | 1.00 | 12 | 9.6 | 12.3 | 42 |
| `enrich` | certified | 1.00 | 12 | 49.3 | 154.6 | 297 |
| `rate_extract` | certified | 1.00 | 9 | 8.4 | 23.1 | 109 |
| `site_triage` | certified | 1.00 | 18 | 8.8 | 49.5 | 39 |
| `transcript_propose` | certified | 1.00 | 9 | 9.9 | 23.3 | 29 |
| `corpus_ask` | `supported, degraded` | 0.85 | 33 | 22.7 | 54.6 | 111 |
| `deal_health` | not supported | 1.00 | 9 | 90.8 | 117.8 | 387 |
| `weekly_review` | not supported | 1.00 | 30 | 30.4 | 44.2 | 43 |
| `draft_reply` | not supported | 0.95 | 63 | 46.3 | 189.9 | 149 |
| `cold_start` | not supported | 0.88 | 72 | 12.3 | 91.3 | 99 |
| `capture_confidentiality_verdict` | not supported | 0.86 | 42 | 10.4 | 11.6 | 70 |
| `offer_draft` | not supported | 0.83 | 18 | 15.6 | 86.1 | 173 |
| `voice_build` | not supported | 0.83 | 18 | 28.3 | 108.6 | 128 |
| `capture_classify` | not supported | 0.80 | 15 | 12.9 | 27.8 | 101 |
| `capture_counterparty_verdict` | not supported | 0.77 | 123 | 12.7 | 37.4 | 71 |
| `site_extract` | not supported | 0.76 | 45 | 30.4 | 82.1 | 233 |
| `site_fact_extract` | not supported | 0.67 | 9 | 48.3 | 67.8 | 197 |
| `stage_evidence_extract` | not supported | 0.67 | 30 | 19.0 | 31.9 | 116 |
| `weekly_learnings` | not supported | 0.60 | 15 | 36.4 | 66.4 | 226 |
| `summarize` | not supported | 0.54 | 57 | 64.7 | 119.0 | 405 |
| `propose_roles` | not supported | 0.54 | 24 | 26.3 | 81.6 | 164 |
| `request_settlement` | not supported | 0.50 | 12 | 21.5 | 57.6 | 140 |
| `signal_extract` | not supported | 0.50 | 12 | 14.2 | 40.4 | 69 |
| `owed_verdict` | not supported | 0.47 | 15 | 17.4 | 26.7 | 136 |
| `growth_fit` | not supported | 0.00 | 12 | 119.6 | 195.2 | 886 |

8 certified, 1 supported with care, 19 not supported, of 28 scored; the median
call took 19.3 seconds. We ran these records again on 2026-09-28/29 under the
grade rule of that time. That rule runs a scenario close to the bar again before
it decides it, so the run count of a row is not fixed.

The machine was short of memory for part of that night. So read the latency
columns as an upper limit, and measure serving speed with a direct call.
`document_extract` has no record: it sends a PDF, which this text-only binding
does not carry. A pass rate of 1.00 next to "not supported" (`deal_health`,
`weekly_review`) is the grade rule at work. Every run passed the own validator of
the site, and the judge still scored a scenario below its bar.

## 6. What the product does for vLLM

This run found two errors, and both are now fixed:

**A batch of verdicts names only sent ids.** Five sites ask the model to
answer per message by its id of 36 signs. `Qwen3-14B` often copied the first
half of a real id and made up the rest, and the site then refused the whole
batch. Each of those sites now sends a response schema whose `id` is an `enum` of
the ids in that call. So a model whose output must follow the grammar cannot write any other id.
Same model, server and sampling, without and with the `enum`: `capture_classify`
moved from 0.40 (8 of 15 runs refused for `result id was not requested`) to 0.87
(none) ([#6111](https://github.com/margince/margince/issues/6111)).

**The onboarding conversation takes turns.** The three onboarding sites send the
context (in a code block) and the message of the admin as one user turn. The chat templates
of the Mistral and Gemma models refuse two user turns in a row
(`conversation roles must alternate`). vLLM uses the own template of the model.
So split turns made Mistral Nemo answer every `cold_start` call with a 400. A gate
holds every request in the certification corpus to roles that take turns.

## 7. What we did not test

- **vLLM on an NVIDIA GPU**, which is where most vLLM installations run. The wire
  findings in section 1 (thinking defaults, the error shape, `max_model_len`)
  belong to vLLM and hold there. The speeds and the two Gemma failures in section
  2 belong to `vllm-metal` on this Mac, and may not.
- **Gemma 4 at all**, because of the `vllm-metal` error above.
- **The rest of the corpus for other models.** They ran 7 tasks; only the
  model of the preset ran all of them.
- **Embedding quality.** The second server of the preset served `bge-m3` during
  the run, but no task measured retrieval quality. The `vllm-metal` plugin lists
  `BAAI/bge-m3` under early pooling support, started with `--runner pooling`.
- **Many calls at once.** One call at a time. Running many calls in one batch is
  the reason vLLM exists, and we have not tested it here.
- **Thinking turned on**, for any model.
