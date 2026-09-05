// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The overnight Morning-Brief assembly: an hourly dispatcher fanning out one
// pass per workspace, each of which snapshots a run for every full-seat rep
// whose local morning has arrived and who has no run for that day yet.
//
// Hourly rather than once at a fixed UTC hour, because the morning being aimed
// at is the installation's local one: an hourly tick crosses briefingHour in
// every zone the setting can name, and the per-rep day uniqueness is what keeps
// twenty-four ticks from producing twenty-four runs. The same property makes a
// missed night self-healing — a worker down until 09:00 assembles the morning
// on its next tick rather than skipping the day.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// briefingHour is the local hour from which a rep's brief is expected to be
// waiting. Early enough that the first coffee finds it already there, late
// enough that "overnight" covers a real night's activity.
const briefingHour = 5

// BriefGenerateArgs runs one overnight brief-assembly pass.
type BriefGenerateArgs struct{}

// Kind is the stable job identifier River persists in river_job.
func (BriefGenerateArgs) Kind() string { return "brief_generate" }

// FleetWide marks this a dispatcher: it enumerates and enqueues,
// and does no tenant work of its own (jobs.FleetWide).
func (BriefGenerateArgs) FleetWide() {}

// briefGenerateWorker is the dispatcher for the overnight assembly.
type briefGenerateWorker struct {
	pool *pgxpool.Pool
}

func (w *briefGenerateWorker) Work(ctx context.Context, _ *river.Job[BriefGenerateArgs]) error {
	return jobs.FaultContext(ctx, dispatchPerWorkspace(ctx, w.pool,
		workspaceSweepOpts(BriefGenerateWorkspaceArgs{}.Kind()),
		func(ws ids.UUID) river.JobArgs { return BriefGenerateWorkspaceArgs{Workspace: ws} }))
}

// BriefGenerateWorkspaceArgs assembles one workspace's morning briefs.
type BriefGenerateWorkspaceArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
}

// Kind is the stable job identifier River persists in river_job.
func (BriefGenerateWorkspaceArgs) Kind() string { return "brief_generate_workspace" }

// WorkspaceID binds this pass to its tenant (jobs.WorkspaceScoped).
func (a BriefGenerateWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }

// briefGenerateWorkspaceWorker assembles one workspace's briefs.
//
// The engine here carries no L2 ranker: the worker role holds no brief_ranking
// model lane, so the overnight run is the deterministic §10.1 composite order.
// That is the floor the whole feature is built on rather than a degradation —
// the model re-order stays what the api role adds when a rep refreshes — and a
// run assembled without it is still a complete, ranked, cited brief.
type briefGenerateWorkspaceWorker struct {
	engine *briefs.BriefEngine
	pool   *pgxpool.Pool
	users  *identity.Service
	log    *slog.Logger
	// mail is the outbound channel, off by omission: a nil Mailer mails no
	// briefs, which is what an installation with no operator relay wants.
	mail BriefMailConfig
	// now is the injected clock (nil = wall clock). The morning is read at
	// execution time, not enqueue time: a job that waited in the queue past
	// midnight must assemble the day it actually runs in, which is the day the
	// rep will open.
	now func() time.Time
}

func (w *briefGenerateWorkspaceWorker) Work(ctx context.Context, job *river.Job[BriefGenerateWorkspaceArgs]) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	clock := w.now
	if clock == nil {
		clock = time.Now
	}
	return jobs.FaultContext(ctx, w.assembleWorkspace(wsCtx, job.Args.Workspace, clock().UTC()))
}

// assembleWorkspace snapshots a run for every rep in this workspace whose
// local morning has arrived, and reports what it could not do.
//
// One rep's failure does not cost the others theirs: the loop records the
// error and carries on, then fails the job with all of them joined. A pass
// that stopped at the first failure would leave a whole team without briefs
// because one seat had a broken authority row, and River's retry would keep
// hitting the same seat first.
func (w *briefGenerateWorkspaceWorker) assembleWorkspace(ctx context.Context, wsID ids.UUID, now time.Time) error {
	// The enumeration runs as the system: it reads the workspace's mode, the
	// installation timezone and the seat roster — installation-level facts about
	// WHO is due a brief, which belong to no rep. Every read of a rep's own data
	// happens under her own principal in assembleFor.
	sysCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:brief-overnight",
	})
	pass, err := w.morningPassFor(sysCtx, wsID, now)
	if err != nil {
		return err
	}
	if !pass.due {
		return nil
	}
	var failures []error
	for _, userID := range pass.reps {
		if err := w.assembleFor(sysCtx, wsID, userID, now); err != nil {
			failures = append(failures, fmt.Errorf("brief for user %s: %w", userID, err))
		}
	}
	// THE MAIL AFTER THE WHOLE ASSEMBLY, and as its own pass over whatever is
	// still unmailed rather than a step inside assembleFor.
	//
	// A run is assembled once and may be mailed hours later: `delivery_hour_local`
	// holds a rep's message back until their chosen hour, and the candidate query
	// above lists only reps who hold NO run for the day — so a rep whose run was
	// written at the briefing hour has left that list by the time their own hour
	// comes round. Mailing from inside the assembly would give them one chance,
	// at the wrong hour, and then never look again.
	//
	// Safe to revisit a rep the assembly just served: the claim is what makes a
	// second message impossible, not the shape of this loop.
	if err := w.mailTheMorning(sysCtx, wsID, pass, now); err != nil {
		failures = append(failures, err)
	}
	if len(failures) > 0 {
		return fmt.Errorf("brief_generate_workspace: %d failure(s) over %d rep(s): %w",
			len(failures), len(pass.reps), errors.Join(failures...))
	}
	return nil
}

// morningPass is what one workspace tick resolved before it did anything: which
// local day this is, what hour it is there, and which reps still need a run.
type morningPass struct {
	// due is false when this tick is not the workspace's morning at all — an
	// overlay workspace, or an hour before the briefing hour. Nothing follows.
	due bool
	// day is the installation's local date, which every run is filed under.
	day time.Time
	// hour is the local wall-clock hour, which is what a rep's own delivery
	// hour is compared against.
	hour int
	reps []ids.UUID
}

// mailTheMorning hands every unmailed run of this local day to the mail lane,
// each under its own rep's principal.
//
// Read AFTER the assembly above so the runs it just wrote are included, and in
// its own transaction for the same reason: the candidate read happened before
// those runs existed.
func (w *briefGenerateWorkspaceWorker) mailTheMorning(
	ctx context.Context, wsID ids.UUID, pass morningPass, now time.Time,
) error {
	var awaiting []briefs.BriefRun
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		awaiting, err = briefs.RunsAwaitingMail(ctx, tx, pass.day)
		return err
	}); err != nil {
		return fmt.Errorf("listing the runs still owed a message: %w", err)
	}
	for _, run := range awaiting {
		repCtx, err := w.repContext(ctx, wsID, run.UserID)
		if err != nil {
			// The rep's authority is what the claim and the preference read run
			// under. A seat that cannot be resolved is one this pass cannot mail
			// for, and it must not take the others down with it.
			w.log.WarnContext(ctx, "the morning brief was not mailed: the rep's authority did not resolve",
				"user", run.UserID, "cause", err)
			continue
		}
		w.mailMorning(repCtx, run, now, pass.hour)
	}
	return nil
}

// repContext binds one rep's own authority, which every read and write on their
// behalf runs under.
//
// A fresh correlation id per rep, so one morning's work for one person is one
// recoverable trace in the audit spine rather than a fleet pass nobody can take
// apart.
func (w *briefGenerateWorkspaceWorker) repContext(
	ctx context.Context, wsID, userID ids.UUID,
) (context.Context, error) {
	rbac, seat, err := w.users.EffectiveAuthority(ctx, wsID, userID)
	if err != nil {
		return nil, fmt.Errorf("resolving the rep's authority: %w", err)
	}
	repCtx := principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + userID.String(),
		UserID:      userID,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	return principal.WithCorrelationID(repCtx, ids.NewV7()), nil
}

// assembleFor snapshots one rep's run under that rep's OWN authority.
//
// The principal is the whole security argument for this job: the run is
// assembled by a system process but it must contain exactly what the rep
// herself could see, so every read inside SnapshotRun resolves through her
// grants, her teams and her seat. EffectiveAuthority reads the grants and the
// seat as one snapshot — composed from separate reads they can describe an
// authority she never held, permissions from before a role change with a seat
// from after.
//
// The MAIL is not here. It is a pass of its own over whatever is still unmailed
// once the whole workspace is assembled — mailTheMorning says why, and the short
// version is that a run may be mailed hours after it is written.
func (w *briefGenerateWorkspaceWorker) assembleFor(ctx context.Context, wsID, userID ids.UUID, now time.Time) error {
	repCtx, err := w.repContext(ctx, wsID, userID)
	if err != nil {
		return err
	}
	// The run itself is not read back here. SnapshotRun writes it, and the
	// mailing pass reads whatever is still unmailed for the day — including
	// this one.
	_, err = w.engine.SnapshotRun(repCtx, now)
	if err != nil {
		// A seat whose role grants no deal read has no brief to assemble, and
		// that is a configuration rather than a fault: refusing it as a job
		// failure would make one such seat fail the whole workspace's morning,
		// and River would retry the pass into the same refusal every hour.
		// The rep is left without a brief, which is the honest answer, and the
		// line says which seat so an admin can grant the role if it was a
		// mistake.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			w.log.InfoContext(ctx, "no overnight brief for a seat whose role does not grant reading deals",
				"user", userID, "workspace", wsID)
			return nil
		}
		return err
	}
	return nil
}

// repsDueTheirMorning lists the workspace's active full-seat humans whose local
// day has reached briefingHour and who hold no run for that day.
//
// Refusing an overlay workspace outright rather than returning nobody: the
// brief ranks native deal rows, which an overlay workspace keeps in the
// incumbent, so a pass there would assemble an empty queue — and "nothing needs
// your attention today" and "this cannot be answered here" read identically on
// the screen while only one of them is true.
//
// Agents are excluded: a brief is a person's morning, and an agent seat has no
// morning to prepare. Read seats are excluded because the brief's whole content
// is deals to act on.
func (w *briefGenerateWorkspaceWorker) morningPassFor(ctx context.Context, wsID ids.UUID, now time.Time) (morningPass, error) {
	var pass morningPass
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		overlay, err := overlayModeOf(ctx, tx)
		if err != nil {
			return fmt.Errorf("resolving the workspace's system-of-record mode: %w", err)
		}
		if overlay {
			w.log.InfoContext(ctx, "overnight brief skipped: the workspace keeps its deals in the incumbent",
				"workspace", wsID)
			return nil
		}
		// The engine's own day arithmetic, not a second copy of it: this decides
		// which reps are due, and the store then dates the run it writes. Two
		// spellings would let the pair disagree about which morning a run belongs
		// to, which is how a rep gets a brief dated to a day she has not lived.
		day, local, err := briefs.LocalDayAt(ctx, tx, now)
		if err != nil {
			return err
		}
		if local.Hour() < briefingHour {
			return nil
		}
		pass = morningPass{due: true, day: day, hour: local.Hour()}
		pass.reps, err = repsWithoutARunFor(ctx, tx, day)
		return err
	})
	return pass, err
}

// repsWithoutARunFor is the candidate query: every seat that should have a
// brief for this local day and does not yet.
//
// The anti-join is an optimisation of the morning, not the correctness of it —
// uq_brief_run_user_day is what makes a second run impossible, and a rep who
// gains a run between this read and the snapshot simply joins it.
func repsWithoutARunFor(ctx context.Context, tx pgx.Tx, day time.Time) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id
		FROM app_user u
		WHERE `+identity.LiveMemberSQL("u")+`
		  AND u.is_agent = false
		  AND u.seat_type = 'full'
		  AND NOT EXISTS (
			SELECT 1 FROM brief_run br
			WHERE br.user_id = u.id AND br.local_day = $1)
		ORDER BY u.id`, day)
	if err != nil {
		return nil, fmt.Errorf("listing the reps due a brief: %w", err)
	}
	defer rows.Close()
	var due []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		due = append(due, id)
	}
	return due, rows.Err()
}
