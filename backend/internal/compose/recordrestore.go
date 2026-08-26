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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/privacy"
	"github.com/gradionhq/margince/backend/internal/platform/database"
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

// readRow loads the target entry, bound to the path's own record. An audit row
// belonging to a DIFFERENT record is ErrNotFound and never a 403: telling a
// caller that a row exists but is not theirs discloses the row, which is the
// existence-hiding rule every single-record read here keeps.
func (s RestoreSeam) readRow(ctx context.Context, entityType string, id, auditID ids.UUID) (AuditRow, error) {
	var row AuditRow
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
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
		return AuditRow{}, fmt.Errorf("compose: read the change to put back: %w", err)
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
	patch, err := filterImage(row.EntityType, row.Before)
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
// word rather than prose, because the disabled button and the 409 must say the
// SAME thing — a person who reads one sentence before pressing and a different
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

// Unwrap makes a refusal a conflict to every layer that classifies by sentinel,
// so the route answers 409 without the transport learning this type.
func (e RefusedRestore) Unwrap() error { return apperrors.ErrConflict }
