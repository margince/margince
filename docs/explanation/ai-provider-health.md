# AI provider health — a provider that answers nobody

A budget stop is the installation running out of its own allowance. A provider can
also stop answering for everyone, and a pass that keeps asking charges every item
for a failure that is not the item's. Each provider therefore reports one health,
`ProviderHealth` in `internal/shared/ports/model/health.go`, and the router brakes
on it instead of re-reading error text.

| Health | Meaning | Blocks calls |
|---|---|---|
| `ok` | answering | no |
| `degraded` | three timeouts in a row; some calls still pass | no |
| `down` | host unreachable, or three 5xx in a row | yes |
| `out_of_credit` | the account is empty (402, or a 400, 403, 429 or other 4xx whose text says the balance is empty) | yes |
| `unauthorized` | the key is revoked or invalid (401, or a 400 or 403 whose text names the key) | yes |

`out_of_credit` and `unauthorized` trip on **one** failure, because nothing but an
operator changes them, and are probed again every 15 minutes. `down` trips at once
on an unreachable host, or after three consecutive 5xx, and is probed at 30 seconds
doubling to 5 minutes. Only one probe runs at a time and it holds its slot for the longest
call, five minutes; other calls during it are refused untraced and uncharged, told to
retry shortly. One success clears any state, and
so does a successful key test (`POST /ai/provider-keys/{provider}/test`) at once. A
key or routing save clears it at the next routing recheck, about every 30 seconds in
each process, and a provider removed from routing is cleared too.

The status codes alone do not decide. A 401 always marks the key unauthorized, but a
400 or 403 does so only when its text names the key or credential, and a 400, 403 or
other 4xx marks the account empty only when its text says the balance is empty (402
always does). A plain 403, a moderation flag or a model not enabled for the project is
the message's own, and is still charged. The classification is `providerFaultOf` in
`budget.go`.

**A refusal is read by its text.** A 400 or 403 marks the key rejected only when its
text says so (`api key not valid`, `invalid api key`, `reported as leaked`), never for a vendor's "does not have permission" refusal for one
model. An empty balance is recognised from a 402 or from text such as `credit balance
is too low`.

**Health belongs to the provider.** Every tier bound to one provider
sees the same state, so the failure of one tier's call blocks the others and one
fixed key unblocks them all.

**A throttling 429 leaves health alone.** It says "slow down", not "stop":
the ladder escalates to the next rung as before, and a rung that is merely busy
never marks its provider blocked. Only a quota 429 means the account is empty.

**The router skips a blocked rung.** No request leaves the process, no
timeout is waited out and nothing is charged or traced as a provider error (`attemptLadder` in
`tracing.go`, `serveAttempt` in `router.go`). When every rung is blocked the router
returns `ErrProviderDown` carrying the provider, its health and the moment of the
next probe.

**A lane treats it as a deferral.** It is handled like the budget stop, and
`IsDeferral` covers both: the attempt is refunded, the item waits until the retry
moment and the pass stops. An outage no longer parks senders as unsure or spends the
attempts of enrichment and River jobs. The call that trips an out-of-credit or rejected-key status
is refunded too. So is a call that finds the host unreachable (down trips at once),
and every failed probe while blocked. Each returns a deferral that keeps the cause, so
an interactive user gets the reason-specific 503 on the first call. The first two 5xx of a run of
three are charged, being ordinary failures, and the third trips down and is refunded.
Timeouts are never refunded: three in a row make the provider degraded, which blocks
and defers nothing, so every timed-out call is charged. A failure that is the message's own still
charges the item, because it would fail on a healthy provider too.

An embedding for a blocked provider is refused untraced with `ai.ErrProviderDown`,
not wrapped as an embed-lane failure. Search reindex callers see it like any other
embed failure and retry on their own schedule.

**An interactive request fails fast.** It returns a 503 with a specific code,
`provider_out_of_credit`, `provider_unauthorized` or `provider_unavailable`, and a
message telling the user to contact their system administrator
(`internal/compose/modelfailure`). Nobody waits out a timeout to learn the same.

`GET /ai/provider-health` lists the providers that are not answering normally, from
a view shared between the API and the worker through Redis. Every process
publishes its status changes, and the request reads the merged view. The worst or
blocking status wins, so an outage only the worker saw still shows. Without Redis a
process shows only its own view. Keys expire after about half an hour, so a status is
refreshed every 10 minutes while the provider stays unhealthy. It is not a probe. Settings → AI models marks the
provider and Settings → System health shows the **AI provider status** card. Work an outage already parked
is reopened by an operator:
[recover-after-a-provider-outage.md](../how-to/recover-after-a-provider-outage.md).
The budget's own deferral is above, under *The monthly budget* in [ai-runtime.md](ai-runtime.md).
What each task does in an outage, row by row, is generated into
[ai-provider-outages.md](../reference/ai-provider-outages.md).

## What is not covered

- A stream that opens cleanly but fails in-band is not seen as a provider failure.
- An in-flight call on a superseded key can re-trip the provider after a fix.
- A probe cut short by the caller's own deadline counts as a failed probe.
- While a provider is blocked only its probe is heard: a call admitted before the block that finishes later neither restores the provider nor re-arms the backoff.
- Other processes do not brake on the shared status; it is for display.
- A website read or voice build waiting for the provider keeps the status code
  `budget_deferred`, which its CHECK constraint requires; only its status detail
  (`shared/kernel/providerwait`) tells the two waits apart.

## Where a provider wait is counted

Settings → AI's waiting-work list shows two numbers per carrier. `count` is what
an allowance raise would resume, and the budget recovery pass wakes only that.
`waiting_on_provider` is work waiting for the provider's next probe, which
resumes by itself. A company scan is told apart by `degrade_reason =
'provider_deferred'`; a site read and a voice build by the detail they write.
