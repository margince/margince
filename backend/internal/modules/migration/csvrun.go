// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The two lifecycle states the direct importer adds to the flip's
// running→complete|failed (IEM-DDL-1): a run is validated first and committed
// only after a human has read what the validation found. The flip has no such
// pair because its own preflight is the gate.
const (
	StatusValidating       = "validating"
	StatusAwaitingApproval = "awaiting_approval"
)

// A saved session adds a draft before the test run. It also adds an end for a
// run that stopped before it wrote: cancelled, superseded or left idle.
const (
	StatusPending   = "pending"
	StatusCancelled = "cancelled"
)

// RunMapping is what a staged run knows about its source beyond the file
// itself: which object the rows are, which source column lands in which field,
// and which column identifies a row.
//
// It lives in import_run.mapping rather than a table of its own — one run's
// mapping is one fact about that run, and the ratification (IEM-GAP-2) chose
// the column over IEM-DDL-4's table, whose composite tenant key ADR-0091 had
// already retired.
type RunMapping struct {
	Object    string            `json:"object"`
	Fields    map[string]string `json:"fields"`
	SourceKey string            `json:"source_key"`
	// OnDuplicate is what to do with a row naming a record the estate already
	// holds. It rides with the mapping so the commit honours the same choice
	// the dry run reported on — a run approved as "skip the 94 duplicates"
	// must not create them because the decision lived only in the request.
	// Empty means the contract's default, `create`.
	OnDuplicate string `json:"on_duplicate,omitempty"`
	// ContextTag files every record this run CREATES under one word, so a batch
	// stays findable as a batch. It rides with the mapping for the same reason
	// OnDuplicate does: the commit happens on a later request than the one that
	// chose it, and a decision that lived only in the create call would be lost
	// by the time anything wrote a row.
	ContextTag string `json:"context_tag,omitempty"`
	// Parts is a ZIP run's files, one object each; empty for a single CSV,
	// whose one part is Object, Fields and SourceKey above.
	Parts []PartMapping `json:"parts,omitempty"`
	// SourceName is the namespace the run's provenance is filed under.
	SourceName string `json:"source_name,omitempty"`
	// FileName is the uploaded file's name, for display only: nothing about
	// the run is decided by it.
	FileName     string `json:"file_name,omitempty"`
	OnBrokenLink string `json:"on_broken_link,omitempty"`
}

// PartMapping is one file of a run: which object its rows are, which column
// lands in which field, and which column identifies a row.
type PartMapping struct {
	File      string            `json:"file"`
	Object    string            `json:"object"`
	Fields    map[string]string `json:"fields"`
	SourceKey string            `json:"source_key"`
}

// PartList answers the run's parts, so a reader walks a single CSV and a ZIP
// the same way: a single-CSV mapping is one part.
func (m RunMapping) PartList() []PartMapping {
	if len(m.Parts) > 0 {
		return m.Parts
	}
	return []PartMapping{{File: m.FileName, Object: m.Object, Fields: m.Fields, SourceKey: m.SourceKey}}
}

// CreateStagedRunInput opens a run that must be approved before it writes.
type CreateStagedRunInput struct {
	Connector string
	SourceRef string
	Source    string
	Mapping   RunMapping
}

// CreateStagedRun opens a run in `validating`: the row exists, carries its
// mapping, and has written nothing to the estate. The dry run reports against
// it and AwaitApproval hands it to a human.
func (s *RunStore) CreateStagedRun(ctx context.Context, in CreateStagedRunInput) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionCreate); err != nil {
		return Run{}, err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Run{}, err
	}
	mapping, err := json.Marshal(in.Mapping)
	if err != nil {
		return Run{}, fmt.Errorf("encoding the import mapping: %w", err)
	}

	var run Run
	id := RunID(ids.NewV7())
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// A new test run replaces the unwritten runs of its name and of its upload,
		// which each hold one active run.
		if err := supersedeName(ctx, tx, id, in.Mapping.SourceName); err != nil {
			return err
		}
		if err := supersedeWhere(ctx, tx, id, "source_ref", in.SourceRef); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO import_run (id, connector, status, mapping, source_ref, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, connector, status, source_ref, checkpoint, created_at, updated_at`,
			id, in.Connector, StatusValidating, mapping, in.SourceRef, in.Source, capturedBy)
		if err := scanRun(row, &run); storekit.IsUniqueViolation(err) {
			return fmt.Errorf("the upload is held by an import run that has written: %w", apperrors.ErrConflict)
		} else if err != nil {
			return fmt.Errorf("creating import run: %w", err)
		}
		if _, err := storekit.Audit(ctx, tx, "create", ImportRunObject, run.ID, nil, map[string]any{
			"connector": run.Connector, auditFieldStatus: run.Status, "source_ref": run.SourceRef,
			"object": in.Mapping.Object,
		}); err != nil {
			return err
		}
		// Retired on THIS transaction, now that a run names the source. A profile
		// the contact abandons never reaches here, which is the point: the bytes
		// were stored to be profiled and nothing adopts them, so the ledger
		// collects them a grace period later instead of keeping them forever.
		return storedobjects.Clear(ctx, tx, in.SourceRef)
	})
	if err != nil {
		return Run{}, err
	}
	run.Mapping = &in.Mapping
	run.CapturedBy = capturedBy
	return run, nil
}

// AwaitApproval records what the dry run found and parks the run for a human.
// Valid only from `validating`: a run that already moved on is not the one the
// report describes.
func (s *RunStore) AwaitApproval(ctx context.Context, id RunID, report Report) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("encoding the dry-run report: %w", err)
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		return stageTransition(ctx, tx, id, moveReport, runChange{values: []runValue{{"report", encoded}}})
	})
}

// Approve moves a validated run to `running`.
//
// Valid ONLY from `awaiting_approval`, and the refusal is the point: approving
// a run that is already running, complete or failed means the approver is
// acting on a state that is not the one they judged. The conflict says so
// rather than starting a second pass over the same file.
func (s *RunStore) Approve(ctx context.Context, id RunID) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	approver, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Run{}, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		return stageTransition(ctx, tx, id, moveApprove, runChange{
			values:   []runValue{{"approved_by", approver}},
			literals: []string{"approved_at = now()"},
			after:    map[string]any{"approved_by": approver},
		})
	})
	if err != nil {
		return Run{}, err
	}
	return s.GetStaged(ctx, id)
}

// ResumeApproved continues a run that failed part-way, from its checkpoint.
//
// This is what IEM-WIRE-6 means by a resumable failure being "a resumable
// state, not a dead end". Without it an interrupted import can only be
// abandoned and re-uploaded. Only an approved run resumes: a failed run
// nobody approved never reached a commit.
func (s *RunStore) ResumeApproved(ctx context.Context, id RunID) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return Run{}, err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return stageTransition(ctx, tx, id, moveResume, runChange{approved: true, literals: []string{"error = NULL"}})
	})
	if err != nil {
		return Run{}, err
	}
	return s.GetStaged(ctx, id)
}

// FailValidation records a dry run that could not finish, with a fixed cause.
// Valid only from `validating`, so it can never overwrite the outcome of a run
// that has since been approved and committed.
func (s *RunStore) FailValidation(ctx context.Context, id RunID) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE import_run SET status = $2, error = $3, updated_at = now()
			 WHERE id = $1 AND status = $4`, id, StatusFailed, CauseValidationStopped, StatusValidating)
		if err != nil {
			return fmt.Errorf("failing import run %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return s.explainMiss(ctx, tx, id, StatusValidating, StatusFailed)
		}
		_, err = storekit.Audit(ctx, tx, "update", ImportRunObject, id,
			map[string]any{auditFieldStatus: StatusValidating}, map[string]any{auditFieldStatus: StatusFailed})
		return err
	})
}

// explainMiss turns a transition that moved nothing into the honest refusal:
// not-found when this installation has no such run, conflict when it does but
// the run had already moved on.
func (s *RunStore) explainMiss(ctx context.Context, tx pgx.Tx, id RunID, from, to string) error {
	var status string
	err := tx.QueryRow(ctx, `
		SELECT status FROM import_run
		 WHERE id = $1`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("import run %s: %w", id, apperrors.ErrNotFound)
	}
	if err != nil {
		return fmt.Errorf("reading import run %s: %w", id, err)
	}
	return fmt.Errorf("import run %s is %s, not %s, so it cannot become %s: %w", id, status, from, to, apperrors.ErrConflict)
}

// GetStaged reads one run with its mapping and report — everything the status
// and report surfaces answer from. A run outside the caller's scope answers
// not-found, never forbidden.
func (s *RunStore) GetStaged(ctx context.Context, id RunID) (Run, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionRead); err != nil {
		return Run{}, err
	}
	var run Run
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		run, err = readStaged(ctx, tx, id, true)
		return err
	})
	if err != nil {
		return Run{}, err
	}
	return run, nil
}

// readStaged is GetStaged on the caller's transaction. The profile is read
// only when asked for: a list never carries it.
func readStaged(ctx context.Context, tx pgx.Tx, id RunID, withProfile bool) (Run, error) {
	columns := stagedColumns
	if withProfile {
		columns += `, profile`
	}
	run, err := scanStaged(tx.QueryRow(ctx, `SELECT `+columns+` FROM import_run WHERE id = $1`, id), withProfile)
	if errors.Is(err, pgx.ErrNoRows) {
		return Run{}, fmt.Errorf("import run %s: %w", id, apperrors.ErrNotFound)
	}
	if err != nil {
		return Run{}, fmt.Errorf("reading import run %s: %w", id, err)
	}
	return run, nil
}

// ListStaged reads one page of CSV runs, newest first, each as GetStaged reads
// one. The flip's own runs are left out: they carry no mapping this surface
// describes. An empty cursor starts at the newest run.
func (s *RunStore) ListStaged(ctx context.Context, cursor string, limit int) ([]Run, storekit.Page, error) {
	// The exported name, not the alias: the grant-reachability census resolves
	// only a literal-valued constant.
	if err := auth.Require(ctx, ImportRunObject, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := storekit.SQLf(`connector = $%d`, arg(ConnectorCSV))
	if cursor != "" {
		after, err := storekit.DecodeCursor(cursor)
		if err != nil {
			return nil, storekit.Page{}, err
		}
		where += storekit.SQLf(` AND (created_at, id) < ($%d, $%d)`, arg(after.CreatedAt), arg(after.ID))
	}
	query := `SELECT ` + stagedColumns + ` FROM import_run WHERE ` + where +
		storekit.SQLf(` ORDER BY created_at DESC, id DESC LIMIT $%d`, arg(limit+1))
	runs := []Run{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("listing import runs: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			run, err := scanStaged(rows, false)
			if err != nil {
				return err
			}
			runs = append(runs, run)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	if len(runs) <= limit {
		return runs, storekit.Page{}, nil
	}
	runs = runs[:limit]
	last := runs[limit-1]
	next, err := storekit.EncodeCursor(last.CreatedAt, last.ID)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return runs, storekit.Page{HasMore: true, NextCursor: next}, nil
}

// stagedColumns is what scanStaged reads, in its order.
const stagedColumns = `id, connector, status, source_ref, checkpoint, created_at, updated_at,
			mapping, report, undo_report, error, captured_by,
			version, coalesce(draft_step, ''), coalesce(approved_by, ''), approved_at, superseded_by,
			source_released_at IS NOT NULL`

func scanStaged(row pgx.Row, withProfile bool) (Run, error) {
	var (
		run        Run
		mapping    []byte
		report     []byte
		undoReport []byte
		runErr     *string
	)
	dest := []any{&run.ID, &run.Connector, &run.Status, &run.SourceRef, &run.Checkpoint,
		&run.CreatedAt, &run.UpdatedAt, &mapping, &report, &undoReport, &runErr, &run.CapturedBy,
		&run.Version, &run.DraftStep, &run.ApprovedBy, &run.ApprovedAt, &run.SupersededBy, &run.SourceReleased}
	if withProfile {
		dest = append(dest, &run.Profile)
	}
	if err := row.Scan(dest...); err != nil {
		return Run{}, err
	}
	if runErr != nil {
		run.Error = *runErr
	}
	if len(mapping) > 0 {
		var m RunMapping
		if err := json.Unmarshal(mapping, &m); err != nil {
			return Run{}, fmt.Errorf("reading the mapping of import run %s: %w", run.ID, err)
		}
		// An empty object is what the flip's own runs carry; only a mapping
		// that actually names one is handed on, so a caller cannot mistake
		// "no mapping" for "a mapping of nothing".
		if m.Object != "" || len(m.Parts) > 0 {
			run.Mapping = &m
		}
	}
	if len(report) > 0 {
		var rep Report
		if err := json.Unmarshal(report, &rep); err != nil {
			return Run{}, fmt.Errorf("reading the report of import run %s: %w", run.ID, err)
		}
		run.Report = &rep
	}
	if len(undoReport) > 0 {
		var rep UndoReport
		if err := json.Unmarshal(undoReport, &rep); err != nil {
			return Run{}, fmt.Errorf("reading the undo report of import run %s: %w", run.ID, err)
		}
		run.UndoReport = &rep
	}
	return run, nil
}

// UnreferencedImportKeys answers which of these keys no unreleased import run's
// source names: a run releases its file once no later step reads it.
//
// Answered here because import_run is this module's table. An abandoned profile
// legitimately has no run: the key is handed to the client, and only a submitted
// mapping makes it durable — so "unreferenced" is the ordinary outcome rather than
// a fault, and a grace period is what separates abandoned from in-flight.
func (s *RunStore) UnreferencedImportKeys(ctx context.Context, keys []string) ([]string, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	var out []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT k FROM unnest($1::text[]) AS k
			 WHERE NOT EXISTS (
			       SELECT 1 FROM import_run r WHERE r.source_ref = k AND r.source_released_at IS NULL)`, keys)
		if err != nil {
			return fmt.Errorf("check which import source keys are unreferenced: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return fmt.Errorf("read an unreferenced import source key: %w", err)
			}
			out = append(out, key)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RecordImportSourceIntent declares an uploaded source's key provisional.
//
// The same grant the upload itself takes: this row exists because that upload is
// about to put bytes, and a caller who may not create an import run has no business
// recording one's source either.
func (s *RunStore) RecordImportSourceIntent(ctx context.Context, key string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionCreate); err != nil {
		return err
	}
	return storedobjects.NewLedger(s.db).Record(ctx, storedobjects.KindImport, key)
}

// BindingsOf is LookupBinding for every key one source has landed of one
// object, keyed by external id. A dry run reads them once instead of once per
// row.
func (s *RunStore) BindingsOf(ctx context.Context, sourceSystem, object string) (map[string]Binding, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionRead); err != nil {
		return nil, err
	}
	out := map[string]Binding{}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT m.external_id, m.native_id, r.status FROM import_record_map m
			  JOIN import_run r ON r.id = m.import_run_id
			WHERE m.source_system = $1 AND m.object = $2`, sourceSystem, object)
		if err != nil {
			return fmt.Errorf("migration: reading the %s identities of %s: %w", object, sourceSystem, err)
		}
		defer rows.Close()
		for rows.Next() {
			var external string
			var binding Binding
			if err := rows.Scan(&external, &binding.NativeID, &binding.RunStatus); err != nil {
				return fmt.Errorf("migration: reading the %s identities of %s: %w", object, sourceSystem, err)
			}
			out[external] = binding
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
