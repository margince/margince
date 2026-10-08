<!-- prose:plain -->
# Tune AI requests

How to read the record of the model calls of an installation, and then change how Margince
sends them. You can set the routing and privacy of OpenRouter, and the thinking level and
timeouts of each task. All of it is under **Settings → AI**. To change it, you need the `ai_routing`
grant, and to see the numbers you need `ai_diagnostics` read.

## 1. Read the numbers first

- **Providers card.** Each provider reports its calls of the last 7
  days: `7 d: 98 calls · 11 failed (4 timeouts)`. Open a provider with **Edit**.
  The **Recent calls** part of its sheet breaks them down by host (OpenRouter only), model or
  tier. It covers `24 h`, `7 d` or `30 d`, with `p50`, `p95` and cost. A row opens the call log
  for only these calls.
- **Model tiers card.** The mark next to a tier opens its health, with its
  line for 7 days and how it sorts its hosts.
- **AI tasks card.** **Edit** opens the sheet of a task. **Recent calls** says how
  many calls have an answer, and from which step of the route. It also says why a step gave a
  call up (`timed out`, `failed`, `not sure enough`). The latency view puts the
  `p50` and `p95` next to the timeout that stops a call.

## 2. Choose the host routing

When an `openai_compatible` binding points at OpenRouter, the **Serving** part of the binding
form takes the request JSON of OpenRouter itself. Leave it empty for the
default that ships (`sort: throughput`, `fp16` or `bf16`, `require_parameters`). Write
`{}` to let OpenRouter route on its own.

- To speed up the slowest calls, sort by `throughput`. `latency` reached slower hosts in
  our tests ([reference/openrouter.md](../reference/openrouter.md)).
- `preferred_max_latency` and `preferred_min_throughput` only change the order of hosts.
  Use them with a sort or a filter when the slowest calls are your problem.
- `max_price` leaves out the hosts above it. With a sort, it can leave out the host
  the sort would choose.

The form asks the server as you type, and it lists each problem with its line and
key path. The **What OpenRouter will be asked for** part shows the full request in
plain words. It marks each key with where it comes from: the Margince default, the
connection, this tier, or each task.

## 3. Set privacy on the connection

While the service of an `openai_compatible` provider is OpenRouter, its sheet has **OpenRouter settings**.
These are: no data retention, refuse hosts that learn from prompts,
distillable models only, allow fallbacks, and the host lists. They apply to
every tier on the connection, and a tier cannot have other values. The save
refuses a tier value that does not agree with the connection. The settings in your OpenRouter account
also apply, and this page does not show them.

## 4. Set the thinking level and timeouts of a task

In the sheet of a task:

- **Thinking level** sends one level through the thinking setting of each provider.
  A model with no thinking control does not use it, and a broker model gets
  the closest level it lists. *Default* leaves it to the binding and the
  floor of each prompt.
- **Decision model timeout** (deciding tasks only, `5` to `60 s`, default `15 s`) is
  how long the decision model may take. After that, the task uses its tier
  model instead. If its `p95` is close to the line and the fallback answers, give it
  some more time. Then timeouts turn into answers at the first try.
- **Model call timeout** (`10` to `300 s`, default `300 s`) stops each model call on
  the ladder that runs longer. Then the next tier gets a try. Make it smaller when a host
  stops answering and the tier above it works.

**Save settings** writes for the whole installation, and every role applies it within a
minute. **Reset to defaults** removes the task's own value.

## 5. Check the change

Come back to the same window after some calls. The page of a call shows the
routing block, thinking level and time limit that the call used, under its
settings.
