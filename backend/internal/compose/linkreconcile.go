// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The nightly repair of captured mail that never reached its record.
//
// The cg:cohort-promote consumer fixes an ordering as it happens: a person
// arrives, their earlier mail is linked. That covers everything from the day it
// shipped and nothing from before it, and it depends on an event actually being
// delivered — a consumer that was down while a backfill ran leaves exactly the
// damage it exists to prevent.
//
// So the same repair also runs as a sweep, and the sweep is what makes the
// ordering irrelevant rather than merely handled. It is the reconcile a dozen
// comments in capture have promised for a year: a link-less connector activity
// is the retry marker, and this is the pass that reads it.
//
// The selector and the write ask the same question with the same guards, so the
// scan drains: every repaired row leaves the selection, and a row the write
// refuses is one the selector never offers. A workspace with nothing owed costs
// one query and writes nothing.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkReconcilePeoplePerTick bounds one workspace's pass.
//
// Each person costs one bounded repair transaction, so this is the ceiling on
// what a single tick writes. A workspace with more owed than this is not
// stranded: what the pass leaves behind still matches the selector, and the next
// tick takes the next slice — which is the same reason the repair itself is
// batched rather than unbounded.
const linkReconcilePeoplePerTick = 200

// linkReconcileDomainsPerTick bounds the company half of one pass, for the same
// reason: each domain is one plant transaction, and what a tick leaves behind
// still matches the selector.
const linkReconcileDomainsPerTick = 50

// LinkReconcileArgs is the sweep's (empty) job payload.
type LinkReconcileArgs struct{}

// Kind is the River job kind for the captured-mail link reconcile.
func (LinkReconcileArgs) Kind() string { return "link_reconcile" }

// FleetWide marks this a dispatcher: it enumerates and enqueues, and does no
// tenant work of its own (jobs.FleetWide).
func (LinkReconcileArgs) FleetWide() {}

type linkReconcileWorker struct {
	pool  *pgxpool.Pool
	store *people.Store
	log   *slog.Logger
}

func newLinkReconcileWorker(pool *pgxpool.Pool, store *people.Store, log *slog.Logger) *linkReconcileWorker {
	return &linkReconcileWorker{pool: pool, store: store, log: log}
}

// Work enqueues one pass per workspace, so a failure in one leaves the rest
// swept.
func (w *linkReconcileWorker) Work(ctx context.Context, _ *river.Job[LinkReconcileArgs]) error {
	return jobs.FaultContext(ctx, dispatchPerWorkspace(ctx, w.pool,
		workspaceSweepOpts(LinkReconcileWorkspaceArgs{}.Kind()),
		func(ws ids.UUID) river.JobArgs { return LinkReconcileWorkspaceArgs{Workspace: ws} }))
}

// LinkReconcileWorkspaceArgs is one workspace's repair pass.
type LinkReconcileWorkspaceArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (LinkReconcileWorkspaceArgs) Kind() string { return "link_reconcile_workspace" }

// WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
func (a LinkReconcileWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }

// linkReconcileWorkspaceWorker runs one workspace's pass, reusing the
// dispatcher's wiring rather than a second copy of it.
type linkReconcileWorkspaceWorker struct {
	*linkReconcileWorker
}

func (w *linkReconcileWorkspaceWorker) Work(ctx context.Context, job *river.Job[LinkReconcileWorkspaceArgs]) error {
	if _, err := workspaceJobCtx(ctx, job.Args); err != nil {
		return jobs.FaultContext(ctx, err)
	}
	sweepCtx := w.systemContext(ctx, job.Args.Workspace)
	owed, err := w.store.PeopleOwedACohortRepair(sweepCtx, linkReconcilePeoplePerTick)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	var linked, promoted int64
	var failed error
	for _, person := range owed {
		// Per person, each on its own transaction, so one contact whose repair
		// fails — a lock it could not take, a row a concurrent merge moved —
		// costs that contact and not the rest of the sweep.
		//
		// The failure is KEPT rather than logged away. A pass that repaired
		// nine hundred contacts and could not repair one did not succeed, and
		// River recording it green would retire the only signal that a contact
		// is permanently stuck. The retry re-walks the repaired ones for
		// nothing, which is cheap: they no longer match the selector.
		done, err := w.store.RepairPersonCohort(sweepCtx, person)
		if err != nil {
			failed = errors.Join(failed, fmt.Errorf("repairing %s: %w", person, err))
			continue
		}
		linked += done.Linked
		promoted += done.Promoted
	}
	if linked > 0 || promoted > 0 {
		w.log.InfoContext(ctx, "link reconcile: captured mail put back on its records",
			"workspace", job.Args.Workspace.String(),
			"people", len(owed), "linked", linked, "promoted", promoted)
	}
	lifted, err := w.liftFiledMeetingHolds(sweepCtx)
	if err != nil {
		failed = errors.Join(failed, err)
	}
	if lifted > 0 {
		w.log.InfoContext(ctx, "link reconcile: filed meetings are no longer held to their attendees",
			"workspace", job.Args.Workspace.String(), "meetings", lifted)
	}
	return jobs.FaultContext(ctx, errors.Join(failed, w.attachDomainBacklogs(sweepCtx, job.Args.Workspace)))
}

// liftFiledMeetingHoldsPerTick bounds the drain. The population is finite and
// shrinks as it is worked, so a small bound costs one probe a tick once it is
// empty and never delays the repair above it.
const liftFiledMeetingHoldsPerTick = 200

// liftFiledMeetingHolds re-derives the audience of records that are filed under
// something and still carry the limiter's "named nobody" hold.
//
// These are the rows this change would otherwise leave behind. A meeting
// captured before its attendee was a contact was held to its participants, the
// cohort repair filed it under them a day later, and nothing re-asked the
// question — so the meeting on a colleague's page stayed invisible to everyone
// but the people on the invitation, while the invitation EMAILS beside it were
// workspace-readable.
//
// ReasonNoCounterparty only. A judged sender's hold (ReasonNoRecord) is not
// this pass's to touch however the row is filed.
//
// It drains permanently: the recompute rewrites the reason on every row it
// selects, so a row worked once cannot match again, and afterwards the same
// predicate guards the invariant for the price of one probe.
func (w *linkReconcileWorker) liftFiledMeetingHolds(ctx context.Context) (int, error) {
	var held []ids.ActivityID
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id
			  FROM activity a
			 WHERE a.audience = 'participants'
			   AND a.audience_reason = $1
			   AND a.restricted_at IS NULL
			   AND a.archived_at IS NULL
			   AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id)
			   -- A row with no import rows is not a captured row, and the
			   -- recompute leaves it alone. Selecting one would return it every
			   -- tick, and a full page of them would starve the rows this can
			   -- actually repair.
			   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id)
			 ORDER BY a.occurred_at DESC, a.id
			 LIMIT $2`, activities.ReasonNoCounterparty, liftFiledMeetingHoldsPerTick)
		if err != nil {
			return fmt.Errorf("selecting the filed meetings still held: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.ActivityID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("reading a filed meeting still held: %w", err)
			}
			held = append(held, id)
		}
		return rows.Err()
	}); err != nil {
		return 0, err
	}
	lifted := 0
	for _, id := range held {
		// One transaction per meeting, like the repair above: a row another
		// writer holds a lock on costs that row and not the whole drain.
		if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
			return activities.RecomputeAudienceTx(ctx, tx, id)
		}); err != nil {
			return lifted, fmt.Errorf("re-deriving the audience of %s: %w", id, err)
		}
		lifted++
	}
	return lifted, nil
}

// attachDomainBacklogs gives the people on a company's domain their employer.
//
// It runs here, as the system, rather than on the create that records the
// company: attaching a person to a company is a write about the PERSON, and the
// human typing in a company name holds no authority over contacts they may not
// see. A rep scoped to their own records would otherwise plant employment for a
// colleague's private contact as a side effect of naming a company.
func (w *linkReconcileWorker) attachDomainBacklogs(sweepCtx context.Context, ws ids.UUID) error {
	owed, err := w.store.DomainsOwedTheirPeople(sweepCtx, linkReconcileDomainsPerTick)
	if err != nil {
		return err
	}
	planted := 0
	var failed error
	for _, domain := range owed {
		got, err := w.store.AttachDomainBacklog(sweepCtx, domain)
		if err != nil {
			failed = errors.Join(failed, fmt.Errorf("attaching %s: %w", domain.OrganizationID, err))
			continue
		}
		planted += got
	}
	if planted > 0 {
		w.log.InfoContext(sweepCtx, "link reconcile: a company's people were attached to it",
			"workspace", ws.String(), "domains", len(owed), "employed", planted)
	}
	return failed
}

// systemContext binds the workspace and the maintenance principal the pass runs
// under.
//
// The mail half creates nothing — it attaches messages the workspace already
// holds to records it already has. The company half DOES write: it plants the
// employment edges a domain's people are owed, which is precisely the write no
// human on this path may make, since a rep naming a company holds no authority
// over contacts they cannot see. A sweep with no human behind it is the honest
// actor for both.
func (w *linkReconcileWorker) systemContext(ctx context.Context, ws ids.UUID) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	// The correlation id is not decoration: storekit.EmitEvent REFUSES to
	// publish without one, so a pass that omitted it repaired nothing at all —
	// every contact failed on its own event, the whole sweep retried three
	// times and was discarded, and the backlog it exists to clear sat there
	// looking as though the job had simply not run yet.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem,
		ID:   linkReconcileActor,
	})
}

// linkReconcileActor names the sweep in the trail, so a link that appeared with
// nobody clicking is attributable to the pass that wrote it.
const linkReconcileActor = "link-reconcile"

// NewLinkReconcileWorkspaceWorkerForTest builds the workspace pass for an
// integration test that drives it the way River does.
//
// It exists because the store-level tests build their OWN context, so they
// prove the repair's SQL and nothing about the context this job runs under —
// and that is exactly where a defect hid: a missing correlation id made every
// publish refuse, so the sweep repaired nothing while looking like a job that
// had simply not run yet.
func NewLinkReconcileWorkspaceWorkerForTest(pool *pgxpool.Pool, store *people.Store) *linkReconcileWorkspaceWorker {
	return &linkReconcileWorkspaceWorker{
		linkReconcileWorker: newLinkReconcileWorker(pool, store, slog.Default()),
	}
}
