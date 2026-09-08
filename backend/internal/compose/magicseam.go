// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Wiring the machinery's receipt.
//
// The one edge that needs explaining is the brief: the window this surface
// reports over defaults to when the night last READ the records, which is what
// the reader has already seen. That instant lives in compose/briefs, a sibling
// package, so it arrives through a seam rather than an import.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/magic"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newMagicService assembles the receipt's read.
func newMagicService(pool *pgxpool.Pool, now func() time.Time) *magic.Service {
	db := InstallationDB(pool)
	return magic.NewService(pool, magicBriefCutoff{
		engine: briefs.NewBriefEngine(pool, nil),
		now:    now,
	}, now).
		WithTroubledRuns(automation.NewAutomationStore(db))
}

// magicBriefCutoff answers when the acting rep's night last read the records.
//
// The run's as_of, not its generated_at. A run written at 06:42 over records
// read at 06:00 has a 42-minute window in which the machinery kept working, and
// reporting from the write time would hide exactly the overnight work this
// surface exists to show.
type magicBriefCutoff struct {
	engine *briefs.BriefEngine
	now    func() time.Time
}

// CutoffFor answers the cutoff and whether a run exists at all.
//
// No run is not a failure: the reader simply has no brief to date the window
// from, and the service falls back to a day. The refusal is reported as
// not-found rather than as an error for the same reason — a rep who has never
// had a brief is an ordinary state, not a broken read.
func (m magicBriefCutoff) CutoffFor(ctx context.Context) (time.Time, bool, error) {
	run, err := m.engine.LatestRun(ctx, m.now())
	if errors.Is(err, apperrors.ErrNotFound) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return run.AsOf, true, nil
}

// magicUndoJudge answers the receipt's "can this be taken back" from the SAME
// evaluator the restore route itself uses.
//
// One evaluator, deliberately. The receipt's offer and the write's refusal have
// to agree — a line promising an Undo the writer then declines is worse than no
// line at all — and two constructions of the same judgment drift the moment
// somebody adds a rule to one of them.
type magicUndoJudge struct {
	seam RestoreSeam
}

// JudgeUndo reads the audit row and asks the evaluator in ADVISORY mode.
//
// Advisory because this is a page being drawn, not a change being made: the
// binding answer is taken again inside the reversal's own transaction, under
// the row lock, where it can refuse a caller who lost a race this read could
// not have seen.
func (j magicUndoJudge) JudgeUndo(
	ctx context.Context, tx pgx.Tx, auditID ids.UUID, entityType string,
) (bool, string, error) {
	if !servesRecordType(entityType) {
		return false, string(ReasonUnsupportedRecordType), nil
	}
	// Read IN THE CALLER'S transaction. RestoreSeam.readRow opens its own,
	// which nested inside the page's would take a second connection while this
	// one holds locks — the deadlock every reader in this tree is written to
	// avoid.
	var row AuditRow
	err := tx.QueryRow(ctx, `
		SELECT id, entity_type, entity_id, action, before, after, occurred_at
		  FROM audit_log
		 WHERE id = $1`, auditID).
		Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action,
			&row.Before, &row.After, &row.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, string(ReasonUnsupportedRecordType), nil
	}
	if err != nil {
		return false, "", err
	}
	// The same two gates readRow takes, in the same order: the record's own row
	// scope first, then membership of its history. A line the reader may not
	// see must not learn it exists from the shape of an undo answer — and the
	// done lane is already scoped, so this is belt and braces rather than the
	// only guard.
	// A scope MISS is an answer — no control for a record this reader cannot
	// change — while a broken read is not, and the two must not collapse into
	// the same silent "no". Swallowing both would hide a failing gate behind a
	// greyed button nobody questions.
	if err := j.seam.visible(ctx, tx, row.EntityType, row.EntityID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return false, string(ReasonUnsupportedRecordType), nil
		}
		return false, "", err
	}
	served, err := privacy.HistoryServesEntry(ctx, tx, row.EntityType, row.EntityID, auditID)
	if err != nil {
		return false, "", err
	}
	if !served {
		return false, string(ReasonUnsupportedRecordType), nil
	}
	answer, err := j.seam.evaluator.Evaluate(ctx, tx, row, Advisory)
	if err != nil {
		return false, "", err
	}
	if answer.Undoable {
		return true, "", nil
	}
	return false, string(answer.Reason), nil
}
