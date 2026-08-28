// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// "Do it now", for a unit whose value otherwise arrives on a schedule.
//
// ErrAttendedIngest tells a unit what to do when somebody asks for an on-demand
// sync — "a unit wanting an on-demand sync enqueues its job rather than
// ingesting inline" — and until this existed there was no way to enqueue one.
// The sentence described a pattern the surface did not offer, and the refusal
// it explains had no escape hatch.
//
// What that cost is not abstract. A member connects an account, chooses what to
// capture, presses save, and nothing can happen until the next scheduled tick —
// a full cadence away, longer once adaptive backoff has engaged. From the
// outside that is indistinguishable from a broken feature, and the first person
// it happens to is the rep whose belief in the connector is the whole product.
//
// WHY THE OTHER THREE ANSWERS ARE WORSE, since each is the obvious one:
//
//   - draining inline on the save is what ErrAttendedIngest refuses, and
//     correctly: ingress runs on the authority of the member whose credential
//     produced the record, and a call that ALSO has a caller has two
//     authorities in play;
//   - shortening the cadence multiplies a login and a handshake per member,
//     for every member, to make one member feel responsive — the exact cost
//     adaptive cadence exists to reduce;
//   - saying nothing is what shipped.
//
// WHAT CROSSES HERE IS A REQUEST TO SCHEDULE, never an authority to ingest. The
// tick it asks for runs exactly as the clock's does: unattended, under the
// workspace's own agent seat, with the same grants. So ErrAttendedIngest is
// untouched — a unit still cannot ingest on a caller's authority, it can only
// ask for the unattended run sooner.
//
// WHAT IT PROMISES is that the job is QUEUED, and nothing about when it runs.
// That is the honest contract and it is also the useful one: a screen can then
// say "checking now" truthfully instead of guessing, where a synchronous
// version would have to hold a request open across a provider handshake it does
// not control.
//
// WHAT IT CANNOT DO, by construction rather than by check:
//
//   - reach another unit's job. The name is resolved against the DECLARING
//     unit's set, which the Runtime knows and the handler cannot supply.
//   - reach another tenant. The workspace is the invocation's, taken the way
//     every other capability takes it, so there is no argument to get wrong.
//   - outrun itself. Repeated calls coalesce onto one queued run for the
//     workspace, so a member holding down save enqueues one tick, not a
//     thousand.

import "errors"

// JobName is the name of one of the DECLARING unit's own jobs.
//
// A distinct type rather than a string, because on this surface a bare string
// is the shape a handler could use to ask to be re-scoped, and the seam refuses
// that parameter class on purpose. This one cannot be that: it is resolved
// against the declarations of the unit the Runtime was minted for, so a name
// belonging to another unit reads as a name belonging to nobody.
type JobName string

// ErrNoSuchJob reports a name that is not one of this unit's declared jobs.
//
// A unit may only ask for its OWN, and it names it rather than being given a
// default: a unit with two jobs has two answers to "run it now", and picking
// one for it would silently sync the wrong thing on a screen that said it
// synced the right one.
var ErrNoSuchJob = errors.New("extension: this unit declares no job by that name")

// ErrNoUnattendedSeat reports a workspace with no agent seat, for which no
// unattended tick can run at all.
//
// It is the same state the scheduled fan-out skips a workspace for, said out
// loud here because a caller ASKED. A screen reporting "checking now" over a
// tick that will never run is the failure this capability was added to end,
// reappearing one layer along.
var ErrNoUnattendedSeat = errors.New("extension: this workspace has no agent seat, so no unattended tick can run for it")
