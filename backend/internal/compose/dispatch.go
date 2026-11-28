// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// How a scheduled pass reaches the installation: runInstallationPass runs it in
// this process, and dispatchOnce / dispatchOne queue a child row. A failed pass
// fails its row, because a failure logged inside a row River then records as
// completed is success on the outside and silent failure within.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/zonefault"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The bounded queues a fanned-out pass lands on. MaxWorkers: 5 bounds only
// the default queue, and deep_read and rate_refresh already run alongside it
// on the same process against one pgx pool — so converted sweeps must not
// simply pile onto default.
const (
	// aiCaptureQueue carries the passes that make serial model calls. Two
	// workers, matching deep_read: the same species of work — long,
	// model-bound, and fine to run behind the short maintenance jobs. WHICH
	// kinds land here is api/jobs.yaml's to say, not this comment's.
	aiCaptureQueue      = "ai_capture"
	aiCaptureMaxWorkers = 2
)

// workspaceSweepOpts is the enqueue policy for one fanned-out child, read
// from the child kind's own declaration. Taking the KIND rather than the
// numbers is what makes the sweep tag unforgettable: a FLEET PASS's child opts
// are built here or in fanOutChildOpts and nowhere else, and both stamp it.
//
// oneOffChildOpts below builds the same kinds' opts WITHOUT the tag, and is
// the only thing that does. That is not a hole in the argument, it is the
// distinction the tag exists to draw: a job enqueued for one already-known
// workspace by an event is not one workspace's share of a fleet pass, and the
// helper takes no fleet as an argument, so it cannot be reached from a
// dispatcher's loop by accident.
//
// The attempt cap is small on every fan-out kind because a fanned-out pass's
// real retry cadence is the dispatcher's tick — a workspace that fails is
// re-enqueued on the next pass — and River's ladder is there only to ride out
// a transient blip inside one tick window. Left unset it would silently
// become River's default ladder of 25 attempts on attempt⁴ backoff, which
// retries far more aggressively than the tick early on and far less often
// later; and because retryable is one of activeSweepStates, a backing-off job
// suppresses the tick's own re-enqueue of that workspace until it discards.
// api/jobs.yaml carries each kind's number and the reason for it.
func workspaceSweepOpts(childKind string) *river.InsertOpts {
	spec := fanOutChildSpec(childKind)
	if spec.OptsOwner != jobs.OptsFanOut {
		panic("compose: " + childKind + " declares an opts_owner other than fan_out, so its queue and attempt cap are not this helper's to set")
	}
	return markedAsFleetPass(jobs.QueuedAsKind(childKind, &river.InsertOpts{
		MaxAttempts: spec.MaxAttempts,
		UniqueOpts:  river.UniqueOpts{ByArgs: true, ByState: activeSweepStates},
	}))
}

// oneOffChildOpts is workspaceSweepOpts' counterpart for the same child kind
// enqueued for ONE already-known workspace by an event — a tenant's backfill
// finishing, not a clock reaching a fleet. It reads the queue and the attempt
// cap from the same declaration, so the two doors cannot route one workspace's
// pass onto a queue the fleet's share of it never lands on.
//
// It deliberately omits BOTH of the things that make a fleet child a fleet
// child, and each omission is the point rather than an oversight:
//
//   - No sweep tag. The tag is what lets a reader tell one workspace's share
//     of a fleet pass from the same kind enqueued for its own reason, and the
//     sweep gauges count tagged rows. A one-off tagged as a fleet pass would
//     inflate the coverage reading of a pass that never ran.
//   - No active-state uniqueness. The event's whole claim is that new rows
//     landed, and a pass already RUNNING may have read the workspace before
//     they did. Deduplicating against it would drop exactly the data the
//     event fired about, and the caller would have no way to tell.
//
// Both are safe to omit only because a one-off enqueue is a single insert for
// a single known workspace: it fires once per event, so nothing here bounds a
// fan-out that could otherwise repeat per tick.
func oneOffChildOpts(childKind string) *river.InsertOpts {
	spec := fanOutChildSpec(childKind)
	if spec.OptsOwner != jobs.OptsFanOut {
		panic("compose: " + childKind + " declares an opts_owner other than fan_out, so its queue and attempt cap are not this helper's to set")
	}
	return jobs.QueuedAsKind(childKind, &river.InsertOpts{MaxAttempts: spec.MaxAttempts})
}

// oneOffPassOpts is oneOffChildOpts for a COLLAPSED pass: a scheduled kind that
// does the work in its own row rather than fanning out (ADR-0103).
//
// Its sibling insists the kind is somebody's fan-out child, which this one
// cannot be — that is the whole of the collapse. What it keeps is the reason
// that helper exists: the queue comes from the DECLARATION, so an on-demand
// enqueue lands on the same pool the scheduled tick uses instead of a queue the
// caller happened to name.
//
// The attempt cap is periodicPassMaxAttempts, the same one the CLOCK gives
// this kind: a collapsed pass enqueued by a trigger is the identical pass the
// tick enqueues, and the two doors already dedupe against each other, so a
// trigger-queued row riding a different ladder from a scheduled one would be
// the same pass behaving two ways depending on which door it came through. It
// is not read off the declaration because a caller-owned kind publishes no
// max_attempts — api/jobs.yaml refuses a number nothing applies.
//
// No sweep tag, for the reason oneOffChildOpts gives: the tag marks a row as
// one workspace's share of a fleet pass, and the gauges count tagged rows. A
// one-off wearing it would inflate the coverage reading of a pass that never
// ran.
func oneOffPassOpts(kind string) *river.InsertOpts {
	spec, ok := jobs.SpecFor(kind)
	if !ok {
		panic("compose: enqueuing " + kind + ", which api/jobs.yaml does not declare")
	}
	if spec.OptsOwner != jobs.OptsCaller {
		panic("compose: " + kind + " declares an opts_owner other than caller, so its queue is not this helper's to set")
	}
	return jobs.QueuedAsKind(kind, &river.InsertOpts{MaxAttempts: periodicPassMaxAttempts})
}

// fanOutChildren is every kind some dispatcher DECLARES it fans out to —
// fans_out_to, read as the registry it is, over BOTH halves of the declaration
// table (jobs.Declared covers the compiled core kinds and this installation's
// composed extension kinds).
//
// Recomputed per call rather than memoised. It used to be a sync.OnceValue,
// which was correct while the table was compiled and therefore fixed at link
// time; it is not correct now that an installation's composed kinds are
// declared at BOOT. A memoised answer is whatever the table held at the first
// insert of the process, so a fan-out that happened to run before
// jobs.RegisterComposed would cache the core set and then PANIC on every
// extension child forever after — a boot-ordering bug that would look like a
// bad kind. The cost it buys back is one map build over a few dozen specs per
// dispatched insert, which is beside a database round trip.
func fanOutChildren() map[string]struct{} {
	children := map[string]struct{}{}
	for _, spec := range jobs.Declared() {
		if spec.FanOutTo != "" {
			children[spec.FanOutTo] = struct{}{}
		}
	}
	return children
}

// fanOutChildSpec answers the declaration for a kind a dispatcher may fan out
// to, and refuses one no dispatcher declares. Both refusals are programming
// errors a boot-time tick surfaces immediately, not conditions a running
// deployment can reach: what a kind is, and who fans out to it, are fixed at
// compile time by the contract the generated table is built from.
func fanOutChildSpec(childKind string) jobs.Spec {
	spec, ok := jobs.SpecFor(childKind)
	if !ok {
		panic("compose: fanning out to " + childKind + ", which api/jobs.yaml does not declare")
	}
	if _, ok := fanOutChildren()[childKind]; !ok {
		panic("compose: fanning out to " + childKind + ", which no declared kind names in fans_out_to — declare the dispatcher's fan-out in api/jobs.yaml")
	}
	return spec
}

// insertManyFunc is the slice of River's insert surface a dispatch needs.
// Narrowed to the one method so the fan-out's failure posture can be proven
// without a database or a live client.
type insertManyFunc func(ctx context.Context, params []river.InsertManyParams) error

// runInstallationPass runs a scheduled pass once, in this process, over the
// installation's company.
//
// The callback receives the workspace id because the stores it reaches still
// take one. An installation with no company has nothing to pass over, so the
// pass ends without calling run.
//
// A pass that could not load the installation zone counted its days in UTC,
// and the pass records that once. A fault row that cannot be written fails the
// pass, so the retry writes it.
func runInstallationPass(ctx context.Context, pool *pgxpool.Pool, run func(context.Context, ids.UUID) error) error {
	ws, ok, err := database.LiveWorkspace(ctx, pool)
	if err != nil || !ok {
		return err
	}
	watched, zones := storekit.WatchZones(ctx)
	return errors.Join(run(watched, ws), zonefault.Record(ctx, pool, ws, zones))
}

// clientInsertMany binds the fan-out to the River client already in context —
// the shape gmailSyncWorker's dispatcher uses.
//
// The client is resolved INSIDE the closure, not when the closure is built:
// river.ClientFromContext panics when there is none, and a dispatch over an
// empty fleet never inserts, so resolving eagerly would turn "no workspaces
// yet" into a panic on a path that has nothing to do.
func clientInsertMany(ctx context.Context) insertManyFunc {
	return func(ctx context.Context, params []river.InsertManyParams) error {
		_, err := river.ClientFromContext[pgx.Tx](ctx).InsertMany(ctx, params)
		return err
	}
}

// dispatchOnce enqueues one fan-out child through an injected inserter, so the
// failure posture can be proven without a database or a live client.
//
// A refused insert fails the dispatcher: swallowing it would leave the pass
// un-run while River recorded the tick as completed.
func dispatchOnce(ctx context.Context, insert insertManyFunc, opts *river.InsertOpts, args river.JobArgs) error {
	params := []river.InsertManyParams{{Args: args, InsertOpts: jobs.QueuedAsArgs(args, markedAsFleetPass(opts))}}
	if err := insert(ctx, params); err != nil {
		return fmt.Errorf("compose: dispatching %s: %w", args.Kind(), err)
	}
	return nil
}

// dispatchOne enqueues ONE fan-out child through the River client working the
// current job. It serves the dispatchers that cannot use dispatchOnce because
// they fan out per CONNECTION or per BUILD rather than once for the
// installation: there is no one set to insert atomically, so they loop single
// inserts instead.
//
// The client is resolved per insert, and only inside the loop, for the reason
// clientInsertMany states: river.ClientFromContext panics when there is none,
// and a tick with nothing due must not turn "nothing to do" into a panic.
func dispatchOne(ctx context.Context, pool *pgxpool.Pool, args river.JobArgs, callerOpts *river.InsertOpts) error {
	runner := jobs.FromClient(pool, river.ClientFromContext[pgx.Tx](ctx))
	return runner.Enqueue(ctx, args, jobs.QueuedAsArgs(args, fanOutChildOpts(args.Kind(), callerOpts)))
}

// fanOutChildOpts is what dispatchOne hands River, decided by the child's
// declared opts_owner. The three owners genuinely differ, and routing the
// choice through the declaration is what keeps a dispatcher from flattening
// them into one:
//
//   - fan_out — the queue and attempt cap come from the declaration, exactly
//     as dispatchOnce's child gets them.
//   - args — the child's own InsertOpts() owns them, so it gets a TAG-ONLY
//     value and River's field-by-field merge leaves the args' rule standing.
//     Handing telegram_poll a populated opts would drop its per-bot
//     uniqueness, and two in-flight polls for one bot steal each other's
//     batches.
//   - caller — callerOpts, which is the caller's to build precisely because
//     the same value serves a path that is NOT a fan-out: voiceBuildInsertOpts
//     is shared with the user-initiated build. The tag goes on this call and
//     never into that shared helper, because a build someone asked for is not
//     one workspace's share of a fleet pass.
//
// callerOpts is read only for opts_owner: caller, and the guard runs in BOTH
// directions — supplying one for another owner is refused, and so is omitting
// one for caller. The two are the same defect: the reason a caller-owned kind
// carries that owner is that its options exist and are nobody else's to
// reconstruct, so a nil here yields the tag-only value and drops them, and a
// uniqueness window that was silently dropped reads, in the source, exactly
// like one that was applied.
//
// It is separate from dispatchOne so a unit test can assert all three
// postures directly, without a River client or a database: the postures are
// the whole content of the decision, and the insert below them is one line.
func fanOutChildOpts(childKind string, callerOpts *river.InsertOpts) *river.InsertOpts {
	spec := fanOutChildSpec(childKind)
	if spec.OptsOwner != jobs.OptsCaller && callerOpts != nil {
		panic("compose: insert options passed for " + childKind + ", whose opts_owner is not caller — they would be dropped, not merged")
	}
	if spec.OptsOwner == jobs.OptsCaller && callerOpts == nil {
		panic("compose: no insert options passed for " + childKind + ", whose opts_owner is caller — its queue and uniqueness window are the caller's to supply, and this child would be inserted with neither")
	}
	switch spec.OptsOwner {
	case jobs.OptsFanOut:
		return workspaceSweepOpts(childKind)
	case jobs.OptsArgs:
		return markedAsFleetPass(nil)
	case jobs.OptsCaller:
		return markedAsFleetPass(callerOpts)
	}
	panic("compose: " + childKind + " declares no opts_owner; every kind the contract compiles has one")
}

// markedAsFleetPass copies opts with the sweep tag added, so a reader can
// tell one workspace's share of a fleet pass from the same kind enqueued by
// hand — they are the same kind, and the tag is the only difference in the
// row. Tags are validated for format only and take no part in River's
// unique key, so this changes no scheduling behaviour.
//
// Every production caller is in this file. A FLEET PASS's child opts are built
// by workspaceSweepOpts (a fleet insert) or fanOutChildOpts (a single one) and
// nowhere else, and both stamp the tag — so a dispatcher cannot forget it,
// which is what an untagged child costs: silent absence from the sweep
// gauges, while the gauge's own HELP text blames River's retention for the
// gap. The third builder of the same kinds' opts, oneOffChildOpts, omits the
// tag deliberately and states why; it is reachable only from a site that
// already knows the one workspace it is enqueueing for. WHICH kinds may be fanned out to is the contract's to say and not this
// comment's: fans_out_to in api/jobs.yaml is the registry, and both builders
// refuse a kind no dispatcher declares there.
//
// It COPIES because a single dispatch shares ONE opts value across every
// workspace in its loop, and voiceBuildInsertOpts' value is shared with the
// user-initiated build path besides. Appending in place would grow one tag
// per workspace and hand the caller back a mutated struct.
//
// A nil opts yields a tag-ONLY value on purpose. For the fields that matter
// here — queue, max attempts, priority, tags and the uniqueness window —
// River falls back to the args' own InsertOpts whenever the explicit value
// leaves that field at its zero, so a caller that passes nil to let its
// args declare the uniqueness window (the telegram poll's per-bot rule)
// keeps that fallback intact. It is NOT a blanket rule over every field:
// metadata, for one, is defaulted rather than inherited.
func markedAsFleetPass(opts *river.InsertOpts) *river.InsertOpts {
	marked := river.InsertOpts{}
	if opts != nil {
		marked = *opts
	}
	if slices.Contains(marked.Tags, jobs.SweepTag) {
		return &marked
	}
	marked.Tags = append(slices.Clone(marked.Tags), jobs.SweepTag)
	return &marked
}
