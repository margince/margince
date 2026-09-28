# Self-hosting models with vLLM: what we measured

**Tested 2026-09-24** on the same machine as
[ollama-self-hosting.md](ollama-self-hosting.md) (Mac mini M4, 24 GB unified
memory, about 17.8 GiB of it usable by the GPU), with **vLLM 0.30.0** through
the Apple-silicon plugin **vllm-metal 0.30.0.dev20260924**, through this tree's
own certification lane. Every figure is a measurement from that night. vLLM
releases about every two weeks: re-measure before trusting a number here.

Related: [certify-an-ai-model.md](../how-to/certify-an-ai-model.md) for how to
run the lane, [configuration.md](configuration.md) for the `vllm` binding, and
[system-requirements.md](system-requirements.md) for sizing a GPU host.

## The short answer

On a 24 GB Apple-silicon machine, **serve `Qwen3-14B` (4-bit MLX) if it must be
vLLM**: of five models it led or tied on six of seven tasks, at 10 to 13
seconds for a verdict. It is the preset
[`qwen3_local_vllm.yaml`](../../config/presets/qwen3_local_vllm.yaml), measured
on every task (section 5): 6 of 28 certified and 2 usable with care, where the
Ollama Gemma 4 preset certifies 6 of the 12 it was measured on. **If it need not be vLLM, use Ollama with Gemma 4 12B**
([ollama-self-hosting.md](ollama-self-hosting.md)): on this machine vLLM is not
faster for one user, Gemma 4 12B answers gibberish through it (section 2), and
Gemma 3 12B's structured output loops (section 4).

The Mistral models are not a replacement for either here: Mistral Nemo 12B and
Ministral 8B trail Qwen3-14B on every task but `enrich`, where all five tie.

Whatever you serve, **start vLLM with the flags in section 1**. Without them
Qwen3 and Gemma 4 think before every answer, a short output budget comes back
with no answer at all, and the agent loop's prompt does not fit.

## 1. Start the server

The `vllm` binding sends a plain OpenAI-wire request: no key, no
model-specific switches. Everything that depends on WHICH model you serve is
the server's job, and belongs on its command line:

```bash
vllm serve <model> \
  --max-model-len 40960 \
  --default-chat-template-kwargs '{"enable_thinking": false}' \
  --reasoning-parser <parser> \
  --enable-prompt-tokens-details
```

| flag | why |
|---|---|
| `--max-model-len 40960` | The agent loop budgets its prompt against a 32,768-token window and asks for output on top. A server started with less refuses those calls with a 400 (`max_tokens=… cannot be greater than max_model_len`). 40,960 is what the Ollama adapter asks for, so the two serve the same prompts. |
| `--default-chat-template-kwargs '{"enable_thinking": false}'` | Qwen3 and Gemma 4 think by default. Measured on Qwen3-14B with a 300-token budget and a JSON schema: 299 tokens of thinking, 29 s, **no answer** (`content: null`, `finish_reason: length`). With this flag: the answer in 22 tokens and 2.4 s. A template that has no such variable (gpt-oss, Mistral, Gemma 3) ignores it. |
| `--reasoning-parser <parser>` | `qwen3`, `gemma4`, `openai_gptoss`, … Separates any thinking from the answer into its own field instead of leaving it in the text the product parses as JSON. Omit it for a model that does not think. |
| `--enable-prompt-tokens-details` | Reports cached prompt tokens, which the product records per call. Without it the count is always 0. |

The binding in the routing config then needs only the model id and, if it is
not `http://localhost:8000`, the host root (no `/v1`). The id is the name the
server answers to: what `vllm serve` was given, unless `--served-model-name`
renames it. A server on another host carries every prompt and answer across
the network, so start it with `--ssl-certfile` and `--ssl-keyfile` (or put it
behind a TLS proxy) and bind it by `https`:

```yaml
local_small: { provider: vllm, model: "mlx-community/Qwen3-14B-4bit", base_url: https://gpu-box.internal:8000 }
```

Two more things the server decides, and the product cannot see:

- **Sampling.** The product sends no temperature, so the server's defaults are
  what every call gets. vLLM takes them from the model's `generation_config.json`
  when the repository has one (it logs "Default vLLM sampling parameters have
  been overridden"), and otherwise samples at temperature 1.0 with no top-k.
  Many MLX conversions ship without that file: of the ones we served,
  `mlx-community/Qwen3-14B-4bit`, `Mistral-Nemo-Instruct-2407-4bit` and
  `Ministral-8B-Instruct-2410-4bit` have none (Gemma 3's has one, with no
  sampling values in it), and Qwen3 then runs far hotter
  than its model card recommends. Set the card's values yourself with
  `--override-generation-config '{"temperature": 0.7, "top_p": 0.8, "top_k": 20}'`
  (Qwen3 without thinking); section 4 shows what it changes.
- **Memory.** vLLM takes `--gpu-memory-utilization` (0.92 by default) of the
  machine up front and fills what the weights leave with KV cache. Nothing else
  that needs the GPU (a second model, an embedding server, a local judge) fits
  beside it unless you lower that number.

**gpt-oss cannot be quieted from the server.** Its thinking is set by a
`reasoning_effort` level, which vLLM takes only per request, and has no server
default for; `enable_thinking` does not reach it. Served as above it thinks at
`medium`: 188 tokens and 14 s on the request above, against 96 tokens and 3 s at
`low`. The answers were correct either way. `reasoning_effort: "none"` is
refused with a 400.

**Never publish the port.** The `vllm` binding sends no key, so the server must
be reachable only from the product's hosts: loopback or a private network. That
is also what the `sovereign` profile enforces on `base_url`. vLLM's own
`--api-key` guards only the `/v1`, `/v2`, `/inference` and `/cohere` routes;
`/metrics` and `/health` stay open.

## 2. On a Mac: vllm-metal

vLLM runs on Apple silicon through the
[vllm-metal](https://github.com/vllm-project/vllm-metal) plugin, which serves
MLX weights on the GPU:

```bash
curl -fsSL https://raw.githubusercontent.com/vllm-project/vllm-metal/main/install.sh | bash
source ~/.venv-vllm-metal/bin/activate
```

It installs vLLM beside the plugin in its own virtual environment (Python
3.12, arm64). Serve MLX builds (`mlx-community/...-4bit`): a GGUF file from
Ollama does not load.

What we found serving the models below with it:

| model (MLX build) | on disk | outcome |
|---|---|---|
| `mlx-community/Qwen3-14B-4bit` | 8.3 GB | Served the full 40,960 window. Measured in section 4 |
| `mlx-community/gpt-oss-20b-MXFP4-Q4` | 11.2 GB | Served the full window. Measured in section 4 |
| `mlx-community/Mistral-Nemo-Instruct-2407-4bit` | 6.9 GB | Served the full window. Measured in section 4 |
| `mlx-community/Ministral-8B-Instruct-2410-4bit` | 4.5 GB | **Refuses a 40,960 window**: its configuration declares 32,768 positions, and vLLM will not exceed that without an override that risks garbage past the trained length. At 32,768 it cannot carry the agent loop's prompt and an answer. Measured at 32,768 in section 4 |
| `mlx-community/gemma-3-12b-it-4bit` | 8.0 GB | **Refuses a 40,960 window**: it needs 15 GiB of KV cache and 8.5 GiB is left after the weights. The largest window that starts is about 23,000 tokens, which is below the agent loop's 32,768. Measured at 20,480 in section 4 |
| `mlx-community/gemma-4-12B-it-4bit` | 6.7 GB | **Serves gibberish.** Starts, answers, and every answer is random multilingual tokens, at temperature 0 as well. The same files through `mlx_lm` directly answer correctly (14 tokens a second), so the fault is in vllm-metal's path for this checkpoint's architecture (`Gemma4UnifiedForConditionalGeneration`), not in the weights |

Gemma 4 12B also needs `--language-model-only` to start at all: without it
vLLM loads the multimodal processor, and the MLX build carries no video
processor configuration.

On this machine vLLM reported the GPU budget Ollama did (17.8 GB wired limit),
started a model in 30 to 55 seconds, and filled everything the weights left
with KV cache (8 to 9 GiB).

## 3. How quality was measured

The same way as the Ollama page, so the two can be read side by side: the
certification lane runs the hand-written corpus through the model, applies each
site's own validator, and has a second model grade the answers
([certify-an-ai-model.md](../how-to/certify-an-ai-model.md)). Seven tasks, three
runs per scenario, the judge `openai/gpt-oss-120b` in the cloud, so the latency
below is the candidate alone on the GPU. The server was started with the flags
in section 1.

The comparison runs in section 4 were written under a scratch `eu_hosted`
profile and are **not committed**. The preset's own run (section 5) is
`sovereign` and committed: a certification run holds only the candidate to the
profile, and the judge, which is sent the corpus and nothing of an
installation's, may be the default cloud one. No local judge would fit beside a
vLLM server on 24 GB (see "Memory" in section 1).

Statistics warning: three runs per scenario, one machine, one quantization. A
difference of about 0.15 in pass rate on 15 runs is inside the noise.

## 4. Results

Pass rate, then the median time of one model call in seconds. Seven tasks, three
runs per scenario, which is the `runs` column. Every model
ran at its model card's sampling, set with `--override-generation-config`
(section 1), except gpt-oss, whose repository carries its own.

| task | runs | Qwen3-14B | gpt-oss-20b | Gemma 3 12B¹ | Mistral Nemo 12B | Ministral 8B² |
|---|---|---|---|---|---|---|
| capture_classify | 15 | **0.80** · 13³ | 0.67 · 30 | **0.80** · 10 | 0.47 · 11 | 0.33 · 8 |
| enrich | 3 | **1.00** · 27 | **1.00** · 36 | **1.00** · 28 | **1.00** · 31 | **1.00** · 20 |
| capture_counterparty_verdict | 57 | **0.79** · 11 | 0.74 · 34 | loops⁴ | 0.53 · 12 | 0.67 · 9 |
| cold_start | 27 | **0.85** · 10 | 0.59 · 24 | loops⁴ | 0.52 · 13 | 0.44 · 10 |
| corpus_ask | 15 | **0.87** · 19 | **0.87** · 40 | — | 0.60 · 19 | 0.60 · 11 |
| site_extract | 15 | 0.60 · 29 | **0.73** · 34 | — | 0.40 · 21 | 0.27 · 20 |
| summarize | 27 | **0.41** · 41 | 0.33 · 57 | — | 0.37 · 56 | 0.30 · 45 |

¹ At a 20,480-token window (section 2). A dash is a task not run: after the two loops below there was no configuration left worth measuring.
² At a 32,768-token window: its configuration declares no more.
³ With the id fix in section 6. Before it, at vLLM's default sampling, 0.40; after it, at the same sampling, 0.87.
⁴ Never answered; see the paragraph after this list.

- **Qwen3-14B is the model to serve through vLLM here.** It leads or ties on six
  of seven tasks at the speed of a small model (10 to 13 seconds for a verdict).
  Its sampling made no measurable difference: at vLLM's default of temperature
  1.0 it scored within 0.07 of the card's values on every task.
- **gpt-oss-20b is two to three times slower than it needs to be**, because the
  server cannot turn its thinking down (section 1): it wrote about 1,000 output
  tokens a call where Qwen3 wrote about 100 on the same tasks, and the
  difference is thinking. It is the best at `site_extract`.
- **Neither Mistral model is a Qwen3 or Gemma replacement on this machine.**
  Mistral Nemo lost six of its fifteen `capture_classify` runs to a confidence
  written as a percentage (`90`, `100`) where the schema wants 0 to 1
  ([#6165](https://github.com/margince/margince/issues/6165)); Ministral 8B is
  the fastest and the weakest.
- Against the same models on Ollama ([ollama-self-hosting.md](ollama-self-hosting.md)
  section 5): Qwen3-14B scored within noise of its Ollama figures once the id fix
  was in, and at a similar speed. vLLM on a Mac is not faster than Ollama for one
  user; what it adds is the server most GPU deployments run.

**Gemma 3 looped inside the JSON grammar.** With vLLM's default structured-output
settings, two of its tasks never answered: the model kept writing whitespace
inside the JSON until the 8,192-token output budget ran out, which at 9 tokens a
second outlasts the client's timeout, and all three retries did the same. Start
the server with
`--structured-outputs-config.backend xgrammar --structured-outputs-config.disable_any_whitespace true`
to forbid it; the flag is refused unless the backend is named, and the `auto`
default names none. It stops the loop and breaks the answer: with it,
Gemma 3 answered every `capture_counterparty_verdict` run, and 56 of 57 carried a
confidence the schema's own range excludes (`2`, `-1`, `-5`, `23`), for a pass
rate of 0.02 at 39 seconds a call. Forbidding whitespace forces the model off the
tokens it would write, and the number is where that shows. Through vllm-metal,
Gemma 3's structured output is not usable either way.

## 5. The preset, certified

[`config/presets/qwen3_local_vllm.yaml`](../../config/presets/qwen3_local_vllm.yaml)
binds every tier to Qwen3-14B on vLLM and embeddings to `bge-m3` on a second vLLM,
under `sovereign`. Its header carries the two `vllm serve` command lines; with
both servers up (0.87 and 0.1 of memory) about 9% of the machine is left free.
Every shipped task this text-only binding can carry was run through it (all but
`document_extract`), three runs per scenario, judged by the default cloud judge. The records are committed and the readiness page reads them.

| task | verdict | pass rate | runs | median s | 95th pct s | output tokens |
|---|---|---|---|---|---|---|
| agent_loop | certified | 1.00 | 18 | 6.1 | 29.5 | 18 |
| brief_ranking | certified | 1.00 | 3 | 9.4 | 9.9 | 73 |
| cert_judge | certified | 1.00 | 6 | 6.7 | 7.1 | 35 |
| enrich | certified | 1.00 | 3 | 25.2 | 26.6 | 227 |
| rate_extract | certified | 1.00 | 9 | 10.3 | 22.9 | 113 |
| transcript_propose | certified | 1.00 | 9 | 9.8 | 16.4 | 29 |
| weekly_review | supported, degraded | 0.92 | 12 | 6.7 | 11.6 | 39 |
| capture_classify | supported, degraded | 0.80 | 15 | 10.5 | 25.6 | 102 |
| account_scan | not supported | 1.00 | 6 | 18.7 | 27.4 | 118 |
| draft_reply | not supported | 0.92 | 36 | 14.0 | 61.9 | 164 |
| deal_health | not supported | 0.89 | 9 | 41.4 | 52.7 | 369 |
| corpus_ask | not supported | 0.87 | 15 | 19.1 | 50.2 | 92 |
| site_triage | not supported | 0.87 | 15 | 6.2 | 10.6 | 39 |
| capture_confidentiality_verdict | not supported | 0.86 | 42 | 10.8 | 13.8 | 71 |
| cold_start | not supported | 0.85 | 27 | 9.8 | 44.0 | 110 |
| capture_counterparty_verdict | not supported | 0.81 | 57 | 10.8 | 15.0 | 71 |
| voice_build | not supported | 0.75 | 12 | 14.9 | 92.2 | 221 |
| site_fact_extract | not supported | 0.67 | 9 | 20.5 | 32.7 | 173 |
| offer_draft | not supported | 0.60 | 15 | 12.4 | 45.9 | 167 |
| site_extract | not supported | 0.60 | 15 | 27.4 | 35.2 | 225 |
| propose_roles | not supported | 0.56 | 9 | 20.8 | 50.1 | 190 |
| owed_verdict | not supported | 0.50 | 12 | 16.3 | 26.5 | 143 |
| request_settlement | not supported | 0.50 | 12 | 21.2 | 24.5 | 140 |
| signal_extract | not supported | 0.50 | 12 | 12.1 | 16.5 | 66 |
| summarize | not supported | 0.48 | 27 | 40.0 | 98.2 | 437 |
| stage_evidence_extract | not supported | 0.33 | 27 | 17.5 | 34.2 | 96 |
| weekly_learnings | not supported | 0.22 | 9 | 33.9 | 44.7 | 338 |
| growth_fit | not supported | 0.00 | 3 | 88.0 | 105.7 | 888 |

6 certified, 2 supported with care, 20 not supported, of 28 scored; the median call
took 14.9 s. `document_extract` has no record: it sends a PDF, which this
text-only binding does not carry. A pass rate of 1.00 beside "not supported"
(`account_scan`) is the grading rule at work: every run passed the site's own
validator, and the judge still scored a scenario below its bar.

## 6. What we changed in the product

Two defects no other runtime had shown, both fixed in the same change as this
page:

**A batched verdict could name an id it was never sent.** Five sites ask the
model to answer per message by its 36-character id. Qwen3-14B often copied the
first half of a real id and invented the rest; the site then refused the whole
batch. Each of those sites now sends a response schema whose `id` is an enum of
the ids in that call, so a grammar-constrained decoder cannot write any other
one. Same model, server and sampling, before and after: `capture_classify` went
from 0.40 (8 of 15 runs refused for "result id was not requested") to 0.87 (none)
([#6111](https://github.com/margince/margince/issues/6111)).

**The onboarding conversation sent two user turns in a row.** The three
onboarding sites opened with the fenced context as one user turn and then the
administrator's message as another. The chat templates of Mistral's and Gemma's
instruction models refuse that ("conversation roles must alternate"), and vLLM
renders the model's own template, so Mistral Nemo answered every `cold_start`
call with a 400. The turns are now joined, and a gate holds every request in the
certification corpus to alternating roles.

## 7. What we did not test

- **vLLM on an NVIDIA GPU**, which is where most vLLM deployments run. The wire
  findings in section 1 (thinking defaults, the error shape, `max_model_len`)
  are properties of vLLM and hold there; the speeds and the two Gemma failures
  in section 2 are properties of vllm-metal on this Mac and may not.
- **Gemma 4 at all**, because of the vllm-metal fault above.
- **The rest of the corpus for the other models**: they ran seven tasks; only
  the preset's model ran all of them.
- **Embeddings.** vllm-metal lists `BAAI/bge-m3` under experimental pooling
  support; a second server with `--runner pooling` would serve it. Not run.
- **Concurrency.** One call at a time. Continuous batching is vLLM's reason to
  exist, and it is untested here.
- **Thinking turned on**, for any model.
