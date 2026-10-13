// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// A commit or an undo runs in the worker as slices of one attempt. The run row
// is the fence. Every move that queues work bumps job_attempt, and every write
// an attempt makes names the status and attempt it was queued for. A write that
// finds either moved on answers ErrSuperseded, and the attempt exits without
// recording a failure.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrSuperseded answers a fenced write whose run left the status or attempt the
// lease names. It is never a failure of the run: a newer attempt, a cancel or
// the sweep owns the run now.
var ErrSuperseded = errors.New("the import run moved on to another attempt")

// StopFlushTimeout bounds the flush a stopped slice writes after its context
// ended. It fits inside the worker's cancel window, so the flush lands first.
const StopFlushTimeout = 2 * time.Second

// The phases a run reports; import_run_phase_check holds the same list. A
// test run reads, then checks; a commit or an undo reports the rest.
const (
	PhaseReading      = "reading"
	PhaseChecking     = "checking"
	PhaseCreatingTags = "creating_tags"
	PhaseImporting    = "importing"
	PhaseLinking      = "linking"
	PhaseUndoing      = "undoing"
	PhaseFinishing    = "finishing"
)

// progressLinks is the progress entry of the link phase, after every object.
const progressLinks = "links"

// Lease names one attempt at a run: the status it was queued for and its number.
type Lease struct {
	Run     RunID
	Status  string
	Attempt int
}

// Progress is what a run has done so far, for a reader polling it.
type Progress struct {
	Objects []ObjectProgress `json:"objects"`
	Counts  ProgressCounts   `json:"counts"`
	// Attempt is the attempt that wrote it, so an attempt's first flush counts
	// one more walk in the report.
	Attempt int `json:"attempt,omitempty"`
}

// ObjectProgress is one object's rows, or the links, done of its total.
type ObjectProgress struct {
	Object    string `json:"object"`
	RowsDone  int    `json:"rows_done"`
	RowsTotal int    `json:"rows_total"`
}

// ProgressCounts are the row outcomes so far. Problems counts rows that carry
// a problem, skipped or landed.
type ProgressCounts struct {
	Created   int `json:"created"`
	Updated   int `json:"updated"`
	Unchanged int `json:"unchanged"`
	Skipped   int `json:"skipped"`
	Problems  int `json:"problems"`
}

// Flush is one fenced progress write: the cursor, the report leg since the last
// flush, and where the run stands.
type Flush struct {
	Checkpoint int
	Leg        Report
	// StartsWalk marks the leg at the run's first row, whose edges replace
	// the ones the test run predicted.
	StartsWalk bool
	Progress   Progress
	Phase      string
	Object     string
}

// StoredActor is the principal that moved a run, kept so the worker acts as it.
type StoredActor struct {
	Type         principal.PrincipalType `json:"type"`
	ID           string                  `json:"id"`
	UserID       ids.UUID                `json:"user_id"`
	PassportID   ids.UUID                `json:"passport_id"`
	ConnectionID ids.UUID                `json:"connection_id"`
	OnBehalfOf   ids.UUID                `json:"on_behalf_of"`
}

// StoredActorOf is what a queued job needs to act as this principal again.
// Grants are left out, because the job reads them live.
func StoredActorOf(p principal.Principal) StoredActor {
	return StoredActor{
		Type: p.Type, ID: p.ID, UserID: p.UserID, PassportID: p.PassportID,
		ConnectionID: p.ConnectionID, OnBehalfOf: p.OnBehalfOf,
	}
}

// encodeActorOf encodes the acting principal for a move that queues work.
func encodeActorOf(ctx context.Context) ([]byte, error) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return nil, fmt.Errorf("queueing import work needs an authenticated principal: %w", apperrors.ErrPermissionDenied)
	}
	return json.Marshal(StoredActorOf(p))
}

// openAttempt is the SET of every move that queues work. The attempt is new, so
// the sweep counts its resumes from zero.
const openAttempt = `job_attempt = job_attempt + 1, resume_count = 0`

// fence is the predicate of every write a lease makes.
const fence = ` WHERE id = $1 AND status = $2 AND job_attempt = $3`

func (l Lease) args(more ...any) []any {
	return append([]any{l.Run, l.Status, l.Attempt}, more...)
}

// execFenced runs one fenced UPDATE and answers ErrSuperseded when it missed.
func execFenced(ctx context.Context, tx pgx.Tx, l Lease, set string, more ...any) error {
	tag, err := tx.Exec(ctx, `UPDATE import_run SET `+set+fence, l.args(more...)...)
	if err != nil {
		return fmt.Errorf("writing import run %s attempt %d: %w", l.Run, l.Attempt, err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("import run %s attempt %d: %w", l.Run, l.Attempt, ErrSuperseded)
	}
	return nil
}

// Claim starts or continues the lease's attempt: the run is claimed and its
// heartbeat set. It answers the run as it stands, which seeds the slice.
func (s *RunStore) Claim(ctx context.Context, l Lease) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	var run Run
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := execFenced(ctx, tx, l, `claimed_attempt = $3, heartbeat_at = now(), updated_at = now(),
			started_at = coalesce(started_at, now())`); err != nil {
			return err
		}
		var err error
		run, err = readStaged(ctx, tx, l.Run)
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

// Heartbeat tells the sweep the lease's attempt is alive while it writes no
// progress. It leaves updated_at alone, which dates the run's last real change.
func (s *RunStore) Heartbeat(ctx context.Context, l Lease) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	ctx = database.Detached(ctx)
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execFenced(ctx, tx, l, `heartbeat_at = now()`)
	})
}

// Yield hands the attempt back to its queue between two slices. The run then
// reads queued, so a wait behind other runs never reads as a dead process.
func (s *RunStore) Yield(ctx context.Context, l Lease) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return execFenced(ctx, tx, l, `claimed_attempt = NULL, updated_at = now()`)
	})
}

// Flush writes the cursor, the report with this leg folded in, and the
// progress, so all three always describe the same prefix of the source.
func (s *RunStore) Flush(ctx context.Context, l Lease, f Flush) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return flushInTx(ctx, tx, l, f)
	})
}

func flushInTx(ctx context.Context, tx pgx.Tx, l Lease, f Flush) error {
	var stored []byte
	var writtenBy *int
	err := tx.QueryRow(ctx, `SELECT report, (progress->>'attempt')::int FROM import_run`+fence+` FOR UPDATE`,
		l.args()...).Scan(&stored, &writtenBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("import run %s attempt %d: %w", l.Run, l.Attempt, ErrSuperseded)
	}
	if err != nil {
		return fmt.Errorf("reading import run %s's recorded report: %w", l.Run, err)
	}
	var base Report
	if len(stored) > 0 {
		if err := json.Unmarshal(stored, &base); err != nil {
			return fmt.Errorf("decoding import run %s's recorded report: %w", l.Run, err)
		}
	}
	folded := base.withLeg(f.Leg, f.StartsWalk)
	if writtenBy == nil || *writtenBy != l.Attempt {
		folded.Attempts = base.Walks() + 1
	}
	report, err := json.Marshal(folded)
	if err != nil {
		return fmt.Errorf("encoding import run %s's report: %w", l.Run, err)
	}
	f.Progress.Attempt = l.Attempt
	progress, err := json.Marshal(f.Progress)
	if err != nil {
		return fmt.Errorf("encoding import run %s's progress: %w", l.Run, err)
	}
	return execFenced(ctx, tx, l, `checkpoint = GREATEST(checkpoint, $4), report = $5, progress = $6,
		phase = $7, phase_object = $8, progress_at = now(), heartbeat_at = now(), updated_at = now()`,
		f.Checkpoint, report, progress, nullable(f.Phase), nullable(f.Object))
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// FailLease records why the attempt stopped. A test run returns to its draft
// with the cause. A commit becomes failed, which a resume continues from its
// checkpoint. An undo stays undoing with the cause, because failing it would
// let a resume re-run the commit over a half-undone run.
// Its job_status is cleared: no job is queued for it, so the sweep leaves it to
// an undo request.
func (s *RunStore) FailLease(ctx context.Context, l Lease, cause string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return failLeaseInTx(ctx, tx, l, cause)
	})
}

// failLeaseInTx is FailLease on the caller's transaction.
func failLeaseInTx(ctx context.Context, tx pgx.Tx, l Lease, cause string) error {
	switch l.Status {
	case StatusRunning:
		if err := execFenced(ctx, tx, l, `status = $4, error = $5, updated_at = now()`, StatusFailed, cause); err != nil {
			return err
		}
		_, err := storekit.Audit(ctx, tx, "update", ImportRunObject, l.Run,
			map[string]any{auditFieldStatus: l.Status}, map[string]any{auditFieldStatus: StatusFailed})
		return err
	case StatusValidating:
		return refuseValidationInTx(ctx, tx, l, cause)
	case StatusUndoing:
		var prior *string
		err := tx.QueryRow(ctx, `SELECT error FROM import_run`+fence+` FOR UPDATE`, l.args()...).Scan(&prior)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("import run %s attempt %d: %w", l.Run, l.Attempt, ErrSuperseded)
		}
		if err != nil {
			return fmt.Errorf("reading import run %s: %w", l.Run, err)
		}
		if err := execFenced(ctx, tx, l, `error = $4, job_status = NULL, updated_at = now()`, cause); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", ImportRunObject, l.Run,
			map[string]any{errorColumn: prior}, map[string]any{errorColumn: cause})
		return err
	default:
		return fmt.Errorf("import run %s: a %s attempt has no failed state: %w", l.Run, l.Status, apperrors.ErrConflict)
	}
}

// completeLeaseInTx finishes a commit attempt: its last leg, then complete, and
// the uploaded file released, because undo reads the identity map, never the file.
func completeLeaseInTx(ctx context.Context, tx pgx.Tx, l Lease, f Flush) error {
	if err := flushInTx(ctx, tx, l, f); err != nil {
		return err
	}
	if err := execFenced(ctx, tx, l, `status = $4, finished_at = now(), updated_at = now()`, StatusComplete); err != nil {
		return err
	}
	if err := releaseSource(ctx, tx, l.Run); err != nil {
		return err
	}
	_, err := storekit.Audit(ctx, tx, "update", ImportRunObject, l.Run,
		map[string]any{auditFieldStatus: l.Status}, map[string]any{auditFieldStatus: StatusComplete})
	return err
}

// readLeased reads the run the lease names, or ErrSuperseded once it moved on.
func readLeased(ctx context.Context, tx pgx.Tx, l Lease) (Run, error) {
	run, err := readStaged(ctx, tx, l.Run)
	if err != nil {
		return Run{}, err
	}
	if run.Status != l.Status || run.JobAttempt != l.Attempt {
		return Run{}, fmt.Errorf("import run %s is %s at attempt %d, not %s at %d: %w",
			l.Run, run.Status, run.JobAttempt, l.Status, l.Attempt, ErrSuperseded)
	}
	return run, nil
}

func decodeLeaseColumns(run *Run, actor, progress []byte) error {
	if len(actor) > 0 {
		run.Actor = &StoredActor{}
		if err := json.Unmarshal(actor, run.Actor); err != nil {
			return fmt.Errorf("reading the actor of import run %s: %w", run.ID, err)
		}
	}
	if len(progress) > 0 {
		run.Progress = &Progress{}
		if err := json.Unmarshal(progress, run.Progress); err != nil {
			return fmt.Errorf("reading the progress of import run %s: %w", run.ID, err)
		}
	}
	return nil
}

// flushContext answers ctx while it is live, and once it ended a context of
// its own, so the write recording a stop still lands.
func flushContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx.Err() == nil {
		return ctx, func() {}
	}
	return context.WithTimeout(context.WithoutCancel(ctx), StopFlushTimeout)
}
