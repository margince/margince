// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Session is what a run keeps between requests while its importer is still
// choosing, and what approval and release later stamp on it.
type Session struct {
	// Version is bumped by every write to a draft's choices, so a client
	// acting on an older read is refused rather than overwriting.
	Version   int
	DraftStep string
	// Profile is what the upload read of each file; read only by GetStaged,
	// and gone once the source is released.
	Profile        json.RawMessage
	ApprovedBy     string
	ApprovedAt     *time.Time
	SupersededBy   *RunID
	SourceReleased bool
}

// The wizard steps a draft can be reopened on.
const (
	DraftStepFiles = "files"
	DraftStepMap   = "map"
)

// ErrVersionMismatch refuses a write made against an older read of the run.
var ErrVersionMismatch = fmt.Errorf("the import run changed since it was read: %w", apperrors.ErrConflict)

// moveName names one row of sessionMoves.
type moveName string

const (
	moveRedraft   moveName = "redraft"
	moveValidate  moveName = "validate"
	moveReport    moveName = "report"
	moveRefuse    moveName = "refuse"
	moveApprove   moveName = "approve"
	moveResume    moveName = "resume"
	moveCancel    moveName = "cancel"
	moveSupersede moveName = "supersede"
)

// sessionMove is one transition: the states it may start from and its end.
type sessionMove struct {
	from []string
	to   string
}

// sessionMoves is the run's one transition table. stageTransition makes every
// status change of a staged run and refuses a start this table does not name.
var sessionMoves = map[moveName]sessionMove{
	moveRedraft:   {from: []string{StatusPending, StatusAwaitingApproval}, to: StatusPending},
	moveValidate:  {from: []string{StatusPending, StatusAwaitingApproval}, to: StatusValidating},
	moveReport:    {from: []string{StatusValidating}, to: StatusAwaitingApproval},
	moveRefuse:    {from: []string{StatusValidating}, to: StatusPending},
	moveApprove:   {from: []string{StatusAwaitingApproval}, to: StatusRunning},
	moveResume:    {from: []string{StatusFailed}, to: StatusRunning},
	moveCancel:    {from: []string{StatusPending, StatusAwaitingApproval}, to: StatusCancelled},
	moveSupersede: {from: []string{StatusValidating, StatusAwaitingApproval}, to: StatusCancelled},
}

// runValue is one column a move sets to a value. The column is always a
// literal written at the call site.
type runValue struct {
	column string
	value  any
}

// runChange is what a move writes beside the status, and what it audits.
type runChange struct {
	// version, when not zero, must equal the run's.
	version int
	// approved admits the move only for a run someone approved.
	approved bool
	values   []runValue
	// literals are fixed assignments such as "report = NULL".
	literals      []string
	before, after map[string]any
}

// sessionState is the locked row a move decides on.
type sessionState struct {
	status, connector, sourceName, sourceRef, draftStep string
	version                                             int
	approved                                            bool
	mapping                                             []byte
}

// lockSession reads the run and holds it until the transaction ends, so the
// decision a move makes on it cannot be overtaken by another writer.
func lockSession(ctx context.Context, tx pgx.Tx, id RunID) (sessionState, error) {
	var state sessionState
	err := tx.QueryRow(ctx, `
		SELECT status, connector, source_name, source_ref, coalesce(draft_step, ''), version,
		       approved_at IS NOT NULL, mapping
		  FROM import_run WHERE id = $1 FOR UPDATE`, id).Scan(
		&state.status, &state.connector, &state.sourceName, &state.sourceRef, &state.draftStep,
		&state.version, &state.approved, &state.mapping)
	if errors.Is(err, pgx.ErrNoRows) {
		return sessionState{}, fmt.Errorf("import run %s: %w", id, apperrors.ErrNotFound)
	}
	if err != nil {
		return sessionState{}, fmt.Errorf("reading import run %s: %w", id, err)
	}
	return state, nil
}

// admits answers why move may not start from state, or nil when it may.
func (c runChange) admits(id RunID, name moveName, state sessionState) error {
	move := sessionMoves[name]
	if !slices.Contains(move.from, state.status) {
		return fmt.Errorf("import run %s is %s, so it cannot become %s: %w", id, state.status, move.to, apperrors.ErrConflict)
	}
	if c.version != 0 && c.version != state.version {
		return fmt.Errorf("import run %s is at version %d, not %d: %w", id, state.version, c.version, ErrVersionMismatch)
	}
	if c.approved && !state.approved {
		return fmt.Errorf("import run %s was never approved, so it cannot resume: %w", id, apperrors.ErrConflict)
	}
	return nil
}

// stageTransition makes one move of sessionMoves on the caller's transaction,
// with its audit row. The status the run left is audited only when it moved.
func stageTransition(ctx context.Context, tx pgx.Tx, id RunID, name moveName, change runChange) error {
	state, err := lockSession(ctx, tx, id)
	if err != nil {
		return err
	}
	if err := change.admits(id, name, state); err != nil {
		return err
	}
	to := sessionMoves[name].to
	args := []any{id}
	arg := func(v any) int { args = append(args, v); return len(args) }
	set := storekit.SQLf(`status = $%d, updated_at = now()`, arg(to))
	for _, v := range change.values {
		set += storekit.SQLf(`, %s = $%d`, v.column, arg(v.value))
	}
	for _, literal := range change.literals {
		set += ", " + literal
	}
	where := storekit.SQLf(` WHERE id = $1 AND status = $%d`, arg(state.status))
	if _, err := tx.Exec(ctx, `UPDATE import_run SET `+set+where, args...); err != nil {
		return fmt.Errorf("moving import run %s to %s: %w", id, to, err)
	}
	before, after := map[string]any{}, map[string]any{}
	if state.status != to {
		before[auditFieldStatus], after[auditFieldStatus] = state.status, to
	}
	for field, value := range change.before {
		before[field] = value
	}
	for field, value := range change.after {
		after[field] = value
	}
	_, err = storekit.Audit(ctx, tx, "update", ImportRunObject, id, before, after)
	return err
}

// QueueValidation moves a draft or a report to `validating`: the test run of
// version, after superseding every unwritten run of the same source name. A
// second call for a run already validating that version changes nothing.
func (s *RunStore) QueueValidation(ctx context.Context, id RunID, version int) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		state, err := lockSession(ctx, tx, id)
		if err != nil {
			return err
		}
		if state.status == StatusValidating && state.version == version {
			return nil
		}
		if err := (runChange{version: version}).admits(id, moveValidate, state); err != nil {
			return err
		}
		if err := supersedeName(ctx, tx, id, state.sourceName); err != nil {
			return err
		}
		return stageTransition(ctx, tx, id, moveValidate, runChange{
			version: version, literals: []string{"report = NULL", "error = NULL"},
		})
	})
	if err != nil {
		return Run{}, err
	}
	return s.GetStaged(ctx, id)
}

// ReturnToDraft ends a test run that was refused, keeping why on the draft so
// the importer sees it beside the choices they can change.
func (s *RunStore) ReturnToDraft(ctx context.Context, id RunID, cause string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return stageTransition(ctx, tx, id, moveRefuse, runChange{values: []runValue{{"error", cause}}})
	})
}

// Cancel ends a draft or a report that nobody will approve, and releases its
// file. Neither has written anything, so nothing is left to undo.
func (s *RunStore) Cancel(ctx context.Context, id RunID) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := stageTransition(ctx, tx, id, moveCancel, runChange{}); err != nil {
			return err
		}
		return releaseSource(ctx, tx, id)
	})
	if err != nil {
		return Run{}, err
	}
	return s.GetStaged(ctx, id)
}

// SourceBusyError refuses a test run while another run of the same source
// name is writing: two runs filing one name would race on its bindings.
type SourceBusyError struct {
	RunID RunID
}

func (e *SourceBusyError) Error() string {
	return fmt.Sprintf("import run %s of the same source is still writing", e.RunID)
}

func (e *SourceBusyError) Unwrap() error { return apperrors.ErrConflict }

// Supersede cancels every other test run and report of sourceName on the
// caller's transaction, naming id as the run that replaced them. A run of
// that name that is writing refuses it with SourceBusyError.
func (s *RunStore) Supersede(ctx context.Context, tx pgx.Tx, id RunID, sourceName string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return supersedeName(ctx, tx, id, sourceName)
}

func supersedeName(ctx context.Context, tx pgx.Tx, id RunID, sourceName string) error {
	if sourceName == "" {
		return nil
	}
	var busy RunID
	err := tx.QueryRow(ctx, `
		SELECT id FROM import_run
		 WHERE connector = $1 AND source_name = $2 AND id <> $3 AND status = ANY($4)
		 LIMIT 1`, ConnectorCSV, sourceName, id, []string{StatusRunning, StatusUndoing}).Scan(&busy)
	if err == nil {
		return &SourceBusyError{RunID: busy}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("checking for a writing run of the source: %w", err)
	}
	return supersedeWhere(ctx, tx, id, "source_name", sourceName)
}

// supersedeWhere cancels and releases every unwritten csv run, other than
// id, whose column equals value. column is a literal at every call site.
func supersedeWhere(ctx context.Context, tx pgx.Tx, id RunID, column, value string) error {
	rows, err := tx.Query(ctx, storekit.SQLf(`
		SELECT id FROM import_run
		 WHERE connector = $1 AND %s = $2 AND id <> $3 AND status = ANY($4)
		 ORDER BY id`, column), ConnectorCSV, value, id, sessionMoves[moveSupersede].from)
	if err != nil {
		return fmt.Errorf("finding the runs a new test run replaces: %w", err)
	}
	replaced, err := pgx.CollectRows(rows, pgx.RowTo[RunID])
	if err != nil {
		return fmt.Errorf("finding the runs a new test run replaces: %w", err)
	}
	for _, old := range replaced {
		if err := stageTransition(ctx, tx, old, moveSupersede, runChange{
			values: []runValue{{"superseded_by", id}, {"error", CauseSuperseded}},
			after:  map[string]any{"superseded_by": id},
		}); err != nil {
			return err
		}
		if err := releaseSource(ctx, tx, old); err != nil {
			return err
		}
	}
	return nil
}
