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
	"time"

	"github.com/jackc/pgx/v5"

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
	ReasonChosen     = "chosen"
	ReasonBulk       = "bulk"
	ReasonAutomation = "automation"
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
	// AddedAt puts a member back with the time it was first added; nil is now.
	AddedAt *time.Time
}

// RemovedMember is what a removal took off the list, so an undo can put the
// member back as it was.
type RemovedMember struct {
	Note    *string
	AddedAt time.Time
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
		_, err := s.RemoveMemberTx(ctx, tx, listID, change)
		return err
	})
}

// AddMemberOnBehalf adds one record to a Shortlist for an automation: admit
// carries the rule's owner, whose list authority and row scope decide, and ctx
// the engine that writes the change on their behalf. A record already on the
// list is no change and answers false.
func (s *Store) AddMemberOnBehalf(ctx, admit context.Context, listID ids.ListID, change MemberChange) (bool, error) {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := addMemberTx(ctx, admit, tx, listID, change)
		return err
	})
	if errors.Is(err, ErrAlreadyMember) {
		return false, nil
	}
	return err == nil, err
}

// AddMemberTx adds one record to a Shortlist on the caller's transaction.
func (s *Store) AddMemberTx(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) (memberRow, error) {
	return addMemberTx(ctx, ctx, tx, listID, change)
}

// addMemberTx admits the change as the principal on admit and writes it as
// the one on ctx; for a signed-in user or an agent acting for one, the two
// are one.
func addMemberTx(ctx, admit context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) (memberRow, error) {
	if err := admitMemberChange(admit, tx, listID, change); err != nil {
		return memberRow{}, err
	}
	actor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return memberRow{}, err
	}
	var out memberRow
	err = rowScanMember(tx.QueryRow(ctx, `
		INSERT INTO list_member (list_id, entity_type, entity_id, added_by, note, created_at)
		VALUES (@list_id, @entity_type, @entity_id, @added_by, @note, COALESCE(@added_at, now()))
		ON CONFLICT (list_id, entity_type, entity_id) DO NOTHING
		RETURNING id, list_id, entity_type, entity_id, added_by, created_at, note`,
		pgx.StrictNamedArgs{
			listIDField: listID, entityTypeField: change.EntityType, entityIDField: change.EntityID,
			"added_by": actor, "note": change.Note,
			"added_at": change.AddedAt,
		}), &out)
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
func (s *Store) RemoveMemberTx(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) (RemovedMember, error) {
	if err := admitMemberChange(ctx, tx, listID, change); err != nil {
		return RemovedMember{}, err
	}
	actor, err := storekit.CapturedBy(ctx)
	if err != nil {
		return RemovedMember{}, err
	}
	var removed RemovedMember
	err = tx.QueryRow(ctx, `
		DELETE FROM list_member WHERE list_id = @list_id AND entity_type = @entity_type AND entity_id = @entity_id
		RETURNING note, created_at`,
		pgx.StrictNamedArgs{listIDField: listID, entityTypeField: change.EntityType, entityIDField: change.EntityID},
	).Scan(&removed.Note, &removed.AddedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return RemovedMember{}, ErrNotMember
	}
	if err != nil {
		return RemovedMember{}, err
	}
	return removed, recordMemberChange(ctx, tx, listID, change, memberRemoved, actor)
}

// admitMemberChange asks every gate a membership change passes: the list
// update grant, list authority over a live Shortlist of the record's type,
// and read access to the record itself. A record the caller cannot see is
// refused as absent, so a list cannot become a way to learn one exists.
func admitMemberChange(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange) error {
	if err := httperr.RequireBodyID(entityIDField, change.EntityID); err != nil {
		return err
	}
	switch change.Reason {
	case ReasonChosen, ReasonBulk, ReasonAutomation:
	default:
		return fmt.Errorf("membership change reason %q is not one this writer records", change.Reason)
	}
	if err := admitShortlistChange(ctx, tx, listID, change.EntityType); err != nil {
		return err
	}
	// Naming a record reads it: a caller refused the record type is answered
	// as for a record they cannot see, so the refusal says nothing about it.
	if auth.Require(ctx, change.EntityType, principal.ActionRead) != nil {
		return apperrors.ErrNotFound
	}
	return auth.EnsureLinkTarget(ctx, tx, change.EntityType, change.EntityID)
}

// CheckShortlistChange asks, before any record is named, whether the caller
// may change the membership of this list for records of entityType: a bulk
// change refuses the whole selection here rather than every row in turn.
func (s *Store) CheckShortlistChange(ctx context.Context, listID ids.ListID, entityType string) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		return admitShortlistChange(ctx, tx, listID, entityType)
	})
}

// admitShortlistChange is the list half of a membership change's gates: the
// list update grant, list authority, a live Shortlist, and its record type.
func admitShortlistChange(ctx context.Context, tx pgx.Tx, listID ids.ListID, entityType string) error {
	if err := auth.Require(ctx, listObject, principal.ActionUpdate); err != nil {
		return err
	}
	list, _, err := editableList(ctx, tx, listID)
	if err != nil {
		return err
	}
	if list.ArchivedAt != nil {
		return ErrListArchived
	}
	if list.ListType != listTypeStatic {
		return &BadInputError{Field: "list", Reason: "a Live List's members follow its filter; only a Shortlist takes members by hand"}
	}
	if entityType != list.EntityType {
		return &BadInputError{Field: entityTypeField, Reason: "must match the list's entity_type " + list.EntityType}
	}
	return nil
}

// recordMemberChange writes the membership event row, the audit row and the
// outbox event of one change.
func recordMemberChange(ctx context.Context, tx pgx.Tx, listID ids.ListID, change MemberChange, action, actor string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO list_member_event (list_id, entity_type, entity_id, action, reason, actor, note)
		VALUES (@list_id, @entity_type, @entity_id, @action, @reason, @actor, @note)`,
		pgx.StrictNamedArgs{
			listIDField: listID, entityTypeField: change.EntityType, entityIDField: change.EntityID,
			"action": action, "reason": change.Reason, "actor": actor, "note": change.Note,
		}); err != nil {
		return fmt.Errorf("record the membership change: %w", err)
	}
	member := map[string]any{entityTypeField: change.EntityType, entityIDField: change.EntityID}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", listObject, listID.UUID, map[string]any{action: member})
	if err != nil {
		return err
	}
	// The record is the event's subject, so the fan-out delivers it only to a
	// subscriber who may see that record; that says nothing about the list, so
	// the event names none. The list is on the audit row it links to.
	if action == memberAdded {
		return storekit.EmitEventForEntity(ctx, tx, auditID, change.EntityType, change.EntityID,
			crmcontracts.PublicEventListMemberAdded{Reason: change.Reason})
	}
	return storekit.EmitEventForEntity(ctx, tx, auditID, change.EntityType, change.EntityID,
		crmcontracts.PublicEventListMemberRemoved{Reason: change.Reason})
}
