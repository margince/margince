// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Putting one audited change back is an ORDINARY update. It goes through the
// record's own update path carrying the `restore` verb and a link to the row it
// reverses, so every rule that path holds — RBAC, row scope, the write shape,
// the audit chokepoint, the paired event — still holds without being restated.
// A second write engine here is how the audited path and the reversal path
// would come to disagree about what a write means.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/auditverb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// undidAuditLogID is privacy's spelling of the evidence key naming the row a
// restore reverses. The writer here and the reader there must agree on it, so
// there is one constant and the write side imports it rather than typing it.
const undidAuditLogID = privacy.UndidAuditLogID

// RestoreSeam is the reversal executor. It holds the evaluator's ports and the
// update seam, and nothing else: everything it needs to decide lives on the
// audit spine, and everything it needs to write lives behind Update.
type RestoreSeam struct {
	pool       *pgxpool.Pool
	dispatcher *Dispatcher
	evaluator  Evaluator
	// visible is the record's row-scope gate. It is a field rather than a
	// direct call so the property that matters can be held by a test: that a
	// caller who may not see the record is answered 404 and never a refusal,
	// whatever the gate happens to allow in a given fixture.
	visible func(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID) error
	// edges performs a LINK's inverse. It is a port because `relationship` is the
	// contacts module's table, and the rules an edge write obeys are that module's.
	edges EdgeReverser
	// corrections answers whether an audit row is a machine correction with its
	// own reversal, and performs that reversal — the SAME store magicUndoJudge
	// asks the identical question of, so the line offering an Undo and the
	// write performing it can never name different rows undoable. Nil is a
	// real state: an installation wired without it simply has no corrections
	// to offer, and every row falls through to the generic evaluator below,
	// exactly as it always has.
	corrections *deals.Store
	// afterEdgeDecision is entered between the binding edge decision and the edge
	// write — the ONE window in which a second reverser of the same link can
	// overtake this one. It exists so a test can hold the path open there, and it
	// is unexported and never set by NewRestoreSeam, so no production caller can
	// reach it. Without it a race test only proves whatever the scheduler happened
	// to do that run, which is the same as proving nothing.
	afterEdgeDecision func()
}

// Restore puts the named audit row's before-image back.
//
// The binding evaluation runs in its own transaction immediately before the
// write, and `ifVersion` is what closes the gap rather than a lock held across
// two transactions. Every state change that could alter the answer — a later
// field write, a restore of this row, an erasure, an archive — writes the
// record and bumps its version, so a decision made on a stale reading cannot
// commit: the update refuses with ErrVersionSkew and nothing is written. That
// is why If-Match is required on this route and optional everywhere else.
func (s RestoreSeam) Restore(ctx context.Context, entityType string, id, auditID ids.UUID, ifVersion int64) (privacy.RecordHistoryEntry, error) {
	row, err := s.readRow(ctx, entityType, id, auditID)
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	// The TARGET row's own entity_type decides the mechanism, and it is not the
	// path's: a link's rows sit on ('relationship', edge_id) and appear on the
	// history of both records it joins.
	if row.EntityType == edgeEntityType {
		return s.reverseEdge(ctx, entityType, id, row, ifVersion)
	}
	// THE CORRECTION PATH FIRST, the same order magicUndoJudge.judgeOne reads
	// the receipt by: a machine correction has its own reversal, and the
	// generic evaluator refuses exactly those rows (they write a field the
	// ordinary update shape cannot spell), so asking it first would refuse
	// every change the receipt just told the caller it could undo. ifVersion
	// still travels all the way down to it: RevertCorrection's own per-field
	// conflict check is deliberately looser than a whole-record pin (a rename
	// leaves an undo of an unrelated date correction available — see
	// deals/correctionrevert.go's own header), but it compares VALUES, not
	// versions, so an unrelated field edited away and back to what the
	// correction would itself produce needs the version pin to be caught at
	// all.
	if entry, decided, err := s.reverseCorrection(ctx, entityType, id, row, ifVersion); err != nil || decided {
		return entry, err
	}
	patch, err := s.decide(ctx, row)
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	if err := s.write(ctx, row, patch, ifVersion); err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	return s.readRestoreEntry(ctx, entityType, id, auditID)
}

// reverseCorrection answers for an audit row that is a machine correction's
// own entry, and reports whether it decided at all — false for every entry
// this branch does not serve, so Restore falls through to the generic
// evaluator exactly as it always has.
//
// The lookup asks the SAME question magicUndoJudge.judgeCorrection asks of
// the SAME store: a deal-scoped row with a live deal_correction naming it.
// Anything else — a contact, a company, a deal edit a human made — is
// answered false here and decided by the generic evaluator below, same as
// before this branch existed.
func (s RestoreSeam) reverseCorrection(
	ctx context.Context, entityType string, id ids.UUID, row AuditRow, ifVersion int64,
) (privacy.RecordHistoryEntry, bool, error) {
	if s.corrections == nil || row.EntityType != entityTypeDeal {
		return privacy.RecordHistoryEntry{}, false, nil
	}
	var correction deals.DealCorrection
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		correction, err = s.corrections.CorrectionForAudit(ctx, tx, row.ID)
		return err
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return privacy.RecordHistoryEntry{}, false, nil
	}
	if err != nil {
		return privacy.RecordHistoryEntry{}, true, err
	}
	// The same word judgeCorrection already offers the reader: a correction
	// somebody else took back in the gap between the read and this write
	// refuses the same way a page load after that reversal would have.
	if correction.Reversed() {
		return privacy.RecordHistoryEntry{}, true, RefusedRestore{Reason: ReasonAlreadyUndone}
	}
	if _, err := s.corrections.RevertCorrection(ctx, correction.ID, &ifVersion,
		map[string]any{undidAuditLogID: row.ID.String()}); err != nil {
		// RevertCorrection's own refusals (already reversed under its own
		// lock, a later edit to a corrected field, or its audit row aged out
		// of history) are a 409 on every other route this seam serves — the
		// receipt read this line as undoable, so a plain 500 here would be a
		// harder disagreement between the two than the refusal itself is.
		var conflict *deals.CorrectionReversalError
		if errors.As(err, &conflict) {
			return privacy.RecordHistoryEntry{}, true,
				RefusedRestore{Reason: ReasonNotRestorableByThisPath, Detail: conflict.Reason}
		}
		return privacy.RecordHistoryEntry{}, true, err
	}
	entry, err := s.readRestoreEntry(ctx, entityType, id, row.ID)
	return entry, true, err
}

// readRow loads the target entry — an entry of the path record's HISTORY, which
// is not the same thing as a row the path record owns.
//
// Two identities, and keeping them apart is what this function is for. The
// record's row-scope gate is taken FIRST, and its error is returned unchanged:
// reading the audit row before asking whether the caller may see the record
// would answer "this change cannot be put back" for a record they are not
// allowed to know exists, and a caller who can tell a refusal from a 404 can
// tell a hidden record from an absent one. The audit row is then admitted by
// MEMBERSHIP of that record's history — privacy's own admission, which for a
// link is endpoint membership plus the other end's visibility and erasure.
//
// Bound instead to `entity_type = $2 AND entity_id = $3`, a link's row is never
// found and no link is reversible. Admitted by its id alone, a caller holding an
// audit id could probe for a link whose other end they may not see. Either way
// the answer is ErrNotFound, never a 403: a refusal is proof the row exists.
func (s RestoreSeam) readRow(ctx context.Context, entityType string, id, auditID ids.UUID) (AuditRow, error) {
	var row AuditRow
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.visible(ctx, tx, entityType, id); err != nil {
			return err
		}
		served, err := privacy.HistoryServesEntry(ctx, tx, entityType, id, auditID)
		if err != nil {
			return err
		}
		if !served {
			return apperrors.ErrNotFound
		}
		return tx.QueryRow(
			ctx, `
			SELECT id, entity_type, entity_id, action, before, after, occurred_at
			FROM audit_log
			WHERE id = $1`, auditID,
		).Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action, &row.Before, &row.After, &row.OccurredAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuditRow{}, apperrors.ErrNotFound
		}
		return AuditRow{}, err
	}
	return row, nil
}

// decide runs the binding evaluation and returns the patch a restore may send.
// A refusal is an apperrors.ErrConflict carrying the reason, so the surface
// renders the same word the disabled button showed.
func (s RestoreSeam) decide(ctx context.Context, row AuditRow) (map[string]json.RawMessage, error) {
	var answer Undoability
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		answer, err = s.evaluator.Evaluate(ctx, tx, row, Binding)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("compose: decide whether the change can be put back: %w", err)
	}
	if !answer.Undoable {
		return nil, RefusedRestore{Reason: answer.Reason, Detail: answer.Detail}
	}
	// The evaluator has already refused an image that cannot be spelled in
	// full, so anything unspellable here would be a disagreement between the
	// two rather than a state to handle.
	patch, _, err := filterImage(row.EntityType, row.Before)
	if err != nil {
		return nil, err
	}
	return patch, nil
}

// write sends the filtered image back through the record's own update path.
func (s RestoreSeam) write(ctx context.Context, row AuditRow, patch map[string]json.RawMessage, ifVersion int64) error {
	// The nulls leave the patch and travel as named clears: a JSON null in the
	// body decodes to a nil pointer and reads as "not supplied", so sending one
	// would report success and change nothing.
	values, cleared, unclearable := splitNulls(row.EntityType, patch)
	if len(unclearable) > 0 {
		// The evaluator refuses these before the write. Reaching here means the
		// two disagree, which is worth a fault rather than a silent no-op.
		return fmt.Errorf("compose: %s cannot clear %v, and the evaluator admitted it",
			row.EntityType, unclearable)
	}
	body, err := json.Marshal(values)
	if err != nil {
		return fmt.Errorf("compose: assemble the restore patch: %w", err)
	}
	_, err = s.dispatcher.Update(ctx, datasource.UpdateInput{
		Ref:       datasource.EntityRef{Type: datasource.EntityType(row.EntityType), ID: row.EntityID},
		Patch:     json.RawMessage(body),
		IfVersion: &ifVersion,
		Clear:     cleared,
		Trail: auditverb.Trail{
			Verb:     auditverb.Restore,
			Evidence: map[string]any{undidAuditLogID: row.ID.String()},
		},
	})
	return err
}

// readRestoreEntry returns the restore's own history line, looked up by the
// evidence link rather than by "the newest restore on this record": a record
// with several reversals has several, and taking the newest would hand the
// caller somebody else's line whenever two contacts press Undo in the same
// moment.
func (s RestoreSeam) readRestoreEntry(ctx context.Context, entityType string, id, auditID ids.UUID) (privacy.RecordHistoryEntry, error) {
	entry, err := privacy.ReadRestoreOf(ctx, InstallationDB(s.pool), entityType, id, auditID)
	if errors.Is(err, apperrors.ErrNotFound) {
		// The write committed and left no reversal row, which the update path
		// does when the patch changes nothing. Answering 404 here would say the
		// entry does not exist, when what happened is that the record already
		// held these values — so the contact is told that instead.
		return privacy.RecordHistoryEntry{}, RefusedRestore{
			Reason: ReasonNotRestorableByThisPath,
			Detail: "the record already holds these values, so there was nothing to put back",
		}
	}
	return entry, err
}

// RefusedRestore is a refusal the surface can render. It carries the reason
// WORD rather than prose, because the disabled button and the 409 must say the
// same thing — a contact who reads one sentence before pressing and a different
// one after has been told the product changed its mind.
type RefusedRestore struct {
	Reason Reason
	Detail string
}

func (e RefusedRestore) Error() string {
	if e.Detail == "" {
		return "this change cannot be put back: " + string(e.Reason)
	}
	return "this change cannot be put back: " + string(e.Reason) + " (" + e.Detail + ")"
}

// Unwrap renders the refusal as the fault the contract promises: 409 whose
// `code` IS the reason. Falling back to the generic conflict sentinel would put
// `code: "conflict"` on the wire and leave the reason inside free text, so a
// client built to the enum would match nothing and render generic copy — the
// disabled button and the 409 saying different things, which is the one thing
// the refusal set exists to prevent. The detail names only the caller's own
// record's field names.
func (e RefusedRestore) Unwrap() error {
	return &httperr.DetailedError{
		Status: http.StatusConflict,
		Code:   string(e.Reason),
		Detail: e.Error(),
	}
}
