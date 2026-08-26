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

	"github.com/gradionhq/margince/backend/internal/modules/privacy"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/httperr"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/auditverb"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/datasource"
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
	patch, err := s.decide(ctx, row)
	if err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	if err := s.write(ctx, row, patch, ifVersion); err != nil {
		return privacy.RecordHistoryEntry{}, err
	}
	return s.readRestoreEntry(ctx, entityType, id, auditID)
}

// readRow loads the target entry, bound to the path's own record.
//
// The record's row-scope gate is taken FIRST, and its error is returned
// unchanged. Reading the audit row before asking whether the caller may see the
// record would answer "this change cannot be put back" for a record they are
// not allowed to know exists — and a caller who can tell a refusal from a 404
// can tell a hidden record from an absent one, which is the whole of what the
// row-scope rule hides.
//
// An audit row belonging to a DIFFERENT record is ErrNotFound for the same
// reason, never a 403.
func (s RestoreSeam) readRow(ctx context.Context, entityType string, id, auditID ids.UUID) (AuditRow, error) {
	var row AuditRow
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := s.visible(ctx, tx, entityType, id); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT id, entity_type, entity_id, action, before, occurred_at
			FROM audit_log
			WHERE id = $1 AND entity_type = $2 AND entity_id = $3`,
			auditID, entityType, id,
		).Scan(&row.ID, &row.EntityType, &row.EntityID, &row.Action, &row.Before, &row.OccurredAt)
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
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("compose: assemble the restore patch: %w", err)
	}
	_, err = s.dispatcher.Update(ctx, datasource.UpdateInput{
		Ref:       datasource.EntityRef{Type: datasource.EntityType(row.EntityType), ID: row.EntityID},
		Patch:     json.RawMessage(body),
		IfVersion: &ifVersion,
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
// caller somebody else's line whenever two people press Undo in the same
// moment.
func (s RestoreSeam) readRestoreEntry(ctx context.Context, entityType string, id, auditID ids.UUID) (privacy.RecordHistoryEntry, error) {
	return privacy.ReadRestoreOf(ctx, InstallationDB(s.pool), entityType, id, auditID)
}

// RefusedRestore is a refusal the surface can render. It carries the reason
// WORD rather than prose, because the disabled button and the 409 must say the
// same thing — a person who reads one sentence before pressing and a different
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
