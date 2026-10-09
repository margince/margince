<!-- prose:plain -->
# Self-hosting models with Ollama: what we measured

**A dated measurement, from 2026-09-23 to 2026-09-24.** It records what one
machine (below) did with Ollama 0.34.3, through the certification lane of this
tree. Every number comes from these two days, except the verdicts in section 4,
which were run again on 2026-09-28/29. Models, Ollama and this product all
change: measure again before you trust a number here to decide what to buy.

The current facts live elsewhere:

- [ai-certification.md](../reference/ai-certification.md): the committed report on what is ready today;
- [enrich-with-a-local-llm.md](../how-to/enrich-with-a-local-llm.md): the steps to run Ollama and bind a stack to it;
- [certify-an-ai-model.md](../how-to/certify-an-ai-model.md): how to run the lane;
- [`config/presets/gemma4_local_ollama.yaml`](../../config/presets/gemma4_local_ollama.yaml):
  the binding this page asks you to use.

The same machine serving models through vLLM is
[vllm-self-hosting.md](vllm-self-hosting.md).

## The short answer

On a 24GB Apple machine with an `M4` chip, use **`gemma4:12b`** on every tier.

- It is the only model we tested that passed every `capture_classify` run (15 of
  15), and the only one we ran on the whole corpus. It had 8 tasks at a 100% pass
  rate and 8 more at 80% or better, out of 28 scored (table in section 4). Under
  the current grade rule it certifies 8 of 28, and 6 more are usable with care.
- A call takes about 14 seconds (the median of the medians per task in section
  4). It generates 12 tokens a second.
- It is the largest Gemma 4 that runs in full on the GPU here. Part of the `26B`
  runs on the CPU, and is slower with no better results.
- It does badly on tasks that ask a small model to return the right id of 36
  signs, and on tasks that write long text. Section 6 lists
  why.

Two model behaviors cause failures before the quality of the answer counts, and
the adapter handles both. Gemma 4 thinks by default: 10 to 20 times slower, and
an empty answer when the output budget runs out. And `gpt-oss` breaks if you tell
it not to think. Section 6 has both.

## 1. The machine

| | |
|---|---|
| Machine | `Mac mini` (`Mac16,10`) |
| Chip | Apple `M4`, 10 CPU cores (4 fast, 6 low power), 10 GPU cores |
| Memory | 24 GB, shared by CPU and GPU |
| Memory the GPU can use | **17.8 GiB**, as the Ollama server log reports it. This number, not the 24, decides what fits |
| OS | macOS 26.5 |
| Ollama | 0.34.3 (0.33.1 for the first hours) |
| Quantized as | `Q4_K_M` for every model except `gpt-oss` (`MXFP4`, as shipped) |
| Runner window | 12,288 tokens for most calls (the adapter sizes it per request, up to 40,960) |

Nothing else ran on the machine but the tools that ran the test. We did not
measure whether the chip slowed itself down over the hours to keep safe; the
machine has a fan.

## 2. The models

Speed is the generation speed from one direct call to Ollama. The call had a
prompt of about 480 tokens, a JSON schema, thinking off (`gpt-oss` cannot turn
it off; see section 6), and nothing else loaded. This is the raw generation speed. The latency in the
certification records is slower, for a reason section 6 gives.

| model tag | on disk | in memory | where it ran | tokens per second | verdict for this machine |
|---|---|---|---|---|---|
| `gemma4:12b` | 7.6 GB | 8.3 GB | 100% GPU | 12 | **Use this.** |
| `gemma4:e4b` | 9.6 GB | 9.6 GB | 100% GPU | 27.5 | Fast, and too weak: 5 of 15 runs passed on the easiest classify task |
| `gemma4:26b` (MoE, about `4B` active) | 18 GB | not measured | 65% CPU / 35% GPU | 10 | Does not fit. Needs a machine with more GPU memory; we did not test one |
| `gpt-oss:20b` (MoE) | 13 GB | 12 GB | 100% GPU | 23 | The fastest good model. Very good at answers from given text; weak at strict classify tasks |
| `qwen3:14b` | 9.3 GB | not measured | fit | not measured | Close to Gemma 4 `12B`, and slower per call (median 18 seconds) |
| `qwen3.5:9b` | 6.6 GB | not measured | fit | not measured | Below or level with Gemma 4 `12B` on every task we ran |
| `lfm2:24b-a2b` | 14 GB | not measured | fit | not measured | **Do not use.** 0 of 15 on the classify task |
| `qwen3.8:27b` | 17 GB | 18 GB | 22% CPU / 78% GPU | not measured | Does not fit. Median call 104 seconds; the `summarize` task ran 101 minutes and failed |

We took this list from a public
[ranking of models for a 24GB `Mac mini M4`](https://modelfit.io/blog/best-llm-mac-mini-m4-24gb/),
plus Gemma 4. Its speed guesses were right: it said 12 to 18 tokens a second for
Gemma 4 `12B` (we measured 12) and 18 to 24 for `gpt-oss` `20B` (we measured 23).

Size on disk is what `ollama list` prints. Memory is `ollama ps` with the model
loaded at our window. Its CPU/GPU column reads `65%/35% CPU/GPU` as 65% on the
CPU. We give no memory number for `gemma4:26b` and `qwen3.8:27b`, because
`ollama ps` reported only the GPU part. One reading of `e4b` on the earlier
Ollama (0.33) was 3.3 GB; we did not find why the two are different.

## 3. How quality was measured

The certification lane runs a corpus written by hand through the model. It
applies the own validator of the site, and has a second model grade the answers. Three
runs per scenario. See [certify-an-ai-model.md](../how-to/certify-an-ai-model.md).

This page has two different measures, and you cannot compare them:

- **Pass rate** (sections 4 and 5): the share of runs the own validator of the
  site accepted. We took these hours *before* the certification grade rule
  changed, with a cloud judge (`openai/gpt-oss-120b` through OpenRouter). The
  validator half of this number does not depend on the judge, so it still
  describes the model. The labels from that older rule, which required every
  run to pass, are not shown.
- **Verdict** (section 4, last table): the labels the current rule gives, from
  the 28 records committed for the `sovereign` preset. We ran them again on
  2026-09-28/29 and judged by the default cloud judge (Claude Sonnet 4.6).

The two are different runs with different judges, so one task can score
differently in each. `request_settlement` is 0.25 in the first table and 0.96 in the second.

A word of care on the numbers: three runs per scenario, one machine, one quantization.
A change of about 0.15 in pass rate on 15 runs is within the noise. Read the order,
not the second number after the point.

## 4. Gemma 4 `12B` on the whole corpus

28 of the 29 tasks we ship were scored. `document_extract` has no result: it sends
a PDF, and the chat wire of Ollama carries images only. So it fails by design under
any Ollama binding. Latency is per model call, in seconds, with only the model on
the GPU (the judge was in the cloud). Sorted by pass rate.

| task | pass rate | runs | median | `p95` | output tokens |
|---|---|---|---|---|---|
| `brief_ranking` | 1.00 | 3 | 8.9 | 14.2 | 77 |
| `capture_classify` | 1.00 | 15 | 11.3 | 25.0 | 103 |
| `cert_judge` | 1.00 | 6 | 6.3 | 7.9 | 32 |
| `deal_health` | 1.00 | 9 | 45.5 | 66.1 | 469 |
| `enrich` | 1.00 | 3 | 26.3 | 29.1 | 253 |
| `propose_roles` | 1.00 | 9 | 17.4 | 43.4 | 154 |
| `rate_extract` | 1.00 | 9 | 14.1 | 17.2 | 116 |
| `transcript_propose` | 1.00 | 9 | 10.8 | 17.6 | 36 |
| `cold_start` | 0.96 | 27 | 9.9 | 44.1 | 104 |
| `draft_reply` | 0.92 | 36 | 12.9 | 36.8 | 102 |
| `site_fact_extract` | 0.89 | 9 | 21.3 | 309.4* | 143 |
| `corpus_ask` | 0.87 | 15 | 17.6 | 51.1 | 133 |
| `site_triage` | 0.87 | 15 | 7.2 | 9.8 | 36 |
| `capture_confidentiality_verdict` | 0.86 | 42 | 10.5 | 11.3 | 77 |
| `voice_build` | 0.83 | 12 | 13.7 | 97.5 | 260 |
| `offer_draft` | 0.80 | 15 | 15.0 | 47.8 | 188 |
| `weekly_review` | 0.75 | 12 | 11.0 | 22.3 | 81 |
| `capture_counterparty_verdict` | 0.74 | 57 | 9.9 | 12.0 | 74 |
| `agent_loop` | 0.69 | 72 | 7.8 | 16.8 | 45 |
| `weekly_learnings` | 0.67 | 9 | 34.6 | 52.5 | 319 |
| `site_extract` | 0.60 | 15 | 23.3 | 33.2 | 190 |
| `summarize` | 0.52 | 27 | 55.0 | 83.2 | 471 |
| `owed_verdict` | 0.50 | 12 | 18.0 | 26.4 | 157 |
| `signal_extract` | 0.50 | 12 | 14.4 | 16.1 | 78 |
| `account_scan` | 0.50 | 6 | 23.5 | 30.9 | 116 |
| `stage_evidence_extract` | 0.44 | 27 | 12.0 | 31.5 | 84 |
| `growth_fit` | 0.33 | 3 | 86.0 | 90.5 | 894 |
| `request_settlement` | 0.25 | 12 | 25.6 | 35.3 | 173 |

\* One call reached an Ollama runner that stopped answering, and it took 309 seconds (section 6).

What the latency column means for a user: a short verdict or class is 6 to 11
seconds. Extract tasks are 12 to 26. A task that writes hundreds of tokens
(`summarize`, `deal_health`, `growth_fit`) is 45 to 86 seconds. Generation is 12
tokens a second, and nothing gets past that on this chip.

The table below has the verdicts the current grade rule gives, from the committed
`sovereign` records (2026-09-28/29, cloud judge). A scenario close to the bar is run
again before it is decided, so the run counts are not all the same. Latency here includes the turn
of the judge off the GPU, but not a second model on it; read it next to the table
above.

| task | verdict | pass rate | runs | median (seconds) | `p95` (seconds) | output tokens |
|---|---|---|---|---|---|---|
| `agent_loop` | certified | 1.00 | 18 | 5.9 | 36.7 | 19 |
| `capture_classify` | certified | 1.00 | 18 | 11.4 | 28.5 | 131 |
| `cert_judge` | certified | 1.00 | 12 | 9.3 | 12.2 | 33 |
| `enrich` | certified | 1.00 | 12 | 29.8 | 36.4 | 307 |
| `propose_roles` | certified | 1.00 | 9 | 14.8 | 40.3 | 154 |
| `rate_extract` | certified | 1.00 | 9 | 9.5 | 17.2 | 96 |
| `transcript_propose` | certified | 1.00 | 9 | 11.4 | 21.5 | 36 |
| `request_settlement` | certified | 0.96 | 24 | 23.8 | 26.9 | 160 |
| `deal_health` | `supported, degraded` | 1.00 | 15 | 44.3 | 64.1 | 479 |
| `weekly_review` | `supported, degraded` | 0.97 | 36 | 7.4 | 16.0 | 48 |
| `site_triage` | `supported, degraded` | 0.95 | 21 | 6.7 | 7.3 | 36 |
| `offer_draft` | `supported, degraded` | 0.88 | 24 | 25.4 | 44.9 | 240 |
| `account_scan` | `supported, degraded` | 0.83 | 18 | 23.3 | 33.1 | 123 |
| `brief_ranking` | `supported, degraded` | 0.83 | 6 | 8.7 | 12.5 | 77 |
| `draft_reply` | not supported | 0.91 | 108 | 9.1 | 40.3 | 85 |
| `capture_confidentiality_verdict` | not supported | 0.86 | 42 | 10.6 | 11.4 | 77 |
| `cold_start` | not supported | 0.85 | 39 | 13.9 | 44.3 | 113 |
| `corpus_ask` | not supported | 0.80 | 30 | 19.7 | 48.1 | 142 |
| `capture_counterparty_verdict` | not supported | 0.74 | 72 | 10.0 | 12.7 | 73 |
| `stage_evidence_extract` | not supported | 0.72 | 36 | 21.5 | 33.6 | 107 |
| `voice_build` | not supported | 0.71 | 24 | 14.0 | 111.6 | 397 |
| `site_fact_extract` | not supported | 0.67 | 18 | 15.0 | 24.9 | 136 |
| `weekly_learnings` | not supported | 0.67 | 27 | 5.1 | 5.4 | 11 |
| `summarize` | not supported | 0.62 | 66 | 45.4 | 78.3 | 436 |
| `site_extract` | not supported | 0.60 | 45 | 22.0 | 31.1 | 193 |
| `growth_fit` | not supported | 0.56 | 18 | 109.5 | 123.5 | 1131 |
| `signal_extract` | not supported | 0.56 | 18 | 12.1 | 14.6 | 65 |
| `owed_verdict` | not supported | 0.50 | 12 | 16.1 | 26.1 | 156 |

8 certified, 6 usable with care, 14 not supported, of 28 scored. `document_extract`
has no record, for the reason above. A pass rate of 1.00 next to "usable with
care" (`deal_health`) is the grade rule at work. Every run passed the own validator
of the site, and the judge scored a scenario below its bar.

## 5. The other models: a first look, not a certification

Five tasks each, three runs, the same cloud judge (`gpt-oss:20b` was judged by
`mistral-medium-3.1`, so it did not grade a model from its own maker). Pass rate, then median
call time in seconds. `n/r` means not run.

| task | `gemma4:12b` | `gpt-oss:20b` | `qwen3:14b` | `qwen3.5:9b` | `lfm2:24b-a2b` | `qwen3.8:27b` |
|---|---|---|---|---|---|---|
| `capture_classify` | **1.00** · 11 | 0.53 · 5 | 0.73 · 10 | 0.40 · 8 | 0.00 · 2 | 0.80 · 80 |
| `enrich` | **1.00** · 26 | **1.00** · 14 | **1.00** · 24 | **1.00** · 18 | 1.00, but the judge scored it 60 · 7 | **1.00** · 128 |
| `summarize` | 0.52 · 55 | 0.52 · 18 | 0.52 · 42 | 0.30 · 30 | 0.04 · 9 | failed |
| `site_extract` | 0.60 · 23 | 0.53 · 11 | 0.60 · 18 | 0.60 · 13 | 0.20 · 6 | `n/r` |
| `corpus_ask` | 0.87 · 18 | **1.00** · 8 | 0.87 · 17 | 0.67 · 16 | 0.40 · 6 | `n/r` |

- **`gpt-oss:20b`** is the one to try if you want speed and answers from given
  text. It is better on `corpus_ask` (15 of 15) at less than half the time. Its
  classes are worse, and you cannot tell it to skip thinking (section 6).
- **`qwen3:14b`** is the closest other choice to Gemma 4 `12B`, and was never better.
- **`lfm2:24b-a2b`** is the only model that is fast (2 to 9 seconds) and of no use. The
  article named it as best for tool calls. We did not run `agent_loop` on it, so we
  did not test that claim; on the text tasks it fails.
- **`qwen3.8:27b`** is the article's choice for the best quality that still fits. On this
  machine it does not fit: 22% of it ran on the CPU, and it was still the slowest by far.

## 6. Adapter behavior and problems we know of

Each item below says why the adapter
(`backend/internal/modules/ai/ollama.go`, `ollamathink.go`) acts as it does.

**Gemma 4 thinks by default.** Ollama keeps the own default of the model when a
request says nothing, and the Gemma 4 default is to think. The reasoning comes
first, counts against the output budget, and comes back in a separate field.
Direct calls with the same JSON schema: `gemma4:e4b` took 0.55 seconds with thinking off
and 15.5 seconds with it on (429 thinking tokens). `gemma4:12b` took 1.8 seconds off and 87 seconds
on.

With a budget of 1,024 tokens it used all of it thinking, and returned an empty
answer (`done_reason: length`). One certification task (`capture_classify` on
`e4b`) changed from 3 minutes to 12.

**`gpt-oss` breaks on `think: false`.** `gpt-oss` accepts only a level (`low`,
`medium`, `high`). Given `false` next to a JSON schema, it returns empty content
with a clean `stop`, and a certification run with it scored 0 of 15. 

So the adapter asks Ollama once per model what it accepts (`/api/show`). It sends `false`
where the model can turn thinking off, and the lowest level where it cannot. It
sends nothing for a model that does not think. A server with no `/api/show` (a
proxy in front of Ollama) gets nothing.

**The adapter reads `done_reason`.** The router tries a short answer again with
`answer more briefly` only when the response says the output reached its limit. Ollama
reports such an answer in `done_reason`, and the adapter passes it on so the router can
try again.

**Thinking floors and on/off models.** Two onboarding sites (`company_message` in
`cold_start`, and `sitereadmessage`) ask for `low` thinking. Gemma 4 through Ollama
can only switch thinking on or off. With `think: true`, one turn used about 4,300
tokens and 7 minutes thinking, against 110 tokens and 15 seconds with it off. Every
`cold_start` run ran out of time before the answer. So a floor leaves such a model
off ([ai-thinking.md](../reference/ai-thinking.md)).

**A local judge is slower and less safe.** The `sovereign` profile binds the
candidate only, so the default cloud judge is allowed (see
[vllm-self-hosting.md](vllm-self-hosting.md)). An earlier run with local judges
found two problems. A `7B` local judge (`mistral`) could not be trusted: one call
scored a correct answer 0 and pushed two tasks down a level. `gpt-oss:20b` is the
better local judge, but it shares the GPU with the candidate. Over 12 tasks it
added 4 to 9 seconds per call (7.6 at the median) to the recorded latency.

**Ollama stops answering.** In the about 490 `gemma4:12b` calls, one returned HTTP 500
after 5 minutes. Ollama started its runner again, and the retry in the router
worked. It is
the reason `site_fact_extract` shows 309 seconds in section 4. The runs of the
other models logged five more, most on `qwen3.8:27b` while the machine was short
of memory.

Each one came back, but a local machine has a tail that a cloud API does not.
The server log is `~/.ollama/logs/server.log`.

**Small models return the wrong id.** 9 of the Gemma 4 `12B` failures, in four
tasks, were an answer about an id no one asked for. Two had a clear error (a `-` dropped, an extra group). The other 7 were correct
`UUID` values, but not the one given. The site refuses the whole answer, and does not apply a
verdict to it ([#6111](https://github.com/margince/margince/issues/6111)).

**The first `agent_loop` call is slow.** Every call carries a prompt of about
23,300 tokens. The model reads the prompt at about 100 to 130 tokens a second
here. So a call with a cold prompt cache took about 230 seconds (twice in the
run); calls that used the Ollama cache again took 6 to 8. A change to the start
of that prompt puts every turn back at minutes
([#6112](https://github.com/margince/margince/issues/6112)).

**Ollama skips number limits in a schema.** A `confidence` of 100
where the schema says 0 to 1 came back four times. The validator of the site
catches it after the fact; do not count on the format rule to hold it.

## 7. What we did not test

- **Thinking turned on.** All of the above ran with thinking off (or `low` for
  `gpt-oss`). We did not test whether it helps the weakest Gemma 4 tasks
  (`request_settlement`, `growth_fit`, `stage_evidence_extract`). The direct test
  used 429 tokens thinking on `e4b`, and used the whole budget of 1,024 tokens on
  `12b` without an answer.
- **More memory.** The `26B` and `31B` Gemma 4, and `qwen3.8:27b`, may be very good
  on a machine with 32GB or more. We only know they do not fit 24GB.
- **MLX builds.** Ollama publishes `-mlx` and `-nvfp4` tags. We have not checked
  whether they run, and run faster, on an `M4` with 24GB.
- **A laptop with no fan**, or any other chip. Speed grows with how fast the memory moves data.
- **`agent_loop` on any model but Gemma 4 `12B`**, and 23 of 28 tasks on the others.
- **Embeddings.** We tried only chat models; `bge-m3` is bound in the preset, but
  we did not measure its retrieval quality here.
- **Many calls at once.** One call at a time. Two users at once would queue on the
  GPU.

## 8. Run it again

Start Ollama as step 1 of
[enrich-with-a-local-llm.md](../how-to/enrich-with-a-local-llm.md) shows. Then pull
the model, and `gpt-oss:20b` only for a local judge, and run:

```bash
ollama pull gemma4:12b
ollama pull gpt-oss:20b
# The committed records used the default cloud judge; a local judge works too:
make e2e-ai ROUTING=config/presets/gemma4_local_ollama.yaml \
  JUDGE=ollama:gpt-oss:20b TASK=capture_classify
```

`make e2e-ai-report` prints what is already committed. With the default cloud
judge (leave out `JUDGE=`), the run is faster, and its recorded latency is only that
of the candidate. But it pays for judge tokens, where a local judge costs
nothing.

To time a model with no judge in the way, call it directly:
`curl localhost:11434/api/chat` with `"stream": false` and read
`eval_count / eval_duration`, and `ollama ps` for where it runs.
