<!-- prose:plain -->
# AI provider health: a provider that answers nobody

A budget stop is the installation running out of its own allowance. A provider can also stop answering
for all callers. A pass that keeps asking then charges every item for a failure that is not the fault of
the item. So each provider reports one health, `ProviderHealth` in `internal/shared/ports/model/health.go`,
and the router stops on it, in place of reading error text again.

| Health | Meaning | Blocks calls |
|---|---|---|
| `ok` | answering | no |
| `degraded` | three timeouts in a row; some calls still pass | no |
| `down` | host cannot be reached, or three `5xx` in a row | yes |
| `out_of_credit` | the account is empty (402, or a 400, 403, 429 or other `4xx` whose text says the balance is empty) | yes |
| `unauthorized` | the key is revoked or not valid (401, or a 400 or 403 whose text names the key) | yes |

`out_of_credit` and `unauthorized` trip on **one** failure, because only an operator changes them. They
are probed again every 15 minutes. `down` trips at once on a host that cannot be reached, or after three
`5xx` in a row. It is probed at 30 seconds, and the wait doubles up to 5 minutes.

Only one probe runs at a time, and it holds its slot for the longest call, five minutes. Other calls
during it are refused with no trace and no charge, and told to retry soon. One success clears any state,
and so does a key test that passes (`POST /ai/provider-keys/{provider}/test`), at once. Saving a key or
the routing clears it at the next routing check, about every 30 seconds in each process. A provider
removed from routing is cleared too.

The status codes alone do not decide. A 401 always marks the key `unauthorized`. A 400 or 403 does so
only when its text names the key or credential. A 400, 403 or other `4xx` marks the account empty only
when its text says the balance is empty (402 always does).

Take a plain 403, a flag from a content filter, or a model not turned on for the project. Each is the
message's own fault, and is still charged. The rule that decides is `providerFaultOf` in `budget.go`.

**A refusal is read by its text.** A 400 or 403 marks the key rejected only when its text says so
(`api key not valid`, `invalid api key`, `reported as leaked`). It never does so for a vendor's `does not have permission` refusal for one model. An empty balance is known from a 402, or from text such as
`credit balance is too low`.

**Health belongs to the provider.** Every tier bound to one provider sees the same state. So the failure
of a call on one tier blocks the others, and one fixed key frees them all.

**A 429 for too many requests leaves health alone.** It says "slow down", not "stop". The ladder moves up
to the next rung as before, and a rung that is only busy never marks its provider blocked. Only a 429 for
a used-up quota means the account is empty.

**The router skips a blocked rung.** No request leaves the process, and no timeout is waited out.
Nothing is charged or traced as a provider error (`attemptLadder` in `tracing.go`, `serveAttempt` in
`router.go`). When every rung is blocked, the router returns `ErrProviderDown`. It holds the provider,
its health and the moment of the next probe.

**A lane treats it as a deferral.** It is handled like the budget stop, and `IsDeferral` covers both.
The attempt is given back, the item waits until the retry moment, and the pass stops. An outage does not
park senders as `unsure`, and does not spend the attempts of enrich and River jobs.

- The call that trips an `out_of_credit` or rejected key status gets its attempt back too.
- So does a call that finds the host cannot be reached (`down` trips at once), and every failed probe
  while blocked.
- Each returns a deferral that keeps the cause. So a user who waits on the answer gets the 503 for that
  reason on the first call.
- The first two `5xx` of a run of three are charged, since they are ordinary failures. The third trips
  `down` and gets its attempt back.
- A timeout never gets its attempt back. Three in a row make the provider `degraded`, which blocks and
  holds back nothing, so every call that timed out is charged.
- A failure that is the message's own fault still charges the item, because it would fail on a provider in good health
  too.

An embedding for a blocked provider is refused with no trace, with `ai.ErrProviderDown`. It is not
wrapped as a failure of the embed lane. Search reindex callers see it like any other embed failure, and
retry on their own schedule.

**A request a user waits on fails fast.** It returns a 503 with a clear code (`provider_out_of_credit`,
`provider_unauthorized` or `provider_unavailable`). Its message tells the user to contact their system
admin (`internal/compose/modelfailure`). Nobody waits out a timeout to learn the same thing.

`GET /ai/provider-health` lists the providers that are not answering as normal. It reads a view the API
and the worker share through Redis. Every process publishes its status changes, and the request reads
the merged view. The worst or blocking status wins, so an outage that only the worker found still shows.

Without Redis, a process shows only its own view. Keys expire after about half an hour, so a status is
refreshed every 10 minutes while the provider stays in a bad state. It is not a probe.

Settings → AI models marks the provider, and Settings → System health shows the **AI provider status**
card. An operator reopens the work an outage already parked:
[recover-after-a-provider-outage.md](../how-to/recover-after-a-provider-outage.md). The budget's own
deferral is above, under *The monthly budget* in [ai-runtime.md](ai-runtime.md). What each task does in
an outage, row by row, is generated into [ai-provider-outages.md](../reference/ai-provider-outages.md).

## What is not covered

- A stream that opens cleanly but fails inside the stream does not count as a provider failure.
- A call already under way on an old key can trip the provider again after a fix.
- A probe cut short by the caller's own deadline counts as a failed probe.
- While a provider is blocked, only its probe counts. A call admitted before the block that finishes
  later does not bring the provider back, and does not start the back off again.
- Other processes do not stop on the shared status; it is for display.
- A website read or voice build that waits for the provider keeps the status code `budget_deferred`,
  which its CHECK rule needs. Only its status detail (`shared/kernel/providerwait`) tells the two waits
  from each other.

## Where a provider wait is counted

The list of waiting work under Settings → AI shows two numbers per carrier. `count` is what a higher
allowance would resume, and the budget pass wakes only that. `waiting_on_provider` is work that waits for
the provider's next probe, which resumes by itself. A company scan is marked by
`degrade_reason = 'provider_deferred'`; a site read and a voice build by the detail they write.
