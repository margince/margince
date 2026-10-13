// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// defaultScheduleGrace bounds how late a scheduled message may still fire.
//
// The rep chose a MOMENT, and that choice carries meaning a late send
// destroys: mail timed for Monday 09:00 is wrong mail at 18:00. Past this, the
// message is held for a human rather than sent (ADR-0104 §6).
const defaultScheduleGrace = time.Hour

// scheduledSendMaxAttempts bounds the timer's ladder. Its last rung holds the
// message, so an infrastructure failure surfaces as a message waiting for a
// human rather than a row nothing will ever wake again.
const scheduledSendMaxAttempts = 5

// ScheduledSendArgs wakes ONE message a rep chose to send later. It carries the
// row's id and nothing else: the row is the schedule, and everything this job
// needs to decide — whether it is still wanted, when it is due, who it fires as
// — is read from the row at wake time, never frozen into the alarm.
type ScheduledSendArgs struct {
	Workspace       ids.UUID `json:"workspace_id"`
	ScheduledSendID string   `json:"scheduled_send_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (ScheduledSendArgs) Kind() string { return "comms_scheduled_send" }

// WorkspaceID binds this alarm to its tenant (jobs.WorkspaceScoped).
func (a ScheduledSendArgs) WorkspaceID() ids.UUID { return a.Workspace }

// WithScheduleTimer wires the alarm a deferred send is accepted against.
//
// Without it a scheduling request refuses: accepting "send it Monday" with
// nothing to wake the message is a promise this surface cannot keep, and the
// rep would only discover it on Monday.
func WithScheduleTimer(timer activities.ScheduleTimer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.send.ScheduleTimer = timer
		s.rebuildToolRegistry(pool)
	}
}

// scheduleTimer enqueues the alarm that wakes one scheduled message.
type scheduleTimer struct {
	runner *jobs.Runner
}

// NewScheduleTimer builds the seam the send path defers through.
func NewScheduleTimer(runner *jobs.Runner) scheduleTimer {
	return scheduleTimer{runner: runner}
}

// ScheduleTx enqueues the alarm in the caller's transaction, so the row and its
// timer commit together or not at all.
func (t scheduleTimer) ScheduleTx(ctx context.Context, tx pgx.Tx, id ids.UUID, due time.Time) error {
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return errors.New("compose: scheduling a send outside workspace context")
	}
	return t.runner.EnqueueTx(ctx, tx, ScheduledSendArgs{
		Workspace:       ws,
		ScheduledSendID: id.String(),
	}, &river.InsertOpts{
		ScheduledAt: due,
		MaxAttempts: scheduledSendMaxAttempts,
	})
}

// scheduledSendWorker fires one scheduled message, or holds it.
//
// The job is a dumb alarm and the ROW is the schedule. This wakes, re-reads the
// row, and does nothing if the message was cancelled, snoozes if its moment
// moved later, and otherwise fires it through the one send path — under the
// scheduling human's freshly re-derived authority, never the one they had when
// they pressed the button.
type scheduledSendWorker struct {
	pool     *pgxpool.Pool
	store    *activities.Store
	actors   storedActor
	consent  activities.ConsentGate
	delivery activities.DeliveryStager
	grace    time.Duration
	now      func() time.Time
}

// newScheduledSendWorker assembles the worker that fires deferred messages.
//
// It builds its store through sendStore — the SAME assembly the tool surface
// sends through — so a scheduled message is rendered by the machinery an
// immediate one is: the signature, the sender name, the unsubscribe linker and
// the draft-outcome recorder all present, all identical. A hand-built store
// here would be a second send path wearing the first one's name.
func newScheduledSendWorker(pool *pgxpool.Pool, delivery DeliveryMachinery, blob blobstore.Store, origin SendOrigin) *scheduledSendWorker {
	return &scheduledSendWorker{
		pool: pool,
		// The ORIGIN travels, because a fired message builds its unsubscribe
		// link from it. An empty send path here refuses every scheduled
		// correspondence and marketing send at fire time while the
		// transactional ones beside it go out — which is what it did, and what
		// nothing noticed, because every fixture scheduled a transactional one.
		store:   sendStore(pool, origin.sendPath()).WithBlobstore(blob),
		actors:  storedActor{authority: identity.NewService(pool)},
		consent: consentGateFor(pool),
		// The SAME machinery every other send stages with, handed in rather
		// than built here: firing enqueues a delivery job, and a second
		// construction site would be a second way for the two to disagree.
		delivery: delivery,
		grace:    defaultScheduleGrace,
		now:      time.Now,
	}
}

func (w *scheduledSendWorker) Work(ctx context.Context, job *river.Job[ScheduledSendArgs]) error {
	id, err := ids.Parse(job.Args.ScheduledSendID)
	if err != nil {
		return jobs.FaultContext(ctx, fmt.Errorf("comms_scheduled_send: scheduled send id: %w", err))
	}
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}

	// Who this message fires as. Read under a system scope, because resolving
	// the scheduler is what tells us whose authority to adopt — the answer
	// cannot come from the authority we are still deciding.
	sched, err := w.scheduler(sendWorkerScope(wsCtx), id)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if sched.UserID == (ids.UUID{}) {
		// The row is gone, cancelled, or already finished. The alarm rang for
		// a message that no longer wants sending, which is the ordinary way a
		// cancelled send stops.
		return nil
	}
	scheduler := sched.UserID

	fireCtx, refused, err := w.fireAs(wsCtx, sched)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	if refused != "" {
		// The authority this message needs is gone — the scheduler's seat, or
		// the agent passport it was scheduled under. Holding under a system
		// scope, because the authority we would otherwise hold under is exactly
		// the one that just failed to resolve, and naming the reason fireAs
		// found rather than one blanket "sender_inactive": a rep sent to check
		// their account when their passport was revoked looks in the wrong place.
		// No observation is possible: this refuses before the fire path ever
		// claims the row, so the hold's own claim is the first read of it.
		if err := w.hold(w.holdScope(wsCtx, scheduler), id, refused, activities.UnobservedVersion); err != nil {
			return jobs.FaultContext(ctx, err)
		}
		return nil
	}

	outcome, err := w.store.FireScheduledSend(fireCtx, id, w.grace, w.consent, w.delivery)
	if err != nil {
		if job.Attempt >= scheduledSendMaxAttempts {
			// Last rung. A row left 'scheduled' with no live timer is a message
			// nobody will ever see fail, so the ladder ends by handing it to a
			// human instead of going quiet.
			// Bound to the version that attempt claimed under: a rep who
			// rescheduled after the rollback has made a newer decision, and
			// this ladder's verdict is about the row before theirs. An attempt
			// that died before claiming reports zero, which matches no row and
			// so declines the hold — the right answer, because a verdict from
			// an attempt that never read the row is about nothing.
			if holdErr := w.hold(w.holdScope(wsCtx, scheduler), id, activities.HeldTimerExhausted, outcome.Observed); holdErr != nil {
				return jobs.FaultContext(ctx, errors.Join(err, holdErr))
			}
			return nil
		}
		return jobs.FaultContext(ctx, fmt.Errorf("comms_scheduled_send: firing %s: %w", id, err))
	}
	if !outcome.Due.IsZero() {
		// Its moment moved later. Come back then; the row remains the schedule.
		return river.JobSnooze(time.Until(outcome.Due))
	}
	return nil
}

// schedulerOf is who a pending scheduled message fires as: the authorizing
// human, the kind of principal that scheduled it, and — when that was an
// agent — the agent's own identity as core 0260 recorded it.
type schedulerOf struct {
	UserID ids.UUID
	Kind   string
	// AgentActorID is the acting agent's principal id. Empty for a human, and
	// empty for an agent row scheduled before 0260 existed to record one.
	AgentActorID    string
	AgentPassport   ids.UUID
	AgentOnBehalfOf ids.UUID
}

// scheduler reads who a pending scheduled message fires as. A zero UserID means
// there is nothing to fire.
func (w *scheduledSendWorker) scheduler(ctx context.Context, id ids.UUID) (schedulerOf, error) {
	var (
		out      schedulerOf
		actorID  *string
		passport *ids.UUID
		behalf   *ids.UUID
	)
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT scheduled_by, principal_kind,
			       agent_actor_id, agent_passport_id, agent_on_behalf_of
			  FROM scheduled_send
			 WHERE id = $1 AND status = 'scheduled'`, id).
			Scan(&out.UserID, &out.Kind, &actorID, &passport, &behalf)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return schedulerOf{}, nil
	}
	if err != nil {
		return schedulerOf{}, fmt.Errorf("comms_scheduled_send: reading who scheduled %s: %w", id, err)
	}
	if actorID != nil {
		out.AgentActorID = *actorID
	}
	if passport != nil {
		out.AgentPassport = *passport
	}
	if behalf != nil {
		out.AgentOnBehalfOf = *behalf
	}
	return out, nil
}

// fireAs rebuilds, live, the authority this message fires under, and names the
// hold reason when that authority is gone.
//
// The principal kind is kept (ADR-0104). The send path leaves out a human's
// sign-off when an agent acts. Fired as a human, an agent's message would carry
// the approver's signature, which the same immediate send never does.
func (w *scheduledSendWorker) fireAs(ctx context.Context, sched schedulerOf) (context.Context, string, error) {
	actor, err := w.actors.rebuild(ctx, sched.storedActor())
	switch {
	case errors.Is(err, errActorSeatGone):
		// A read seat may not transmit, and the dispatcher would refuse it too.
		// Holding here tells the rep that their seat is the reason.
		return nil, activities.HeldSenderInactive, nil
	case errors.Is(err, errActorPassportRevoked):
		return nil, activities.HeldPassportRevoked, nil
	case err != nil:
		return nil, "", fmt.Errorf("comms_scheduled_send: rebuilding who fires %s's message: %w", sched.UserID, err)
	}
	return principal.WithCorrelationID(principal.WithActor(ctx, actor), ids.NewV7()), "", nil
}

// storedActor is the scheduling principal as core 0260 recorded it. A row from
// before 0260 names no agent, and the rebuild refuses it rather than inventing one.
func (s schedulerOf) storedActor() migration.StoredActor {
	stored := migration.StoredActor{Type: principal.PrincipalHuman, UserID: s.UserID}
	if s.Kind == "agent" {
		stored.Type = principal.PrincipalAgent
		stored.ID = s.AgentActorID
		stored.PassportID = s.AgentPassport
		stored.OnBehalfOf = s.AgentOnBehalfOf
	}
	return stored
}

// holdScope is the system scope a hold runs under, naming the rep it acts FOR.
//
// The system is the actor — this is the alarm doing what a human asked for
// earlier, not that human acting again — but the message is one rep's, and an
// audit trail that recorded only "the system held something" would not say
// whose message stopped. Surfacing a held message TO that rep is #1312; this is
// the provenance any such surface will read.
func (w *scheduledSendWorker) holdScope(wsCtx context.Context, scheduler ids.UUID) context.Context {
	ctx := sendWorkerScope(wsCtx)
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ctx
	}
	actor.OnBehalfOf = scheduler
	return principal.WithActor(ctx, actor)
}

// hold hands a message to a human with the reason they need.
func (w *scheduledSendWorker) hold(ctx context.Context, id ids.UUID, reason string, observed int64) error {
	if err := w.store.HoldScheduledSend(ctx, id, reason, observed); err != nil {
		return fmt.Errorf("comms_scheduled_send: holding %s: %w", id, err)
	}
	return nil
}
