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
	"github.com/margince/margince/backend/internal/compose/weekly/narrative"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// narrateBudget bounds ONE rep's sentence, and it is the deterministic half's
// protection rather than the model's.
//
// A single model call is allowed 300 seconds and the lane has two rungs, so an
// unreachable provider costs ten minutes for one rep — the whole workspace
// job's deadline (api/jobs.yaml). The reps after them would then go unmeasured,
// which inverts the entire design: the counts and the deal lines are the
// review, and the sentence is a remark that must never be able to cost them.
//
// Forty seconds is generous for two sentences over a dozen numbers and short
// enough that a wedged provider costs one rep their remark rather than a team
// their retrospective.
const narrateBudget = 40 * time.Second

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
	// narrator writes the week's sentence. NIL IS THE WIRING: a role with no
	// weekly_review lane still measures every rep's week and still writes it —
	// only the sentence is absent, and the screen says so rather than
	// pretending the week was unremarkable.
	narrator completer
	// mail is the outbound channel, off by omission the same way. An
	// installation with no operator relay measures every week and mails none.
	mail WeeklyMailConfig
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
	// TWO PASSES, and the order is the protection rather than a tidiness.
	//
	// Measuring is database-only and fast; mailing dials a relay that may
	// stall for its whole budget. Interleaved, one unreachable relay spends the
	// workspace's ten minutes on the first dozen reps and every rep after them
	// loses THE REVIEW — not their mail, the counts and the deal lines
	// themselves. And a rep skipped that way may never be measured at all: the
	// candidate query only ever asks about the week that just closed, so a
	// missed week is gone once the next Monday arrives.
	//
	// So every due rep is measured first, and nothing outbound happens until
	// the last of them has their week written.
	var failures []error
	mailable := make([]mailableReview, 0, len(due))
	for _, userID := range due {
		ready, err := w.measureFor(sysCtx, wsID, userID, now)
		if err != nil {
			failures = append(failures, fmt.Errorf("weekly review for user %s: %w", userID, err))
			continue
		}
		if ready.reviewID != ids.Nil {
			mailable = append(mailable, ready)
		}
	}
	// The mail second, on whatever deadline is left. A relay that eats the
	// remainder now costs reps their MESSAGE, which the claim leaves unspent
	// for a later tick to retry — the review itself is already committed.
	for _, ready := range mailable {
		// The rep's authority on the JOB's live context: her principal decides
		// what may be read and sent, the running job decides how long there is
		// to do it and when to stop.
		sendCtx := principal.WithCorrelationID(
			principal.WithActor(ctx, ready.rep), ids.NewV7())
		w.mailWeekly(sendCtx, ready.reviewID, now)
	}
	// The team snapshots THIRD, and last for a reason of its own: each one is a
	// total over member reviews, so a snapshot assembled while reps were still
	// being measured would freeze a team that was half-counted and nothing
	// afterwards would say so. It runs after the mail because it is the least
	// urgent of the three — a lead reads it on Monday, and a tick that ran out
	// of budget here costs a snapshot the next tick rebuilds, not a review.
	//
	// Failures are collected, not returned: one team that cannot be assembled
	// must not report the whole workspace's reviews as failed when they are
	// written and committed.
	failures = append(failures, w.snapshotTeams(sysCtx, wsID, now)...)
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
) (mailableReview, error) {
	rbac, seat, err := w.users.EffectiveAuthority(ctx, wsID, userID)
	if err != nil {
		return mailableReview{}, fmt.Errorf("resolving the rep's authority: %w", err)
	}
	repPrincipal := principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + userID.String(),
		UserID:      userID,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	}
	repCtx := principal.WithActor(ctx, repPrincipal)
	repCtx = principal.WithCorrelationID(repCtx, ids.NewV7())
	review, created, err := w.engine.AssembleFor(repCtx, now)
	if err != nil {
		// A seat whose role grants no deal read has no week to measure, and
		// that is a configuration rather than a fault: failing the job would
		// make one such seat cost the whole workspace its retrospectives, and
		// River would retry into the same refusal every pass.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			w.log.InfoContext(ctx, "no weekly review for a seat whose role does not grant reading deals",
				"user", userID, "workspace", wsID)
			return mailableReview{}, nil
		}
		return mailableReview{}, err
	}
	// The sentence, after the review is committed and never as part of it. A
	// model that is slow, absent or wrong must not be able to cost the rep the
	// counts and the deal lines — those are the review, and this is a remark
	// about them.
	//
	// RETRIED, not written once. A review can land un-narrated for reasons that
	// pass: the role had no lane, the budget was spent, the provider was down,
	// the reply would not parse. Narrating only on the pass that CREATED the
	// row would make every one of those permanent — the dispatcher ticks again
	// within the week, finds the row already there, and never looks at it
	// again. So a later tick narrates a week that has no stamp, and leaves one
	// that has alone: re-narrating would rewrite a sentence the rep has read.
	if review.NarratedAt == nil {
		w.narrate(repCtx, review, now)
	}
	_ = created
	// HANDED BACK rather than mailed here. The caller runs every send after
	// the last rep is measured — see measureWorkspace for why interleaving
	// them costs later reps their review.
	//
	// It carries the id and not the review: mailWeekly re-reads the row inside
	// its own claim, because the narration above wrote to that same row and
	// mailing the in-memory copy would post a week whose sentence had just
	// been written and was not in it.
	return mailableReview{rep: repPrincipal, reviewID: review.ID}, nil
}

// mailableReview is one measured week waiting for its message, with the
// authority it must be claimed and sent under.
//
// It carries the PRINCIPAL and not a context. The mail runs under the rep's own
// authority, like every other read of her data — the enumeration's system
// principal must not be what claims and sends her week — but the deadline and
// the cancellation belong to the job that is still running. Storing the whole
// context would freeze both, so a worker shutting down would go on dialling a
// relay for a job nobody is waiting for.
type mailableReview struct {
	rep      principal.Principal
	reviewID ids.UUID
}

// narrate asks the model for the week's sentence and stores it, or records
// that no sentence was written.
//
// It returns nothing. Every failure here is a week that reads without its
// remark, which is a complete review — so the failure is loud in the log and
// silent to the reader, and never the job's.
func (w *weeklyGenerateWorkspaceWorker) narrate(ctx context.Context, review weekly.Review, now time.Time) {
	if w.narrator == nil {
		// No lane in this role. Not an error and not worth a line every
		// Monday: the deterministic review is the product either way.
		return
	}
	in := narrative.Input{
		WeekStart: review.LocalWeekStart.Format(time.DateOnly),
		Counts: narrative.Counts{
			TasksDue: review.Counts.TasksDue, TasksDone: review.Counts.TasksDone,
			TasksCarriedOver: review.Counts.TasksCarriedOver,
			DealsMoved:       review.Counts.DealsMoved,
			DealsWon:         review.Counts.DealsWon, DealsLost: review.Counts.DealsLost,
			ProposalsAccepted:   review.Counts.ProposalsAccepted,
			ProposalsRejected:   review.Counts.ProposalsRejected,
			BriefItemsActed:     review.Counts.BriefItemsActed,
			BriefItemsDismissed: review.Counts.BriefItemsDismissed,
		},
	}
	for _, line := range review.Deals {
		in.Deals = append(in.Deals, narrative.Deal{
			Label: line.Label, Outcome: line.Outcome, Stage: line.ToStageLabel,
		})
	}

	lang := identity.BaseLanguageForPrompt(ctx, w.pool)
	// Bounded per rep. See narrateBudget: without this one wedged provider
	// spends the whole workspace deadline on the first rep and every rep after
	// them loses the review itself, not just the remark.
	bounded, cancel := context.WithTimeout(ctx, narrateBudget)
	defer cancel()
	reply, err := ai.Ask(bounded, w.narrator, narrative.Request(in, lang), func(text string) error {
		_, err := narrative.Parse(text)
		return err
	})
	if err != nil {
		w.log.WarnContext(ctx, "the weekly review has no sentence: the model call did not land",
			"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
		return
	}
	sentence, err := narrative.Parse(reply.Text)
	if err != nil {
		w.log.WarnContext(ctx, "the weekly review has no sentence: the reply was refused",
			"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
		return
	}
	// Written even when empty. The stamp is what tells the screen a pass ran
	// and found the week unremarkable, which is a different answer from no
	// pass at all.
	// The STORE runs on the caller's context, not the bounded one: the model
	// call is what needs a leash, and a write cancelled by the model's own
	// deadline would lose a sentence the model successfully produced.
	if err := w.engine.Narrate(ctx, review.ID, sentence, now); err != nil {
		w.log.WarnContext(ctx, "the weekly review's sentence could not be stored",
			"week", review.LocalWeekStart.Format(time.DateOnly), "cause", err)
	}
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

// repsWithoutAReviewFor is the candidate query: every seat whose review for the
// closed week is missing or unnarrated.
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
		  -- A rep is due while they have no review for the week, OR have one
		  -- that nobody has narrated. The second arm is the retry: a review
		  -- can land un-narrated because the role had no lane, the budget was
		  -- spent or the provider was down, and without it every one of those
		  -- would be permanent — the next tick finds the row and looks away.
		  --
		  -- AssembleFor is idempotent (uq_weekly_review_user_week), so a rep
		  -- admitted by the second arm re-reads their own review rather than
		  -- writing a second.
		  AND NOT EXISTS (
			SELECT 1 FROM weekly_review wr
			WHERE wr.user_id = u.id AND wr.local_week_start = $1
			  AND wr.narrated_at IS NOT NULL)
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
