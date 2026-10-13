// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	// JobAttempt is the attempt the run's current job was queued under; zero
	// for a run only older code moved.
	JobAttempt int
	// Actor is who moved the run to the work it is queued for; the job acts as it.
	Actor                *StoredActor
	AttemptCorrelationID *ids.UUID
	Phase                string
	Progress             *Progress
}

// The wizard steps a draft can be reopened on.
const (
	DraftStepFiles = "files"
	DraftStepMap   = "map"
)

// ErrVersionMismatch refuses a write made against an older read of the run. It
// answers version_skew, and stays a conflict for the callers that test for one.
var ErrVersionMismatch = fmt.Errorf("the import run changed since it was read: %w: %w", apperrors.ErrVersionSkew, apperrors.ErrConflict)

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
	moveAbandon   moveName = "abandon"
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
	// moveAbandon closes a failed run carrying CauseValidationStopped, which wrote
	// nothing. Only the sweep selects such a run.
	moveAbandon: {from: []string{StatusFailed}, to: StatusCancelled},
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
	// queue marks a move that queues work: it opens a new attempt, which
	// fences off every older one, and stores who the job acts as.
	queue  bool
	values []runValue
	// literals are fixed assignments such as "report = NULL".
	literals      []string
	before, after map[string]any
}

// sessionState is the locked row a move decides on.
type sessionState struct {
	status, connector, sourceName, sourceRef, draftStep string
	version                                             int
	approved, released                                  bool
	supersededBy                                        *RunID
	mapping                                             []byte
}

// lockSession reads the run and holds it until the transaction ends, so the
// decision a move makes on it cannot be overtaken by another writer.
func lockSession(ctx context.Context, tx pgx.Tx, id RunID) (sessionState, error) {
	var state sessionState
	err := tx.QueryRow(ctx, `
		SELECT status, connector, source_name, source_ref, coalesce(draft_step, ''), version,
		       approved_at IS NOT NULL, source_released_at IS NOT NULL, superseded_by, mapping
		  FROM import_run WHERE id = $1 FOR UPDATE`, id).Scan(
		&state.status, &state.connector, &state.sourceName, &state.sourceRef, &state.draftStep,
		&state.version, &state.approved, &state.released, &state.supersededBy, &state.mapping)
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
	if state.connector != ConnectorCSV {
		return fmt.Errorf("import run %s is a %s run, which keeps no session: %w", id, state.connector, apperrors.ErrConflict)
	}
	if state.supersededBy != nil && !slices.Contains(move.from, state.status) {
		return &SupersededError{RunID: id, By: *state.supersededBy}
	}
	if !slices.Contains(move.from, state.status) {
		return fmt.Errorf("import run %s is %s, so it cannot become %s: %w", id, state.status, move.to, apperrors.ErrConflict)
	}
	if err := c.admitsVersion(id, state); err != nil {
		return err
	}
	if c.approved && !state.approved {
		return fmt.Errorf("import run %s was never approved, so it cannot resume: %w", id, apperrors.ErrConflict)
	}
	if move.to == StatusRunning && state.released {
		return fmt.Errorf("import run %s no longer holds its uploaded file, so it cannot run; upload the file again: %w", id, apperrors.ErrConflict)
	}
	return nil
}

// admitsVersion refuses a version other than zero that is not the run's.
func (c runChange) admitsVersion(id RunID, state sessionState) error {
	if c.version != 0 && c.version != state.version {
		return fmt.Errorf("import run %s is at version %d, not %d: %w", id, state.version, c.version, ErrVersionMismatch)
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
	var set strings.Builder
	set.WriteString(storekit.SQLf(`status = $%d, updated_at = now()`, arg(to)))
	for _, v := range change.values {
		set.WriteString(storekit.SQLf(`, %s = $%d`, v.column, arg(v.value)))
	}
	for _, literal := range change.literals {
		set.WriteString(", " + literal)
	}
	if change.queue {
		actor, err := encodeActorOf(ctx)
		if err != nil {
			return err
		}
		set.WriteString(", " + openAttempt)
		set.WriteString(storekit.SQLf(`, job_status = $%d, actor = $%d, attempt_correlation_id = $%d`,
			arg(to), arg(actor), arg(ids.NewV7())))
	}
	where := storekit.SQLf(` WHERE id = $1 AND status = $%d`, arg(state.status))
	if err := execHoldingName(ctx, tx, id, `UPDATE import_run SET `+set.String()+where, args...); err != nil {
		return fmt.Errorf("moving import run %s to %s: %w", id, to, err)
	}
	before, after := map[string]any{}, map[string]any{}
	if state.status != to {
		before[auditFieldStatus], after[auditFieldStatus] = state.status, to
	}
	maps.Copy(before, change.before)
	maps.Copy(after, change.after)
	_, err = storekit.Audit(ctx, tx, "update", ImportRunObject, id, before, after)
	return err
}

// QueueValidation moves a draft or a report to `validating`: the test run of
// version, after superseding every unwritten run of the same source name. It
// answers whether it moved the run. A run already validating that version is
// left as it stands, so a second request starts no second test.
func (s *RunStore) QueueValidation(ctx context.Context, id RunID, version int) (Run, bool, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, false, err
	}
	moved := false
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := lockSourceName(ctx, tx, id); err != nil {
			return err
		}
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
		if err := supersedeName(ctx, tx, id, state.sourceName, CauseSuperseded); err != nil {
			return err
		}
		moved = true
		return stageTransition(ctx, tx, id, moveValidate, runChange{
			version: version, queue: true, literals: []string{"report = NULL", "error = NULL"},
		})
	})
	if err != nil {
		return Run{}, false, err
	}
	run, err := s.GetStaged(ctx, id)
	return run, moved, err
}

// lockSourceName holds the run's source name until the transaction ends. Two
// test runs of one name are then decided in turn, and the later supersedes the
// earlier. It is taken before any row lock, so it cannot deadlock. A rename bumps the version, so admits refuses a stale name read here.
func lockSourceName(ctx context.Context, tx pgx.Tx, id RunID) error {
	_, err := tx.Exec(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended('import_run_source_name:' || source_name, 0))
		  FROM import_run WHERE id = $1 AND source_name <> ''`, id)
	if err != nil {
		return fmt.Errorf("locking the source name of import run %s: %w", id, err)
	}
	return nil
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
// file. Neither has written anything, so nothing is left to undo. A run already
// cancelled is answered as it stands, so a second click is no error.
func (s *RunStore) Cancel(ctx context.Context, id RunID) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		state, err := lockSession(ctx, tx, id)
		if err != nil {
			return err
		}
		if state.status == StatusCancelled {
			return nil
		}
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

// SourceBusyError refuses a move while another run of the same source name
// holds it: two runs filing one name would race on its bindings.
type SourceBusyError struct {
	RunID RunID
}

func (e *SourceBusyError) Error() string {
	return fmt.Sprintf("import run %s of the same source name is still in progress", e.RunID)
}

func (e *SourceBusyError) Unwrap() error { return apperrors.ErrConflict }

// SupersededError refuses a move on a run another run of its source name
// replaced. It names that run, so the caller can go on with it.
type SupersededError struct {
	RunID, By RunID
}

func (e *SupersededError) Error() string {
	return fmt.Sprintf("import run %s was replaced by import run %s", e.RunID, e.By)
}

func (e *SupersededError) Unwrap() error { return apperrors.ErrConflict }

// nameIndex keeps one run per source name among the statuses that hold it.
const nameIndex = "import_run_one_active_per_source_name"

// execHoldingName runs one status write under a savepoint. A write the name
// index refuses then answers SourceBusyError naming the holder.
func execHoldingName(ctx context.Context, tx pgx.Tx, id RunID, sql string, args ...any) error {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("opening a savepoint: %w", err)
	}
	_, err = savepoint.Exec(ctx, sql, args...)
	if constraint, unique := storekit.UniqueViolation(err); unique && constraint == nameIndex {
		if err := savepoint.Rollback(ctx); err != nil {
			return fmt.Errorf("rolling back to the savepoint: %w", err)
		}
		return nameHolder(ctx, tx, id)
	}
	if err != nil {
		return err
	}
	return savepoint.Commit(ctx)
}

// nameHolder names the other run that holds id's source name.
func nameHolder(ctx context.Context, tx pgx.Tx, id RunID) error {
	var holder RunID
	err := tx.QueryRow(ctx, `
		SELECT other.id FROM import_run run
		  JOIN import_run other ON other.connector = run.connector AND other.source_name = run.source_name
		 WHERE run.id = $1 AND other.id <> run.id AND other.status = ANY($2)
		 LIMIT 1`, id, nameHeldBy).Scan(&holder)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("another run of import run %s's source name moved at the same time: %w", id, apperrors.ErrConflict)
	}
	if err != nil {
		return fmt.Errorf("finding the run that holds the source name: %w", err)
	}
	return &SourceBusyError{RunID: holder}
}

// nameHeldBy is the statuses nameIndex covers.
var nameHeldBy = []string{StatusValidating, StatusAwaitingApproval, StatusRunning, StatusUndoing}

// supersedeName makes way for id on its source name. It cancels every other
// run of the name that wrote nothing, with cause. A run that is writing
// refuses it with SourceBusyError.
func supersedeName(ctx context.Context, tx pgx.Tx, id RunID, sourceName, cause string) error {
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
	rows, err := tx.Query(ctx, `
		SELECT id FROM import_run
		 WHERE connector = $1 AND source_name = $2 AND id <> $3 AND status = ANY($4)
		 ORDER BY id`, ConnectorCSV, sourceName, id, sessionMoves[moveSupersede].from)
	if err != nil {
		return fmt.Errorf("finding the runs a new test run replaces: %w", err)
	}
	replaced, err := pgx.CollectRows(rows, pgx.RowTo[RunID])
	if err != nil {
		return fmt.Errorf("finding the runs a new test run replaces: %w", err)
	}
	for _, old := range replaced {
		if err := stageTransition(ctx, tx, old, moveSupersede, runChange{
			values: []runValue{{"superseded_by", id}, {"error", cause}},
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
