# SonarCloud findings this tree does not fix

SonarCloud is one of the gates a pull request has to pass, so a finding it
raises is normally something to fix rather than something to argue with. This
page is the short list of the exceptions: findings that are open in the
dashboard, that a reader will meet again the next time they sort the issue list,
and that stay open on purpose.

One entry per finding, each saying what the rule wants, what the code does
instead, and what would break if somebody "fixed" it. A finding that is merely
inconvenient does not belong here — only one where applying the rule would make
the product worse.

## godre:S8188 — `laneLifetime` in `backend/cmd/worker/lanes.go`

**What the rule wants.** `context.WithCancel` should be followed by
`defer cancel()` in the same function, so the context cannot outlive the call
that created it.

**What the code does.** `laneLifetime` creates the context the worker's event
lanes live on and returns three values: the context, the wait group that counts
the lanes, and a closure that cancels the context and waits for every lane to
leave. The caller — `run` in `backend/cmd/worker/main.go` — defers that closure
on the line after the call, before any lane exists.

**Why the rule cannot be applied.** The cancel is the *shutdown* of the lanes,
not a cleanup of the constructor. Deferring it inside `laneLifetime` would
cancel the context as that function returns, so every lane would be handed a
context that is already done and the worker would consume nothing. The lifetime
deliberately outlives its constructor, which is the one shape S8188 has no way
to distinguish from a leak: it reads a `CancelFunc` that leaves the function and
cannot follow it into the closure that calls it.

**What holds it instead.** The caller has to defer the closure — nothing in the
type system makes it. What returning the three values together removes is the
*ordering* hazard — the half nothing was holding when a deleted `defer` left
every test in the repository passing: a lane cannot exist before its own
shutdown is registered, because the wait group a lane must be given comes from
the same call that produced the closure. A caller that discards the closure is
still a caller with no shutdown, and the tests are what say otherwise —
`TestJoinCancelsTheLanesAndWaitsForThem` covers the cancel and the wait,
`TestJoinReportsALaneThatDoesNotStop` the bounded overrun.
