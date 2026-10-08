<!-- prose:plain -->
# AI request settings and deadlines

What an admin may change about how the model calls of each task are sent, and how the router keeps each
call to its deadline. The runtime around it: [ai-runtime.md](ai-runtime.md).

`ai.task_overrides` is a setting for the whole installation. It is its own document, with its own ETag,
beside `ai.routing`. An admin edits it in each task's sheet under AI tasks
(`GET`/`PUT /ai/task-overrides`), and `POST /ai/task-overrides/preview` checks it first. Per task it may hold:

- **`thinking`**: one level for every site of the task. It wins over the binding and the site floor
  ([AI thinking levels](../reference/ai-thinking.md)).
- **`decision_timeout_ms`**: how long the decision model may take on a task that asks one, 5,000 to
  60,000; 15,000 when not set (`DecisionCallTimeout`).
- **`attempt_timeout_ms`**: how long each model call on the ladder may take, 10,000 to 300,000; 300,000
  when not set (`CallCeiling`).

When a field is not set, the router uses the default above; for `thinking`, the binding and the site
floor decide. The routing watcher publishes the overrides again on every role within its check window.
So a save takes effect across the installation within a minute.

The save does not move the routing version, so cached briefs stay keyed as they were. A changed `thinking` is part of a call's result cache key. So that
task's next calls miss answers cached at the old level, and ask the model again. Calls ignore an override
for a task that a later release drops, and the preview names it as stale.

**Every ladder rung runs under its own deadline.** The rung's context is the caller's, with the task's
attempt timeout on it. So one slow host spends at most its share, and the walk still reaches the rung
above. A call that its deadline stops is recorded with the **`timeout`** sentinel.

The sentinel is a failure
everywhere `provider_error` is (the health dot, served task totals, the figures). It has its own name so
an admin can tell a slow host from a broken one. A caller's own cancel is never a timeout.

**Each attempt records what it was sent.** Its `config_hash` points at an `ai_call_config` row. That
row's `provider_params` holds the OpenRouter `provider` and `reasoning` blocks that the tier rendered, the
task's thinking level and the deadline (`deadline_ms`). The row is fixed per (binding, task, tier), not
per call, so the table stays a few rows. `GET /ai/calls/{id}` shows it with the call.

All AI call figures come from one endpoint. `GET /ai/call-stats` counts the attempts in a window (`24 h`,
`7 d` or `30 d`, cache hits left out) by provider, model, upstream host, tier or task. It counts calls,
failures, timeouts, `p50` and `p95`, tokens and cost. Cost is worked out the same way as in the cost report,
and failure with the same predicate as the health read. `GET /ai/call-stats/flow` shows which step of one task's
route answered its calls, and why the walk moved past each step. The provider sheets, the tier popover,
the binding dialog and the task sheet all read these. [tune-ai-requests.md](../how-to/tune-ai-requests.md)
is how to decide by them.
