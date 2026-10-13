// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepBatch bounds the runs one rule settles in one pass, so its transaction
// stays short; the next pass takes the rest.
const sweepBatch = 100

// errorColumn is the column that keeps why a run stopped, as the run shows it.
const errorColumn = "error"

// sweepRule selects the csv runs one sweep rule settles.
type sweepRule struct {
	statuses []string
	// idle is how long the run's last write must be behind the database clock.
	idle time.Duration
	// errorIs, when not empty, narrows the rule to runs carrying that error.
	errorIs string
	// unreleased narrows the rule to runs that still hold their source.
	unreleased bool
	// legacy narrows the rule to runs whose status old code entered.
	legacy bool
}

// ReturnStaleTestRuns returns to draft every test run old code started that
// has not moved for idle. Its process is gone, and its choices are still good. A test run a queued job works is the resume rules' to settle.
func (s *RunStore) ReturnStaleTestRuns(ctx context.Context, idle time.Duration) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	rule := sweepRule{statuses: sessionMoves[moveRefuse].from, idle: idle, legacy: true}
	return s.sweepEach(ctx, rule, func(tx pgx.Tx, id RunID) error {
		return stageTransition(ctx, tx, id, moveRefuse, runChange{
			values: []runValue{{errorColumn, CauseTestRunStopped}}, literals: testRunEnded,
		})
	})
}

// CancelStoppedTestRuns closes every test run that old code recorded as
// failed. It wrote nothing, so no resume can need its file.
func (s *RunStore) CancelStoppedTestRuns(ctx context.Context) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	rule := sweepRule{statuses: sessionMoves[moveAbandon].from, errorIs: CauseValidationStopped}
	return s.sweepEach(ctx, rule, func(tx pgx.Tx, id RunID) error {
		if err := stageTransition(ctx, tx, id, moveAbandon, runChange{}); err != nil {
			return err
		}
		return releaseSource(ctx, tx, id)
	})
}

// CancelIdleDrafts closes every draft and report nobody has touched for idle,
// and releases its file.
func (s *RunStore) CancelIdleDrafts(ctx context.Context, idle time.Duration) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	rule := sweepRule{statuses: sessionMoves[moveCancel].from, idle: idle}
	return s.sweepEach(ctx, rule, func(tx pgx.Tx, id RunID) error {
		if err := stageTransition(ctx, tx, id, moveCancel, runChange{values: []runValue{{errorColumn, CauseDraftExpired}}}); err != nil {
			return err
		}
		return releaseSource(ctx, tx, id)
	})
}

// ReleaseFinishedSources releases the file of every complete or undone run
// that still holds it. Neither state reads its source again.
func (s *RunStore) ReleaseFinishedSources(ctx context.Context) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	rule := sweepRule{statuses: []string{StatusComplete, StatusUndone}, unreleased: true}
	return s.sweepEach(ctx, rule, func(tx pgx.Tx, id RunID) error {
		if err := releaseSource(ctx, tx, id); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "update", ImportRunObject, id,
			map[string]any{"source_released": false}, map[string]any{"source_released": true})
		return err
	})
}

// sweepEach settles up to sweepBatch runs the rule selects, in one transaction.
// A run another writer holds is skipped, and the next pass meets it again.
func (s *RunStore) sweepEach(ctx context.Context, rule sweepRule, settle func(pgx.Tx, RunID) error) (int, error) {
	settled := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM import_run
			 WHERE connector = $1 AND status = ANY($2)
			   AND updated_at < now() - make_interval(secs => $3)
			   AND ($4::text = '' OR error = $4::text)
			   AND (NOT $5::bool OR source_released_at IS NULL)
			   AND (NOT $7::bool OR `+legacyRow+`)
			 ORDER BY updated_at, id
			 LIMIT $6
			   FOR UPDATE SKIP LOCKED`,
			ConnectorCSV, rule.statuses, rule.idle.Seconds(), rule.errorIs, rule.unreleased, sweepBatch, rule.legacy)
		if err != nil {
			return fmt.Errorf("selecting %v import runs to sweep: %w", rule.statuses, err)
		}
		due, err := pgx.CollectRows(rows, pgx.RowTo[RunID])
		if err != nil {
			return fmt.Errorf("selecting %v import runs to sweep: %w", rule.statuses, err)
		}
		for _, id := range due {
			if err := settle(tx, id); err != nil {
				return err
			}
		}
		settled = len(due)
		return nil
	})
	return settled, err
}

// The causes a run carries once the sweep stopped resuming its stale attempts.
const (
	CauseStale     = "the import stopped responding too many times; approve it again to continue from its checkpoint"
	CauseUndoStale = "the undo stopped responding too many times; undo it again to continue"
)

// Requeue inserts the job of a run's current attempt, on the sweep's
// transaction. It answers false when a job of that attempt is still queued.
type Requeue func(ctx context.Context, tx pgx.Tx, run Run) (inserted bool, err error)

// legacyRow marks a run whose status old code entered, or whose attempt queues
// no job. The sweep never re-queues such a run.
const legacyRow = `(job_attempt = 0 OR job_status IS DISTINCT FROM status)`

// queuedStatuses are the statuses a queued job works.
var queuedStatuses = []string{StatusValidating, StatusRunning, StatusUndoing}

// queuedWork selects the runs of the given statuses a queued job works.
const queuedWork = `connector = $1 AND status = ANY($2) AND NOT ` + legacyRow

// The guards of the two resume rules, over the age in $4. The selecting read
// and the fenced settle both apply them, so a run that moved since stays put.
const (
	staleClaim = ` AND claimed_attempt = job_attempt AND heartbeat_at < now() - make_interval(secs => $4)`
	lostJob    = ` AND claimed_attempt IS DISTINCT FROM job_attempt AND updated_at < now() - make_interval(secs => $4)`
)

// dueAttempt is one attempt a resume rule selected, with the resumes it had.
type dueAttempt struct {
	lease   Lease
	resumes int
}

// ResumeStaleAttempts opens a new attempt of every test run, commit or undo
// whose claimed attempt has not beaten for staleAfter, and queues its job. A
// run already resumed maxResumes times stops instead. A test run returns to its
// draft, a commit fails, and an undo keeps undoing with the cause.
func (s *RunStore) ResumeStaleAttempts(ctx context.Context, staleAfter time.Duration, maxResumes int, requeue Requeue) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	due, err := s.dueAttempts(ctx, staleClaim, staleAfter)
	if err != nil {
		return 0, err
	}
	return settleEach(due, func(d dueAttempt) error {
		return s.settleStale(ctx, d, staleAfter, maxResumes, requeue)
	})
}

// settleStale resumes or stops one attempt the read found stale, once its row
// lock shows it still is. A slice that beat since then keeps its run.
func (s *RunStore) settleStale(ctx context.Context, d dueAttempt, staleAfter time.Duration, maxResumes int, requeue Requeue) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockGuarded(ctx, tx, d.lease, staleClaim, staleAfter); err != nil {
			return err
		}
		if d.resumes >= maxResumes {
			return failLeaseInTx(ctx, tx, d.lease, staleCause(d.lease.Status))
		}
		return reopenStale(ctx, tx, d.lease, requeue)
	})
}

// RequeueLostJobs inserts once more the job of every test run, commit or undo still
// queued after queuedAfter. A job that is still alive dedupes; one lost before
// its claim is inserted, and only that counts as a resume.
func (s *RunStore) RequeueLostJobs(ctx context.Context, queuedAfter time.Duration, requeue Requeue) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	due, err := s.dueAttempts(ctx, lostJob, queuedAfter)
	if err != nil {
		return 0, err
	}
	reinserted := 0
	_, err = settleEach(due, func(d dueAttempt) error {
		var inserted bool
		err := s.tx(ctx, func(tx pgx.Tx) error {
			var err error
			inserted, err = reinsertLost(ctx, tx, d.lease, queuedAfter, requeue)
			return err
		})
		if err == nil && inserted {
			reinserted++
		}
		return err
	})
	return reinserted, err
}

func staleCause(status string) string {
	switch status {
	case StatusUndoing:
		return CauseUndoStale
	case StatusValidating:
		return CauseTestRunStopped
	default:
		return CauseStale
	}
}

// dueAttempts reads up to sweepBatch attempts the predicate selects. Each is
// settled in its own fenced transaction, so the read takes no lock.
func (s *RunStore) dueAttempts(ctx context.Context, predicate string, age time.Duration) ([]dueAttempt, error) {
	var due []dueAttempt
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, status, job_attempt, resume_count FROM import_run
			 WHERE `+queuedWork+predicate+`
			 ORDER BY updated_at, id
			 LIMIT $3`, ConnectorCSV, queuedStatuses, sweepBatch, age.Seconds())
		if err != nil {
			return fmt.Errorf("selecting import attempts to resume: %w", err)
		}
		due, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (dueAttempt, error) {
			var d dueAttempt
			err := row.Scan(&d.lease.Run, &d.lease.Status, &d.lease.Attempt, &d.resumes)
			return d, err
		})
		if err != nil {
			return fmt.Errorf("selecting import attempts to resume: %w", err)
		}
		return nil
	})
	return due, err
}

// settleEach settles every due attempt, so one that fails keeps no other from
// its resume. One another writer moved on meanwhile is left to that writer.
func settleEach(due []dueAttempt, settle func(dueAttempt) error) (int, error) {
	settled := 0
	var failures []error
	for _, d := range due {
		err := settle(d)
		switch {
		case errors.Is(err, ErrSuperseded):
		case err != nil:
			failures = append(failures, err)
		default:
			settled++
		}
	}
	return settled, errors.Join(failures...)
}

// lockGuarded locks the run while its lease and the rule's guard still hold,
// and answers ErrSuperseded once either moved.
func lockGuarded(ctx context.Context, tx pgx.Tx, l Lease, guard string, age time.Duration) error {
	var held bool
	err := tx.QueryRow(ctx, `SELECT true FROM import_run`+fence+guard+` FOR UPDATE`, l.args(age.Seconds())...).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("import run %s attempt %d: %w", l.Run, l.Attempt, ErrSuperseded)
	}
	if err != nil {
		return fmt.Errorf("locking import run %s attempt %d: %w", l.Run, l.Attempt, err)
	}
	return nil
}

// reopenStale fences off the stale attempt the caller locked, opens the next
// one with its own trace, and queues it.
func reopenStale(ctx context.Context, tx pgx.Tx, l Lease, requeue Requeue) error {
	if err := execFenced(ctx, tx, l, `job_attempt = job_attempt + 1, attempt_correlation_id = $4,
		resume_count = resume_count + 1, updated_at = now()`, ids.NewV7()); err != nil {
		return err
	}
	run, err := readStaged(ctx, tx, l.Run)
	if err != nil {
		return err
	}
	inserted, err := requeue(ctx, tx, run)
	if err != nil {
		return fmt.Errorf("queueing import run %s attempt %d: %w", l.Run, run.JobAttempt, err)
	}
	if !inserted {
		return fmt.Errorf("import run %s attempt %d was already queued, so two attempts share its number", l.Run, run.JobAttempt)
	}
	return nil
}

// reinsertLost queues the run's current attempt once more and counts a resume
// only when that inserted, which means the job had been lost. Either way the
// run is dated, so the rule asks again only a whole window later.
func reinsertLost(ctx context.Context, tx pgx.Tx, l Lease, queuedAfter time.Duration, requeue Requeue) (bool, error) {
	if err := lockGuarded(ctx, tx, l, lostJob, queuedAfter); err != nil {
		return false, err
	}
	run, err := readStaged(ctx, tx, l.Run)
	if err != nil {
		return false, err
	}
	inserted, err := requeue(ctx, tx, run)
	if err != nil {
		return false, fmt.Errorf("queueing import run %s attempt %d again: %w", l.Run, l.Attempt, err)
	}
	if !inserted {
		return false, execFenced(ctx, tx, l, `updated_at = now()`)
	}
	return true, execFenced(ctx, tx, l, `resume_count = resume_count + 1, updated_at = now()`)
}
