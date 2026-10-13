// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// A test run is one attempt in the worker, fenced like a commit slice. It reads
// in one snapshot and writes nothing to the estate, so it takes no run lock.
// Its progress is written beside the snapshot, never inside it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// testRunEnded is what every move out of `validating` clears: no job is
// queued or claimed, and the phase and progress describe a read that is over.
// A commit seeds its counts from progress, so a test run's must never reach it.
var testRunEnded = append([]string{"job_status = NULL", "claimed_attempt = NULL"}, testReadForgotten...)

// testReadForgotten clears how far a test's read had come. An attempt that
// stops reads again from the top, so that progress describes no read going on.
var testReadForgotten = []string{"phase = NULL", "phase_object = NULL", "progress = NULL"}

// restartingTest adds testReadForgotten to set when l is a test run, whose
// next attempt starts its read from the top.
func restartingTest(l Lease, set string) string {
	if l.Status != StatusValidating {
		return set
	}
	return set + ", " + strings.Join(testReadForgotten, ", ")
}

// fencedOff is testRunEnded for a move a human makes while a job may still
// work. The attempt moves on, so that job's next fenced write misses.
var fencedOff = append([]string{"job_attempt = job_attempt + 1"}, testRunEnded...)

// agentTestIndex keeps one test run per agent in `validating`.
const agentTestIndex = "import_run_one_test_per_agent"

// AgentBusyError refuses an agent's second test run while its first is still
// running, and names that run so the agent can wait on it.
type AgentBusyError struct {
	RunID RunID
}

func (e *AgentBusyError) Error() string {
	return fmt.Sprintf("this agent's test run of import run %s is still running", e.RunID)
}

func (e *AgentBusyError) Unwrap() error { return apperrors.ErrConflict }

// agentTestHolder names the acting agent's other test run.
func agentTestHolder(ctx context.Context, tx pgx.Tx, id RunID) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return fmt.Errorf("import run %s: another test run of the same agent is running: %w", id, apperrors.ErrConflict)
	}
	var holder RunID
	err := tx.QueryRow(ctx, `
		SELECT id FROM import_run
		 WHERE status = $1 AND actor->>'type' = $2 AND actor->>'id' = $3 AND id <> $4
		 LIMIT 1`, StatusValidating, string(principal.PrincipalAgent), p.ID, id).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("another test run of this agent moved at the same time as import run %s: %w", id, apperrors.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("finding the agent's running test run: %w", err)
	}
	return &AgentBusyError{RunID: holder}
}

// FlushTestProgress records how far the lease's test run has read. It runs on
// its own transaction even when ctx carries the test run's snapshot. Inside it,
// no reader would see the write until the read ends.
func (s *RunStore) FlushTestProgress(ctx context.Context, l Lease, p Progress, phase, object string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	encoded, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encoding import run %s's progress: %w", l.Run, err)
	}
	ctx = database.Detached(ctx)
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execFenced(ctx, tx, l, `progress = $4, phase = $5, phase_object = $6,
			progress_at = now(), heartbeat_at = now(), updated_at = now()`, encoded, nullable(phase), nullable(object))
	})
}

// FinishValidation parks the lease's test run for its approver with its report.
func (s *RunStore) FinishValidation(ctx context.Context, l Lease, report Report) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encoding the test-run report: %w", err)
	}
	ctx = database.Detached(ctx)
	return s.tx(ctx, func(tx pgx.Tx) error {
		return stageTransition(ctx, tx, l.Run, moveReport, runChange{
			lease: &l, values: []runValue{{"report", encoded}}, literals: testRunEnded,
		})
	})
}

// RefuseValidation returns the lease's test run to its draft with cause, so
// the importer reads why beside the choices they can change.
func (s *RunStore) RefuseValidation(ctx context.Context, l Lease, cause string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	ctx = database.Detached(ctx)
	return s.tx(ctx, func(tx pgx.Tx) error {
		return refuseValidationInTx(ctx, tx, l, cause)
	})
}

func refuseValidationInTx(ctx context.Context, tx pgx.Tx, l Lease, cause string) error {
	return stageTransition(ctx, tx, l.Run, moveRefuse, runChange{
		lease: &l, values: []runValue{{errorColumn, cause}}, literals: testRunEnded,
	})
}
