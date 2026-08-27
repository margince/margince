// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly retrospective's scheduled pass.
//
// It ticks every six hours rather than once a week, and that is the schedule
// being correct rather than lazy: the Monday it aims at is the INSTALLATION's
// local one, so a tick that crosses the review hour in every zone the setting
// can name is what makes one cadence work everywhere. uq_weekly_review_user_week
// collapses the extra ticks into one review, and the same property backfills a
// Monday the worker was down for.

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
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reviewHour is the local hour on Monday from which a rep's week is measured.
//
// Late enough that the week it closes is genuinely over in the installation's
// own zone, early enough that the review is waiting when the first person
// opens Margince.
const reviewHour = 6

// WeeklyReviewGenerateArgs is the fleet-wide dispatcher.
type WeeklyReviewGenerateArgs struct{}

// Kind names the job in the contract.
func (WeeklyReviewGenerateArgs) Kind() string { return "weekly_review_generate" }

// FleetWide marks this as an enumerator that does no tenant work itself.
func (WeeklyReviewGenerateArgs) FleetWide() {}

type weeklyGenerateWorker struct{ pool *pgxpool.Pool }

func (w *weeklyGenerateWorker) Work(ctx context.Context, _ *river.Job[WeeklyReviewGenerateArgs]) error {
	return jobs.FaultContext(ctx, dispatchPerWorkspace(ctx, w.pool,
		workspaceSweepOpts(WeeklyReviewGenerateWorkspaceArgs{}.Kind()),
		func(ws ids.UUID) river.JobArgs {
			return WeeklyReviewGenerateWorkspaceArgs{Workspace: ws}
		}))
}

// WeeklyReviewGenerateWorkspaceArgs measures one workspace's reps.
type WeeklyReviewGenerateWorkspaceArgs struct {
	Workspace ids.UUID `json:"workspace_id"`
}

// Kind names the job in the contract.
func (WeeklyReviewGenerateWorkspaceArgs) Kind() string { return "weekly_review_generate_workspace" }

// WorkspaceID binds the pass to its tenant.
func (a WeeklyReviewGenerateWorkspaceArgs) WorkspaceID() ids.UUID { return a.Workspace }

type weeklyGenerateWorkspaceWorker struct {
	engine *weekly.Engine
	pool   *pgxpool.Pool
	users  *identity.Service
	now    func() time.Time
	log    *slog.Logger
}

func (w *weeklyGenerateWorkspaceWorker) Work(
	ctx context.Context, job *river.Job[WeeklyReviewGenerateWorkspaceArgs],
) error {
	wsCtx, err := workspaceJobCtx(ctx, job.Args)
	if err != nil {
		return jobs.FaultContext(ctx, err)
	}
	clock := w.now
	if clock == nil {
		clock = time.Now
	}
	return jobs.FaultContext(ctx, w.measureWorkspace(wsCtx, job.Args.Workspace, clock().UTC()))
}

// measureWorkspace writes a review for every rep whose local Monday has
// arrived, and reports what it could not do.
//
// One rep's failure does not cost the others theirs: the loop records the
// error and carries on, then fails the job with all of them joined. A pass that
// stopped at the first would leave a whole team without a retrospective because
// one seat had a broken row, and River's retry would keep hitting that seat
// first.
func (w *weeklyGenerateWorkspaceWorker) measureWorkspace(
	ctx context.Context, wsID ids.UUID, now time.Time,
) error {
	// The enumeration runs as the system: it reads the installation timezone
	// and the seat roster — facts about WHO is due a review, which belong to no
	// rep. Every read of a rep's own data happens under her own principal.
	sysCtx := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:weekly-review",
	})
	due, err := w.repsDueTheirReview(sysCtx, wsID, now)
	if err != nil {
		return err
	}
	var failures []error
	for _, userID := range due {
		if err := w.measureFor(sysCtx, wsID, userID, now); err != nil {
			failures = append(failures, fmt.Errorf("weekly review for user %s: %w", userID, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("weekly_review_generate_workspace: %d of %d reps: %w",
			len(failures), len(due), errors.Join(failures...))
	}
	return nil
}

// measureFor writes one rep's week under that rep's OWN authority.
//
// The principal is the whole point: the review counts what SHE did and what she
// could see, so every read inside AssembleFor resolves through her grants, her
// teams and her seat. EffectiveAuthority reads them as one snapshot — composed
// from separate reads they can describe an authority she never held.
func (w *weeklyGenerateWorkspaceWorker) measureFor(
	ctx context.Context, wsID, userID ids.UUID, now time.Time,
) error {
	rbac, seat, err := w.users.EffectiveAuthority(ctx, wsID, userID)
	if err != nil {
		return fmt.Errorf("resolving the rep's authority: %w", err)
	}
	repCtx := principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + userID.String(),
		UserID:      userID,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	repCtx = principal.WithCorrelationID(repCtx, ids.NewV7())
	if _, _, err := w.engine.AssembleFor(repCtx, now); err != nil {
		// A seat whose role grants no deal read has no week to measure, and
		// that is a configuration rather than a fault: failing the job would
		// make one such seat cost the whole workspace its retrospectives, and
		// River would retry into the same refusal every pass.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			w.log.InfoContext(ctx, "no weekly review for a seat whose role does not grant reading deals",
				"user", userID, "workspace", wsID)
			return nil
		}
		return err
	}
	return nil
}

// repsDueTheirReview lists the workspace's active full-seat humans whose local
// Monday has reached reviewHour and who hold no review for the closed week.
//
// Overlay workspaces are refused outright: the review counts native deal rows,
// which an overlay workspace keeps in the incumbent, so a pass there would
// measure a week of zeroes — and "a quiet week" and "this cannot be answered
// here" read identically while only one is true.
func (w *weeklyGenerateWorkspaceWorker) repsDueTheirReview(
	ctx context.Context, wsID ids.UUID, now time.Time,
) ([]ids.UUID, error) {
	var due []ids.UUID
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		overlay, err := overlayModeOf(ctx, tx)
		if err != nil {
			return fmt.Errorf("resolving the workspace's system-of-record mode: %w", err)
		}
		if overlay {
			w.log.InfoContext(ctx, "weekly review skipped: the workspace keeps its deals in the incumbent",
				"workspace", wsID)
			return nil
		}
		// The engine's own week arithmetic, not a second copy: this decides who
		// is due and the store then dates what it writes. Two spellings would
		// let the pair disagree about which week a review belongs to.
		thisWeek, err := weekly.WeekStartOf(ctx, tx, now)
		if err != nil {
			return err
		}
		_, local, err := briefs.LocalDayAt(ctx, tx, now)
		if err != nil {
			return err
		}
		// From Monday's review hour onward, ANY day of the week — not Monday
		// alone.
		//
		// A Monday-only guard loses a week outright: if the worker is down
		// that day, the next Monday measures the NEW closed week and the
		// missed one is never written. Nothing would report it, because the
		// anti-join only ever asks about the current week. Letting the rest of
		// the week backfill is what makes the schedule self-healing, and the
		// per-week uniqueness is what stops it writing twice.
		if local.Weekday() == time.Sunday || local.Hour() < reviewHour {
			return nil
		}
		due, err = repsWithoutAReviewFor(ctx, tx, thisWeek.AddDate(0, 0, -7))
		return err
	})
	return due, err
}

// repsWithoutAReviewFor is the candidate query: every seat that should have a
// review for the closed week and does not yet.
//
// The anti-join is an optimisation, not the correctness —
// uq_weekly_review_user_week is what makes a second review impossible, and a
// rep who gains one between this read and the write simply joins it.
func repsWithoutAReviewFor(ctx context.Context, tx pgx.Tx, week time.Time) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id
		FROM app_user u
		WHERE `+identity.LiveMemberSQL("u")+`
		  AND u.is_agent = false
		  AND u.seat_type = 'full'
		  AND NOT EXISTS (
			SELECT 1 FROM weekly_review wr
			WHERE wr.user_id = u.id AND wr.local_week_start = $1)
		ORDER BY u.id`, week)
	if err != nil {
		return nil, err
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
