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
| `out_of_credit` | the account is empty (402, or a 429 that names quota) | yes |
| `unauthorized` | the key is revoked or invalid (401, 403) | yes |

`out_of_credit` and `unauthorized` trip on **one** failure, because nothing but an
operator changes them, and are probed again every 15 minutes. `down` trips at once
on an unreachable host, or after three consecutive 5xx, and is probed at 30 seconds
doubling to 5 minutes. Saving a key or the routing forgets the provider's state, so
the next call is the probe and a fixed key is felt immediately. One success clears
any state. The classification is `providerFaultOf` in `budget.go`.

**Health belongs to the provider, not to a tier.** Every tier bound to one provider
sees the same state, so the failure of one tier's call blocks the others and one
fixed key unblocks them all.

**A 429 that is a throttle is not a health state.** It says "slow down", not "stop":
the ladder escalates to the next rung as before, and a rung that is merely busy
never marks its provider blocked. Only a quota 429 means the account is empty.

**The router skips a blocked rung with no call.** No request leaves the process, no
timeout is waited out and nothing is charged or traced (`attemptLadder` in
`tracing.go`, `serveAttempt` in `router.go`). When every rung is blocked the router
returns `ErrProviderDown` carrying the provider, its health and the moment of the
next probe.

**A lane treats it as a deferral, exactly like the budget stop.** `IsDeferral`
covers both: the attempt is refunded, the item waits until the retry moment and the
pass stops, so an outage no longer parks senders as unsure or spends the attempts of
enrichment and River jobs. A failure that is the message's own still charges the
item, because it would fail on a healthy provider too.

**An interactive request fails fast.** It returns a 503 with a specific code,
`provider_out_of_credit`, `provider_unauthorized` or `provider_unavailable`, and a
message telling the user to contact their system administrator
(`internal/compose/modelfailure`). Nobody waits out a timeout to learn the same.

`GET /ai/provider-health` lists the providers that are not answering normally, from
the view of the process serving the request. It is not a probe and not a shared
record, so two processes can disagree for a moment. Settings → AI models marks the
provider and Settings → System health shows the **AI provider status** card. Work an outage already parked
is reopened by an operator:
[recover-after-a-provider-outage.md](../how-to/recover-after-a-provider-outage.md).
The budget's own deferral is above, under *The monthly budget* in [ai-runtime.md](ai-runtime.md).
