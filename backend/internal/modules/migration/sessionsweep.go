// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
}

// ReturnStaleTestRuns returns to draft every test run that has not moved for
// idle: its process is gone, and the choices it tested are still good.
func (s *RunStore) ReturnStaleTestRuns(ctx context.Context, idle time.Duration) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	rule := sweepRule{statuses: sessionMoves[moveRefuse].from, idle: idle}
	return s.sweepEach(ctx, rule, func(tx pgx.Tx, id RunID) error {
		return stageTransition(ctx, tx, id, moveRefuse, runChange{values: []runValue{{errorColumn, CauseTestRunStopped}}})
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
			 ORDER BY updated_at, id
			 LIMIT $6
			   FOR UPDATE SKIP LOCKED`,
			ConnectorCSV, rule.statuses, rule.idle.Seconds(), rule.errorIs, rule.unreleased, sweepBatch)
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
