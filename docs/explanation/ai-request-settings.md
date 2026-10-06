# AI request settings and deadlines

What an admin may change about how each task's model calls are sent, and how the
router keeps each call to its deadline. The runtime around it:
[ai-runtime.md](ai-runtime.md).

`ai.task_overrides` is an installation-wide setting, its own document with its
own ETag beside `ai.routing`, that an admin edits in each task's sheet under AI
tasks (`GET`/`PUT /ai/task-overrides`, judged first by
`POST /ai/task-overrides/preview`). Per task it may hold:

- **`thinking`**: one level for every site of the task, outranking the
  binding and the site floor ([AI thinking levels](../reference/ai-thinking.md)).
- **`decision_timeout_ms`**: how long the decision model may take on a task
  that asks one, 5,000 to 60,000; 15,000 when unset (`DecisionCallTimeout`).
- **`attempt_timeout_ms`**: how long each model call on the ladder may take,
  10,000 to 300,000; 300,000 when unset (`CallCeiling`).

When a field is unset, the router uses the default above; for `thinking`, the
binding and the site floor decide. The routing watcher republishes the overrides on every role within its recheck
interval, so a save takes effect installation-wide within a minute. It does not
move the routing version, so cached briefs stay keyed as they were. A changed
`thinking` is part of a call's result-cache key, so that task's next calls miss
answers cached at the old level and ask the model again. An override
for a task a later release drops is ignored by calls and named as stale by the
preview.

**Every ladder rung runs under its own deadline.** The rung's context is the
caller's with the task's attempt timeout on it, so one slow host spends at most
its share and the walk still reaches the rung above. A call its deadline stops
is recorded with the **`timeout`** sentinel: a failure everywhere
`provider_error` is (the health dot, served-task totals, the figures), named
apart so an admin can tell a slow host from a broken one. A caller's own
cancellation is never a timeout.

**Each attempt records what it was sent.** Its `config_hash` points at an
`ai_call_config` row whose `provider_params` holds the OpenRouter `provider` and
`reasoning` blocks that tier rendered, the task's thinking level and the
deadline (`deadline_ms`). The row is fixed per (binding, task, tier) rather than
per call, so the dimension stays a handful of rows; `GET /ai/calls/{id}` shows
it with the call.

All AI call figures come from one endpoint. `GET /ai/call-stats` counts a window's attempts
(24 h, 7 d or 30 d, cache hits excluded) by provider, model, upstream host, tier
or task: calls, failures, timeouts, p50 and p95, tokens and cost. Cost uses the
cost report's own expression, and failure uses the health read's own predicate. `GET /ai/call-stats/flow` shows which step of one task's route
answered its logical calls and why the walk moved past each step. The provider
sheets, the tier popover, the binding dialog and the task sheet all read these;
[Tune AI requests](../how-to/tune-ai-requests.md) is how to decide by them.
