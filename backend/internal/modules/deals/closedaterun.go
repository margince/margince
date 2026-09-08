// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The close-date sweep's receipt: which deals one pass was going to check, how
// far it got, and what happened to each of them.
//
// The sweep used to keep nothing. Its candidate query took the oldest 200
// eligible deals every night, and because a corrected deal stays eligible (the
// sweep leaves it provisional, which is one of the conditions that admits it),
// the same 200 re-qualified each pass. Deal 201 was never reached, and nothing
// recorded that it had not been.
//
// So membership is frozen at the start of a pass and walked by keyset from a
// durable cursor. The freeze is what makes the counts mean something: eligible
// is a set that was decided once, checked is how much of THAT set reached a
// terminal outcome, and a deal that closes mid-pass is recorded as skipped
// rather than quietly leaving the denominator. The durable cursor is what makes
// the 5-minute worker timeout survivable — a pass that runs out of time resumes
// where it stopped instead of starting again at the first row.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Run statuses. A run is running until it either exhausts its frozen set
// (complete) or stops before doing so (incomplete) — and "incomplete" is a
// result the receipt shows, not an error to swallow.
const (
	CloseDateRunRunning    = "running"
	CloseDateRunComplete   = "complete"
	CloseDateRunIncomplete = "incomplete"
)

// Member outcomes. Every frozen member ends on exactly one of these, which is
// what lets checked/eligible reconcile without consulting the live deal table.
const (
	closeDateMemberChecked = "checked"
	closeDateMemberChanged = "changed"
	closeDateMemberStaged  = "staged"
	closeDateMemberSkipped = "skipped"
	closeDateMemberFailed  = "failed"
)

// CloseDateRun is one pass's coverage, as a reader sees it.
type CloseDateRun struct {
	ID        ids.UUID
	AsOf      time.Time
	Status    string
	Eligible  int
	Checked   int
	Corrected int
	Staged    int
	StartedAt time.Time
}

// Remaining is the part of the frozen set this pass has not settled. Derived
// rather than stored: a third stored number is a number that can disagree.
func (r CloseDateRun) Remaining() int { return r.Eligible - r.Checked }

// openRun finds tonight's unfinished pass, so a retry after the worker deadline
// continues it rather than opening a second one over the same deals.
//
// Matched on as_of rather than on "started today": a pass that begins at 23:58
// and is retried at 00:03 is the same pass, and judging it against a new day
// would re-freeze a different set halfway through.
// Returns apperrors.ErrNotFound when there is no such pass, which is the
// ordinary case on a first run of the day rather than a fault: the caller reads
// it as "open a new one".
func (c *CloseDateCorrector) openRun(ctx context.Context, tx pgx.Tx, asOf time.Time) (*CloseDateRun, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, err
	}
	var run CloseDateRun
	err := tx.QueryRow(ctx, `
		SELECT id, as_of, status, eligible, checked, corrected, staged, started_at
		  FROM close_date_run
		 WHERE status = $1 AND as_of = $2
		 ORDER BY started_at DESC
		 LIMIT 1`, CloseDateRunRunning, asOf).
		Scan(&run.ID, &run.AsOf, &run.Status, &run.Eligible, &run.Checked,
			&run.Corrected, &run.Staged, &run.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("close-date: looking for an unfinished pass: %w", err)
	}
	return &run, nil
}

// startRun opens a pass and freezes the deals it will check.
//
// The membership INSERT ... SELECT is the freeze, and it runs in the caller's
// transaction so the set cannot shift between being counted and being recorded.
// Its predicate is the sweep's pre-filter plus one clause — created_at <= the
// run's start — which is the whole of "a deal created tonight belongs to the
// next pass". Without it a deal arriving mid-pass would join a set that has
// already been counted, and eligible would stop matching its own members.
func (c *CloseDateCorrector) startRun(
	ctx context.Context, tx pgx.Tx, asOf time.Time, tzName string,
) (*CloseDateRun, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return nil, err
	}
	var run CloseDateRun
	if err := tx.QueryRow(ctx, `
		INSERT INTO close_date_run (as_of, status, captured_by)
		VALUES ($1, $2, $3)
		RETURNING id, as_of, status, eligible, checked, corrected, staged, started_at`,
		asOf, CloseDateRunRunning, capturedBy).
		Scan(&run.ID, &run.AsOf, &run.Status, &run.Eligible, &run.Checked,
			&run.Corrected, &run.Staged, &run.StartedAt); err != nil {
		return nil, fmt.Errorf("close-date: opening the pass: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO close_date_run_member (run_id, deal_id, deal_created_at)
		SELECT $1, d.id, d.created_at
		  FROM deal d
		 WHERE d.status = 'open' AND d.archived_at IS NULL
		   AND d.created_at <= $2
		   AND (d.expected_close_date IS NULL
		        OR d.expected_close_date <= (timezone($3, now()))::date + $4::int
		        OR d.close_date_provisional)`,
		run.ID, run.StartedAt, tzName, StalledThresholdDays)
	if err != nil {
		return nil, fmt.Errorf("close-date: freezing the eligible set: %w", err)
	}
	run.Eligible = int(tag.RowsAffected())
	if _, err := tx.Exec(ctx,
		`UPDATE close_date_run SET eligible = $2, updated_at = now() WHERE id = $1`,
		run.ID, run.Eligible); err != nil {
		return nil, fmt.Errorf("close-date: recording the frozen size: %w", err)
	}
	return &run, nil
}

// nextMembers reads the next page of unsettled members in the frozen order,
// joined to the deal facts the assessment needs.
//
// The join is to the LIVE deal row on purpose: membership is frozen, the facts
// are not. A deal whose date moved since the freeze must be assessed on what it
// says now, and one that closed or was archived comes back missing — the caller
// settles those as skipped rather than correcting a deal that is no longer open.
func (c *CloseDateCorrector) nextMembers(
	ctx context.Context, tx pgx.Tx, runID ids.UUID, limit int,
) ([]closeDateCandidate, []closeDateMember, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, nil, err
	}
	// PENDING IS THE CURSOR. The walk asks for unsettled members in frozen
	// order and nothing else — no keyset predicate against the run's recorded
	// position.
	//
	// A keyset would be a second answer to "where are we", and the two can
	// disagree: a member reopened behind the cursor (a retry, a repair) is
	// pending but sorts before the recorded position, so a keyset walk would
	// step over it and the pass would finish with work still owed. Settling a
	// member is what advances the walk, because settling is what removes it
	// from this result. The stored cursor stays as the run's reported progress
	// — a number for a reader, not a predicate for the query.
	rows, err := tx.Query(ctx, `
		SELECT m.deal_id, m.deal_created_at,
		       d.id, d.name, d.created_at, d.last_activity_at, d.wait_until,
		       d.expected_close_date, d.close_date_provisional, d.forecast_category,
		       d.pipeline_id, s.win_probability,
		       (SELECT count(*) FROM stage s2
		         WHERE s2.pipeline_id = d.pipeline_id AND s2.archived_at IS NULL
		           AND s2.semantic = 'open' AND s2.position >= s.position)
		  FROM close_date_run_member m
		  LEFT JOIN deal d
		         ON d.id = m.deal_id AND d.status = 'open' AND d.archived_at IS NULL
		  LEFT JOIN stage s ON s.id = d.stage_id
		 WHERE m.run_id = $1 AND m.outcome = 'pending'
		 ORDER BY m.deal_created_at, m.deal_id
		 LIMIT $2`, runID, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("close-date: reading the next members: %w", err)
	}
	defer rows.Close()

	var live []closeDateCandidate
	var members []closeDateMember
	for rows.Next() {
		var m closeDateMember
		var dealID *ids.DealID
		var cand closeDateCandidate
		var name *string
		var createdAt *time.Time
		var provisional *bool
		var pipelineID *ids.PipelineID
		var winProbability, remainingOpen *int
		if err := rows.Scan(&m.dealID, &m.createdAt,
			&dealID, &name, &createdAt, &cand.lastActivityAt, &cand.waitUntil,
			&cand.expectedClose, &provisional, &cand.forecastCat,
			&pipelineID, &winProbability, &remainingOpen); err != nil {
			return nil, nil, err
		}
		if dealID == nil {
			// Closed or archived since the freeze: still a member, still owed
			// an outcome, but nothing left to correct.
			m.gone = true
			members = append(members, m)
			continue
		}
		cand.id, cand.name, cand.createdAt = *dealID, *name, *createdAt
		cand.provisional, cand.pipelineID = *provisional, *pipelineID
		cand.winProbability, cand.remainingOpen = *winProbability, *remainingOpen
		live = append(live, cand)
		members = append(members, m)
	}
	return live, members, rows.Err()
}

// closeDateMember is one frozen row's identity, enough to settle it.
type closeDateMember struct {
	dealID    ids.DealID
	createdAt time.Time
	gone      bool
}

// settle records one member's terminal outcome and advances the run's cursor to
// it, in a single statement pair so a crash cannot leave the cursor ahead of the
// work.
//
// The cursor moves only over SETTLED members, which is what makes a resumed
// pass neither skip a deal nor correct one twice: everything before the cursor
// has an outcome, everything after it is still pending.
func (c *CloseDateCorrector) settle(
	ctx context.Context, tx pgx.Tx, runID ids.UUID, m closeDateMember, outcome string,
) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE close_date_run_member
		   SET outcome = $3, settled_at = now()
		 WHERE run_id = $1 AND deal_id = $2 AND outcome = 'pending'`,
		runID, m.dealID, outcome)
	if err != nil {
		return fmt.Errorf("close-date: settling %s: %w", m.dealID, err)
	}
	if tag.RowsAffected() == 0 {
		// Already settled by an earlier attempt: the retry is a no-op rather
		// than a second count.
		return nil
	}
	// A failed member is settled but NOT checked: it advances the cursor so the
	// pass moves past it, and stays out of the coverage numerator so "checked N
	// of M" never counts a deal the sweep could not assess.
	_, err = tx.Exec(ctx, `
		UPDATE close_date_run
		   SET checked = checked + CASE WHEN $3 = 'failed' THEN 0 ELSE 1 END,
		       corrected = corrected + CASE WHEN $3 = 'changed' THEN 1 ELSE 0 END,
		       staged = staged + CASE WHEN $3 = 'staged' THEN 1 ELSE 0 END,
		       cursor_created_at = $4, cursor_id = $2, updated_at = now()
		 WHERE id = $1`, runID, m.dealID, outcome, m.createdAt)
	if err != nil {
		return fmt.Errorf("close-date: advancing the pass over %s: %w", m.dealID, err)
	}
	return nil
}

// finishRun closes the pass.
// Complete means every member was settled AND none of them failed: a pass that
// walked its whole set but could not assess three deals covered less than it was
// asked to, and calling that "complete" would hide the gap this receipt exists
// to show.
//
// The `status = running` predicate is also the concurrency guard — closing a
// pass is a compare-and-set from running to a terminal word. Two workers racing
// to finish the same run both issue it, exactly one matches a still-running row,
// and the loser affects nothing rather than stamping a second finished_at over
// the winner's.
func (c *CloseDateCorrector) finishRun(ctx context.Context, tx pgx.Tx, runID ids.UUID) error {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE close_date_run r
		   SET status = CASE
		         WHEN NOT EXISTS (SELECT 1 FROM close_date_run_member m
		                           WHERE m.run_id = r.id
		                             AND m.outcome IN ('pending', 'failed'))
		         THEN $2 ELSE $3 END,
		       finished_at = now(), updated_at = now()
		 WHERE r.id = $1 AND r.status = $4`,
		runID, CloseDateRunComplete, CloseDateRunIncomplete, CloseDateRunRunning)
	if err != nil {
		return fmt.Errorf("close-date: closing the pass: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Already closed by a concurrent worker. Nothing to do and nothing
		// wrong: the pass is finished either way.
		return nil
	}
	return nil
}

// LatestCloseDateRun is the most recent pass, for a coverage line that can say
// "checked N of M" instead of implying the whole pipeline was seen.
func (c *CloseDateCorrector) LatestCloseDateRun(ctx context.Context) (*CloseDateRun, error) {
	var run CloseDateRun
	err := c.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT id, as_of, status, eligible, checked, corrected, staged, started_at
			  FROM close_date_run
			 ORDER BY started_at DESC
			 LIMIT 1`).
			Scan(&run.ID, &run.AsOf, &run.Status, &run.Eligible, &run.Checked,
				&run.Corrected, &run.Staged, &run.StartedAt)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}
