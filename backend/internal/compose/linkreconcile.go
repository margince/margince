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
	"github.com/margince/margince/backend/internal/modules/capture"
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

// FleetWide marks this as answering for the whole installation: it owns no
// workspace, and walks them itself (jobs.FleetWide, ADR-0103).
func (LinkReconcileArgs) FleetWide() {}

type linkReconcileWorker struct {
	pool    *pgxpool.Pool
	store   *people.Store
	pending *capture.PendingStore
	log     *slog.Logger
}

func newLinkReconcileWorker(pool *pgxpool.Pool, store *people.Store, log *slog.Logger) *linkReconcileWorker {
	return &linkReconcileWorker{
		pool:    pool,
		store:   store,
		pending: capture.NewPendingStore(InstallationDB(pool)),
		log:     log,
	}
}

// Work enqueues one pass per workspace, so a failure in one leaves the rest
// swept.
func (w *linkReconcileWorker) Work(ctx context.Context, _ *river.Job[LinkReconcileArgs]) error {
	return jobs.FaultContext(ctx, runPerWorkspace(ctx, w.pool, w.reconcileLinksForWorkspace))
}

func (w *linkReconcileWorker) reconcileLinksForWorkspace(ctx context.Context, workspace ids.UUID) error {
	ctx = principal.WithWorkspaceID(ctx, workspace)
	sweepCtx := w.systemContext(ctx, workspace)
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
			"workspace", workspace.String(),
			"people", len(owed), "linked", linked, "promoted", promoted)
	}
	lifted, err := w.liftFiledMeetingHolds(sweepCtx)
	if err != nil {
		failed = errors.Join(failed, err)
	}
	if lifted > 0 {
		w.log.InfoContext(ctx, "link reconcile: filed meetings are no longer held to their attendees",
			"workspace", workspace.String(), "meetings", lifted)
	}
	released, err := w.releaseCalendarMeetingHolds(sweepCtx)
	if err != nil {
		failed = errors.Join(failed, err)
	}
	if released > 0 {
		w.log.InfoContext(ctx, "link reconcile: work-calendar meetings are workspace business again",
			"workspace", workspace.String(), "meetings", released)
	}
	asked, err := w.askAboutStrandedContacts(sweepCtx)
	if err != nil {
		failed = errors.Join(failed, err)
	}
	if asked > 0 {
		w.log.InfoContext(ctx, "link reconcile: captured contacts nobody had been asked about are queued",
			"workspace", workspace.String(), "contacts", asked)
	}
	retracted, err := w.retractNoiseJudgedContacts(sweepCtx)
	if err != nil {
		failed = errors.Join(failed, err)
	}
	if retracted > 0 {
		w.log.InfoContext(ctx, "link reconcile: contacts a noise verdict already covered are retracted",
			"workspace", workspace.String(), "contacts", retracted)
	}
	return jobs.FaultContext(ctx, errors.Join(failed, w.attachDomainBacklogs(sweepCtx, workspace)))
}

// askAboutStrandedContactsPerTick bounds the drain, on the same reasoning as
// the meeting holds above: the population is finite and shrinks as it is
// worked, so a small bound costs one probe a tick once it is empty.
const askAboutStrandedContactsPerTick = 200

// askAboutStrandedContacts opens the questions the capture could not.
//
// The ceiling on open questions is per workspace and per domain, and a refusal
// writes nothing. That is deliberate — the question is delayed, not cancelled —
// but the retry rides the next message from that address, and a correspondence
// that has gone quiet never sends one. The contact then stays the mailbox
// owner's for good: invisible to every colleague, their manager and an admin,
// with nothing left to put it back in the queue.
//
// A contact whose question the ceiling refuses again is simply offered again
// next tick. That is the bound doing its job rather than a failure: the queue
// drains, and the room appears.
func (w *linkReconcileWorker) askAboutStrandedContacts(ctx context.Context) (int, error) {
	stranded, err := w.pending.StrandedContacts(ctx, askAboutStrandedContactsPerTick)
	if err != nil {
		return 0, err
	}
	asked := 0
	var failed error
	for _, c := range stranded {
		opened, err := w.pending.AskWhoseRecord(ctx, c)
		if err != nil {
			// One contact's failure is not the sweep's: the rest of the page is
			// still worth asking about, and a joined error still fails the job.
			failed = errors.Join(failed, fmt.Errorf("asking about %s: %w", c.PersonID, err))
			continue
		}
		if opened {
			asked++
		}
	}
	return asked, failed
}

// liftFiledMeetingHoldsPerTick bounds the drain. The population is finite and
// shrinks as it is worked, so a small bound costs one probe a tick once it is
// empty and never delays the repair above it.
const liftFiledMeetingHoldsPerTick = 200

// releaseCalendarMeetingHolds opens the meetings captured before the limiter
// stopped holding them.
//
// A connected calendar is a WORK calendar, so an event on it is workspace
// business and capture no longer holds one at all. Rows captured before that
// still carry the hold, and no later event re-asks the question: the ones with
// no link are not even offered to the drain above, so a recurring internal
// meeting would stay invisible to the workspace for the rest of its life.
//
// Both spellings, because the reason was split at the writer AFTER these rows
// were written: a meeting held before the split carries ReasonNoRecord and one
// held after it carries ReasonNoCounterparty.
//
// ReasonNoRecord is otherwise the JUDGED hold — a suppressed sender, a thread
// judged the owner's private life — and opening one of those would publish a
// mailbox owner's private correspondence to the workspace. Two conditions make
// reading it safe here, and both are required:
//
//   - kind = 'meeting'. A necessary condition, never a sufficient one: the
//     extension ingress copies Kind straight off a third-party unit's record
//     with no vocabulary check in front of it, so the word is not the calendar's
//     to claim.
//   - counterparty_email IS NULL, which is WHY a meeting reached the limiter at
//     all. Attendance is a list, so the mapper names no counterparty, and the
//     ladder concluded "captured, named nobody" without judging anyone. Every
//     judged hold is about a sender, so its row names one, and this is the
//     condition no writer can forge by choosing a string.
//
// The null counterparty alone would admit the address-less mail that reaches the
// same branch. Together they are the shape only a record that named nobody has.
//
// It drains permanently: the release rewrites audience and reason on every row
// it selects, so a row worked once cannot match again.
func (w *linkReconcileWorker) releaseCalendarMeetingHolds(ctx context.Context) (int, error) {
	var held []ids.ActivityID
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT a.id
			  FROM activity a
			 WHERE a.kind = 'meeting'
			   AND a.counterparty_email IS NULL
			   AND a.audience = 'participants'
			   AND a.audience_reason IN ($1, $2)
			   AND a.restricted_at IS NULL
			   AND a.archived_at IS NULL
			   -- Captured, not booked in the app: the in-app meeting writes its
			   -- own audience and leaves no import row, and the recompute
			   -- declines it. Selecting one would return it every tick.
			   AND EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id)
			 ORDER BY a.occurred_at DESC, a.id
			 LIMIT $3`,
			activities.ReasonNoRecord, activities.ReasonNoCounterparty,
			liftFiledMeetingHoldsPerTick)
		if err != nil {
			return fmt.Errorf("selecting the calendar meetings still held: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.ActivityID
			if err := rows.Scan(&id); err != nil {
				return fmt.Errorf("reading a calendar meeting still held: %w", err)
			}
			held = append(held, id)
		}
		return rows.Err()
	}); err != nil {
		return 0, err
	}
	released := 0
	for _, id := range held {
		if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
			return activities.ReleaseCalendarMeetingHoldTx(ctx, tx, id)
		}); err != nil {
			return released, fmt.Errorf("releasing the hold on %s: %w", id, err)
		}
		released++
	}
	return released, nil
}

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
func NewLinkReconcileWorkspaceWorkerForTest(pool *pgxpool.Pool, store *people.Store) *linkReconcileWorker {
	return newLinkReconcileWorker(pool, store, slog.Default())
}

// ReconcileWorkspaceForTest drives ONE workspace's turn from another package.
//
// Exported beside the constructor above and for the same reason: the suite that
// drives this pass lives in compose/integration. It used to reach the work by
// building the child job's args and calling Work, which the collapse removed —
// Work now walks the fleet, and a suite about one tenant would lose the tenant
// it is about (ADR-0103). This is the turn Work calls per workspace.
func (w *linkReconcileWorker) ReconcileWorkspaceForTest(ctx context.Context, workspace ids.UUID) error {
	return w.reconcileLinksForWorkspace(ctx, workspace)
}
