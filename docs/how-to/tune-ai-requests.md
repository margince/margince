# Tune AI requests

How to read what an installation's model calls did, and then change how they
are sent: OpenRouter's routing and privacy, and each task's thinking level and
timeouts. Everything here is under **Settings → AI** and needs the `ai_routing`
grant to change, and `ai_diagnostics` read to see the figures.

## 1. Read the figures first

- **Providers card.** Each provider says what its calls did in the last seven
  days: `7 d: 98 calls · 11 failed (4 timeouts)`. Open a provider with **Edit**;
  its sheet's **Recent calls** splits them by host (OpenRouter only), model or
  tier over 24 h, 7 d or 30 d, with p50, p95 and cost. A row opens the call log
  filtered to those calls.
- **Model tiers card.** The dot beside a tier opens its health, now with its
  seven-day line and how it sorts its hosts.
- **AI tasks card.** **Edit** opens a task's sheet. **Recent calls** says how
  many calls got an answer, from which step of the route, and why a step gave a
  call up (`timed out`, `failed`, `not sure enough`). The latency bar places the
  p50 and p95 against the timeout that stops a call.

## 2. Choose the host routing

On an `openai_compatible` binding pointed at OpenRouter, the binding dialog's
**Serving** section takes OpenRouter's own request JSON. Leave it empty for the
shipped default (`sort: throughput`, fp16 or bf16, require parameters); write
`{}` to let OpenRouter route on its own.

- To cut a latency tail, sort by `throughput`. `latency` reached slower hosts in
  our measurements ([OpenRouter: upstream selection](../reference/openrouter.md)).
- `preferred_max_latency` and `preferred_min_throughput` only reorder hosts;
  pair them with a sort or a filter when the tail is what matters.
- `max_price` filters out hosts above it; with a sort, it can exclude the host
  the sort would have picked.

The editor asks the server as you type. Each problem is listed with its line and
key path, and **What OpenRouter will be asked for** shows the merged request in
plain words, each key marked with where it came from: Margince's default, the
connection, this tier, or each task.

## 3. Set privacy on the connection

The OpenAI-compatible provider's sheet has **OpenRouter settings** while its
service is OpenRouter: zero data retention, refuse hosts that train on prompts,
distillable models only, allow fallbacks, and the host lists. These apply to
every tier on the connection, and a tier cannot carry different ones: the save
refuses a tier value that disagrees with the connection's. Settings in your OpenRouter account also apply and are not
shown here.

## 4. Set a task's thinking level and timeouts

In a task's sheet:

- **Thinking level** sends one level through each provider's own thinking
  setting; a model with no thinking control ignores it, and a broker model is
  sent the nearest level it lists. *Default* leaves it to the binding and each
  prompt's floor.
- **Decision model timeout** (deciding tasks only, 5 to 60 s, default 15 s) is
  how long the decision model may take before the task falls back to its tier
  model. If its p95 sits near the line and the fallback answers, raising it a
  little turns timeouts into first-try answers.
- **Model call timeout** (10 to 300 s, default 300 s) stops each model call on
  the ladder that runs longer; the next tier is then tried. Lower it when a host
  hangs and the tier above answers well.

**Save settings** writes installation-wide; every role applies it within a
minute. **Reset to defaults** clears the task's override.

## 5. Check the change

Come back to the same window after some traffic. The call detail shows the
routing block, thinking level and deadline each call was sent with, under its
configuration.
