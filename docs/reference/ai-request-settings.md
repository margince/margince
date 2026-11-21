<!-- prose:plain -->
# AI request settings and deadlines

What an admin may change about how the model calls of each task are sent, and the endpoints that
show what each call was sent. Why each rung has its own deadline:
[ai-runtime.md](../explanation/ai-runtime.md#each-rung-has-its-own-deadline).

## The task overrides

`ai.task_overrides` is a setting for the whole installation. It is its own document, with its own ETag,
beside `ai.routing`. An admin edits it in each task's sheet under AI tasks
(`GET`/`PUT /ai/task-overrides`), and `POST /ai/task-overrides/preview` checks it first. Per task it may hold:

- **`thinking`**: one level for every site of the task. It wins over the binding and the site floor
  ([AI thinking levels](ai-thinking.md)).
- **`decision_timeout_ms`**: how long the decision model may take on a task that asks one, 5,000 to
  60,000; 15,000 when not set (`DecisionCallTimeout`).
- **`attempt_timeout_ms`**: how long each model call on the ladder may take, 10,000 to 300,000. When it
  is not set, a task a user waits on gets the limit below, and any other task gets 300,000 (`CallCeiling`).

When a field is not set, the router uses the default above; for `thinking`, the binding and the site
floor decide. The routing watcher publishes the overrides again on every role within its check window.
So a save takes effect across the installation within a minute.

The save does not move the routing version, so cached briefs stay keyed as they were. A changed `thinking` is part of a call's result cache key. So that
task's next calls miss answers cached at the old level, and ask the model again. Calls ignore an override
for a task that a later release drops, and the preview names it as stale.

## The limits of a task a user waits on

A task whose `execution_mode` is `interactive` gets its own default for each call. `make gen` builds it
from the committed certification records into `backend/internal/modules/ai/attemptlimits_gen.go`, and
`make drift` fails a table the records no longer give. It reads the slowest cloud record whose verdict lets
the model serve the task: `certified` or `supported_degraded`. The limit is 1.5 times that record's `p95`,
rounded up to a whole second, at least 30 seconds and at most 300 (`InteractiveAttemptTimeout`).

The limit holds only when each step of the call's route is a cloud model that such a record measured for
the task. A local model, or a model with no record, may be much slower, so its call keeps 300 seconds.
Some tasks also keep 300 seconds: a task no record measures, and a task that declares
`latency_scales_with_input` in `backend/api/ai-tasks.yaml`. The time of such a task grows with the size of
what it reads, so the `p95` of short test input cannot bound it.

The same calls also have a bound on the whole call. It covers every try: the steps up the ladder, the retry
after an answer the check refused, and the step up after that. It counts from the first try, and is two
calls at the task's limit, at most 300 seconds. A try starts only when at least 30 seconds of the bound are
left, or the task's limit when that is lower. When the bound runs out, no further call starts. If the last
call failed, the call ends with the `timeout` sentinel, and the reader sees that the assistant did not
answer. If the model answered and the check refused the answer, the call ends as a refused answer.

An admin's `attempt_timeout_ms` wins over the default, and the bound is then twice that value, with no cap.
A background task has no bound on the whole call. The call page shows the bound as `walk_bound_ms` beside
`deadline_ms`. A try is sent with `deadline_ms`, or with what is left of the bound when that is less.

The certification lane sends every task at 300 seconds with no bound. It measures the time the limits are
built from, so a run stopped at the limit would record the limit itself.

## The `timeout` sentinel

A call that its deadline stops is recorded with the **`timeout`** sentinel. The sentinel is a failure
everywhere `provider_error` is (the health dot, served task totals, the figures). It has its own name so
an admin can tell a slow host from a broken one. A caller's own cancel is never a timeout.

## The `canceled` sentinel

A call whose caller stopped waiting is recorded with the **`canceled`** sentinel: a page closed or
reloaded, or a job stopped. The router then makes no further attempt, so no other tier and no retry
is asked. The sentinel is not a failure. The health dot and the figures leave it out, and the calls list
shows it as its own outcome. The task flow counts it with the calls that were not answered, because nothing reached the caller.
A provider that fails at the moment its caller stops waiting is recorded as `canceled` too, because the
two cannot be told apart.

## What each attempt was sent

Each attempt's `config_hash` points at an `ai_call_config` row. That
row's `provider_params` holds the OpenRouter `provider` and `reasoning` blocks that the tier rendered, the
task's thinking level and the deadline (`deadline_ms`). The row is fixed per (binding, task, tier), not
per call, so the table stays a few rows. `GET /ai/calls/{id}` shows it with the call.

## The call figures

All AI call figures come from one endpoint. `GET /ai/call-stats` counts the attempts in a window (`24 h`,
`7 d` or `30 d`, cache hits left out) by provider, model, upstream host, tier or task. It counts calls,
failures, timeouts, `p50` and `p95`, tokens and cost. Cost is worked out the same way as in the cost report,
and failure with the same predicate as the health read. `GET /ai/call-stats/flow` shows which step of one task's
route answered its calls, and why the walk moved past each step. The provider sheets, the tier popover,
the binding dialog and the task sheet all read these.

The handbook page [AI providers](../handbook/ai-providers.md) is how a user decides by them.
