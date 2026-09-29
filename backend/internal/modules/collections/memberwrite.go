// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// The one writer of Shortlist membership. The library, a record page, a bulk
// change and an agent all add and remove members here, so every change is
// attributed, noted and recorded in list_member_event the same way.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Why a membership changed (list_member_event_reason_check). The archive and
// restore reasons are written by the record modules' own cascades.
const (
	ReasonChosen = "chosen"
	ReasonBulk   = "bulk"
)

// The two membership actions (list_member_event_action_check).
const (
	memberAdded   = "added"
	memberRemoved = "removed"
)

// MemberChange names one record to add to or remove from a Shortlist, with
// the note its author left and why the change was made.
type MemberChange struct {
	EntityType string
	EntityID   ids.UUID
	Note       *string
	Reason     string
}

// ErrAlreadyMember and ErrNotMember are a change that would change nothing.
var (
	ErrAlreadyMember = fmt.Errorf("the record is already on this list: %w", apperrors.ErrConflict)
	ErrNotMember     = fmt.Errorf("the record is not on this list: %w", apperrors.ErrNotFound)
)

// AddMember adds one record to a Shortlist in its own transaction.
func (s *Store) AddMember(ctx context.Context, listID ids.ListID, change MemberChange) (memberRow, error) {
	if err := httperr.RequireBodyID(entityIDField, change.EntityID); err != nil {
		return memberRow{}, err
	}
	var out memberRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.AddMemberTx(ctx, tx, listID, change)
		return err
	})
	return out, err
}

// RemoveMember removes one record from a Shortlist in its own transaction.
func (s *Store) RemoveMember(ctx context.Context, listID ids.ListID, change MemberChange) error {
	if err := httperr.RequireBodyID(entityIDField, change.EntityID); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return s.RemoveMemberTx(ctx, tx, listID, change)
	})
}

// AddMemberTx adds one record to a Shortlist on the caller's transaction.
func (s *Store) AddMemberTx(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) (memberRow, error) {
	actor, err := admitMemberChange(ctx, tx, listID, change)
	if err != nil {
		return memberRow{}, err
	}
	var out memberRow
	err = rowScanMember(tx.QueryRow(ctx, `
		INSERT INTO list_member (list_id, entity_type, entity_id, added_by, note)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (list_id, entity_type, entity_id) DO NOTHING
		RETURNING id, list_id, entity_type, entity_id, added_by, created_at, note`,
		listID, change.EntityType, change.EntityID, actor, change.Note), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return memberRow{}, ErrAlreadyMember
	}
	if err != nil {
		return memberRow{}, err
	}
	return out, recordMemberChange(ctx, tx, listID, change, memberAdded, actor)
}

// RemoveMemberTx removes one record from a Shortlist on the caller's
// transaction.
func (s *Store) RemoveMemberTx(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) error {
	actor, err := admitMemberChange(ctx, tx, listID, change)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`DELETE FROM list_member WHERE list_id = $1 AND entity_type = $2 AND entity_id = $3`,
		listID, change.EntityType, change.EntityID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotMember
	}
	return recordMemberChange(ctx, tx, listID, change, memberRemoved, actor)
}

// admitMemberChange asks every gate a membership change passes: the list
// update grant, list authority over a live Shortlist of the record's type,
// and read access to the record itself. A record the caller cannot see is
// refused as absent, so a list cannot become a way to learn one exists.
func admitMemberChange(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) (string, error) {
	if err := httperr.RequireBodyID(entityIDField, change.EntityID); err != nil {
		return "", err
	}
	if err := auth.Require(ctx, listObject, principal.ActionUpdate); err != nil {
		return "", err
	}
	if change.Reason != ReasonChosen && change.Reason != ReasonBulk {
		return "", fmt.Errorf("membership change reason %q is not one this writer records", change.Reason)
	}
	list, _, err := editableList(ctx, tx, listID)
	if err != nil {
		return "", err
	}
	if list.ArchivedAt != nil {
		return "", ErrListArchived
	}
	if list.ListType != listTypeStatic {
		return "", &BadInputError{Field: "list", Reason: "a Live List's members follow its filter; only a Shortlist takes members by hand"}
	}
	if change.EntityType != list.EntityType {
		return "", &BadInputError{Field: entityTypeField, Reason: "must match the list's entity_type " + list.EntityType}
	}
	if err := auth.EnsureLinkTarget(ctx, tx, change.EntityType, change.EntityID); err != nil {
		return "", err
	}
	return storekit.CapturedBy(ctx)
}

// recordMemberChange writes the membership event row, the audit row and the
// outbox event of one change.
func recordMemberChange(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange, action, actor string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		listID, change.EntityType, change.EntityID, action, change.Reason, actor, change.Note); err != nil {
		return fmt.Errorf("record the membership change: %w", err)
	}
	member := map[string]any{entityTypeField: change.EntityType, entityIDField: change.EntityID}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", listObject, listID.UUID, map[string]any{action: member})
	if err != nil {
		return err
	}
	// The record is the event's subject, so the fan-out delivers it only to a
	// subscriber who may see that record.
	list := openapi_types.UUID(listID.UUID)
	if action == memberAdded {
		return storekit.EmitEventForEntity(ctx, tx, auditID, change.EntityType, change.EntityID,
			crmcontracts.PublicEventListMemberAdded{ListId: list, Reason: change.Reason})
	}
	return storekit.EmitEventForEntity(ctx, tx, auditID, change.EntityType, change.EntityID,
		crmcontracts.PublicEventListMemberRemoved{ListId: list, Reason: change.Reason})
}
