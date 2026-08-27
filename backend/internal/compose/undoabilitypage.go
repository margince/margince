// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Answering undoability for a PAGE of history without asking per entry.
//
// A page is one record's rows, so what the refusals need from the RECORD is
// read once however many entries the page holds: whether it is archived, and
// whether the caller may change it. The rows themselves with their erasure
// boundary, and the entries a live reversal already covers, are one query each
// over the whole page.
//
// Supersession and the custom-field catalog check are NOT among those. They ask
// about one entry's own keys from one entry's own position in the trail, so
// they run per row that reaches them — a page whose entries are mostly
// refused earlier pays for few, and a page of undoable entries pays for all of
// them. Folding those two into the page would mean a second spelling of each,
// and two spellings of supersession is precisely what superseded.go exists to
// argue against.
//
// What is NOT done is a lazy per-entry lookup, which was rejected outright: it
// produces a button whose state is unknown until the user interacts with it,
// the greyed-button-with-no-reason shape this feature exists to remove.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// UndoabilityPage answers undoability for one record's history page. compose
// owns it because the answer needs the update shapes and the record's own
// table, neither of which the history module may reach.
type UndoabilityPage struct {
	seam RestoreSeam
}

// NewUndoabilityPage reads through the same evaluator the write binds with, so
// the advisory and the binding answers are one set of branches rather than two
// that drift.
func NewUndoabilityPage(seam RestoreSeam) UndoabilityPage { return UndoabilityPage{seam: seam} }

// ForRecord answers each audit row the page touches. A row absent from the
// result was not judged, which the caller renders as undoable=false with no
// reason rather than as undoable.
func (p UndoabilityPage) ForRecord(ctx context.Context, entityType string, entityID ids.UUID, auditIDs []ids.UUID) (map[ids.UUID]privacy.UndoabilityAnswer, error) {
	answers := make(map[ids.UUID]privacy.UndoabilityAnswer, len(auditIDs))
	if len(auditIDs) == 0 {
		return answers, nil
	}
	err := database.WithWorkspaceTx(ctx, p.seam.pool, func(tx pgx.Tx) error {
		shared, err := p.recordFacts(ctx, tx, entityType, entityID)
		if err != nil {
			return err
		}
		undone, err := p.liveReversals(ctx, tx, entityType, entityID)
		if err != nil {
			return err
		}
		rows, err := p.pageRows(ctx, tx, entityType, entityID, auditIDs)
		if err != nil {
			return err
		}
		for _, row := range rows {
			answer, err := p.judge(ctx, tx, row, shared, undone)
			if err != nil {
				return err
			}
			answers[row.ID] = answer
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("compose: undoability for a history page: %w", err)
	}
	return answers, nil
}

// recordFacts are the answers that belong to the RECORD rather than to an
// entry, so a page pays for them once however many rows it holds.
type recordFacts struct {
	archived bool
	writable bool
}

func (p UndoabilityPage) recordFacts(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) (recordFacts, error) {
	archived, err := recordIsArchived(ctx, tx, entityType, entityID)
	if err != nil {
		return recordFacts{}, err
	}
	// The REFUSAL is reduced to a boolean: this asks whether the caller COULD
	// write, and "no" is the answer rather than a failure.
	// Carrying it further would put "not yours" versus "does not exist" one
	// careless log line away from a caller. A fault is a different thing and is
	// returned: reduced to false it would claim the caller has no write
	// authority on every entry of the page.
	err = recordIsWritableByCaller(ctx, tx, entityType, entityID)
	if err != nil && !isWriteScopeRefusal(err) {
		return recordFacts{}, err
	}
	return recordFacts{archived: archived, writable: err == nil}, nil
}

// pageRow is an AuditRow with the boundary answer the page query already found.
type pageRow struct {
	AuditRow
	behindErasure bool
}

// pageRows reads the page's audit rows in ONE query, carrying the erasure
// boundary on each through privacy's OWN predicate. Restating that predicate
// here is how two readers of one erasure come to disagree about where it sits,
// and an Art. 17 boundary is not a rule where almost-the-same is survivable.
func (p UndoabilityPage) pageRows(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID, auditIDs []ids.UUID) ([]pageRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.entity_type, a.entity_id, a.action, a.before, a.after, a.occurred_at,
		       NOT (`+privacy.UnscrubbedImageSQL("a", "$3")+`) AS behind_erasure
		FROM audit_log a
		WHERE a.entity_type = $1 AND a.entity_id = $2 AND a.id = ANY($4::uuid[])`,
		entityType, entityID, privacy.ScrubVerbs(), auditIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pageRow
	for rows.Next() {
		var row pageRow
		if err := rows.Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action,
			&row.Before, &row.After, &row.OccurredAt, &row.behindErasure); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// liveReversals is the set of audit rows a live restore already reverses — one
// query for the page, riding idx_audit_entity on the record's own rows. A
// reversal that has itself been reversed is not live, which is what keeps the
// trail navigable in both directions rather than a one-way ratchet.
func (p UndoabilityPage) liveReversals(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT undo.evidence ->> $3 FROM audit_log undo
		WHERE undo.entity_type = $1 AND undo.entity_id = $2
		  AND undo.evidence ->> $3 IS NOT NULL
		  AND NOT EXISTS (
		    SELECT 1 FROM audit_log reundo
		    WHERE reundo.entity_type = undo.entity_type
		      AND reundo.entity_id = undo.entity_id
		      AND reundo.evidence ->> $3 = undo.id::text)`,
		entityType, entityID, privacy.UndidAuditLogID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// A key present but JSON null reads as SQL NULL, which cannot scan into a
	// string: the query filters it out above rather than failing the whole page
	// over one malformed row. The sibling queries COMPARE this expression
	// instead of scanning it, and a comparison against NULL is already false.
	undone := map[string]bool{}
	for rows.Next() {
		var undid string
		if err := rows.Scan(&undid); err != nil {
			return nil, err
		}
		undone[undid] = true
	}
	return undone, rows.Err()
}

// advisoryEvaluator is the page's evaluator: every port the WRITE binds, with
// the four the page already holds facts for answered from those facts. Derived
// from the binding evaluator rather than assembled beside it, so a port added
// to the write is bound here without anyone remembering — which is how the page
// came to omit ExternallyGoverned and light a restore button the write refuses.
// Held by TestTheAdvisoryPathBindsEveryPortTheWriteBinds.
func advisoryEvaluator(binding Evaluator, shared recordFacts, row pageRow, undone map[string]bool) Evaluator {
	advisory := binding
	advisory.Archived = func(context.Context, pgx.Tx, string, ids.UUID) (bool, error) {
		return shared.archived, nil
	}
	advisory.Writable = func(context.Context, pgx.Tx, string, ids.UUID) error {
		if shared.writable {
			return nil
		}
		return errRecordNotWritable
	}
	advisory.BehindErasure = func(context.Context, pgx.Tx, AuditRow) (bool, error) {
		return row.behindErasure, nil
	}
	advisory.AlreadyUndone = func(_ context.Context, _ pgx.Tx, r AuditRow) (bool, error) {
		return undone[r.ID.String()], nil
	}
	return advisory
}

// judge asks the same branches the write binds, in the same order, with the
// page-level facts already in hand. The ports answer from those facts rather
// than querying, which is what makes the page flat in its size while leaving
// ONE set of branches to keep correct.
func (p UndoabilityPage) judge(ctx context.Context, tx pgx.Tx, row pageRow,
	shared recordFacts, undone map[string]bool,
) (privacy.UndoabilityAnswer, error) {
	advisory := advisoryEvaluator(p.seam.evaluator, shared, row, undone)
	answer, err := advisory.Evaluate(ctx, tx, row.AuditRow, Advisory)
	if err != nil {
		return privacy.UndoabilityAnswer{}, err
	}
	return privacy.UndoabilityAnswer{
		Undoable: answer.Undoable,
		Reason:   string(answer.Reason),
		Detail:   answer.Detail,
	}, nil
}

// errRecordNotWritable stands for the row-scope refusal without carrying it.
// The evaluator only asks whether the port returned an error, and the real one
// separates "not yours" from "does not exist" — the distinction the row-scope
// gate keeps hidden.
var errRecordNotWritable = fmt.Errorf("the caller may not change this record")
