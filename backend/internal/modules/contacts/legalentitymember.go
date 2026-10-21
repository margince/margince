// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which legal entity a user works for. A user with no row works for the
// default entity, so moving somebody to the default removes their row. Two
// spellings of "the default" would be two answers a reader has to reconcile.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// userAdminObject is the grant that administers seats. Choosing the entity a
// user works for is administering them, so an assignment asks it too.
const userAdminObject = "user_admin"

// An assignment's audit images name the user and the entity they work for.
const (
	auditKeyUser   = "user_id"
	auditKeyEntity = "legal_entity_id"
)

// LegalEntityMember is one assignment row.
type LegalEntityMember struct {
	UserID        ids.UserID
	LegalEntityID ids.LegalEntityID
	AssignedAt    time.Time
}

// ListLegalEntityMembers lists the users an entity's rows name, oldest first.
// The default entity's list is empty: its members are the users with no row.
func (s *Store) ListLegalEntityMembers(ctx context.Context, id ids.LegalEntityID) ([]LegalEntityMember, error) {
	if err := auth.Require(ctx, legalEntityObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []LegalEntityMember
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := readLegalEntity(ctx, tx, id); err != nil {
			return err
		}
		var args legalEntityArgs
		rows, err := tx.Query(ctx,
			`SELECT user_id, legal_entity_id, assigned_at FROM legal_entity_member
			  WHERE legal_entity_id = `+args.add(id)+` ORDER BY assigned_at, user_id`, args...)
		if err != nil {
			return fmt.Errorf("list legal entity members: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (LegalEntityMember, error) {
			var m LegalEntityMember
			return m, row.Scan(&m.UserID, &m.LegalEntityID, &m.AssignedAt)
		})
		return err
	})
	return out, err
}

// AssignLegalEntityMember makes id the entity user works for, moving them from
// the one they worked for before. Assigning them where they already are writes
// nothing.
func (s *Store) AssignLegalEntityMember(ctx context.Context, id ids.LegalEntityID, user ids.UserID) error {
	for _, object := range []string{legalEntityObject, userAdminObject} {
		if err := auth.Require(ctx, object, principal.ActionUpdate); err != nil {
			return err
		}
	}
	if err := httperr.RequireBodyID(auditKeyUser, user.UUID); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		// ArchiveLegalEntity takes this lock before it counts the members.
		if _, err := storekit.LockRow(ctx, tx, legalEntityTable, id.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		if err := requireUserExists(ctx, tx, user); err != nil {
			return err
		}
		previous, err := UserLegalEntity(ctx, tx, user)
		if err != nil || previous == id {
			return err
		}
		if err := writeMembership(ctx, tx, id, user, by); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, actionAssign, legalEntityObject, id.UUID,
			map[string]any{auditKeyUser: user, auditKeyEntity: previous},
			map[string]any{auditKeyUser: user, auditKeyEntity: id})
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.InternalEventLegalEntityMemberAdded{
			UserId: openapi_types.UUID(user.UUID), PreviousLegalEntityId: openapi_types.UUID(previous.UUID),
		}); err != nil {
			return fmt.Errorf("emit legal_entity.member_added: %w", err)
		}
		return nil
	})
}

// writeMembership stores the assignment, or removes it when the target is the
// default entity.
func writeMembership(ctx context.Context, tx pgx.Tx, id ids.LegalEntityID, user ids.UserID, by string) error {
	var isDefault bool
	var read legalEntityArgs
	if err := tx.QueryRow(ctx, `SELECT is_default FROM legal_entity WHERE id = `+read.add(id), read...).Scan(&isDefault); err != nil {
		return fmt.Errorf("read the legal entity being assigned: %w", err)
	}
	if isDefault {
		var args legalEntityArgs
		if _, err := tx.Exec(ctx, `DELETE FROM legal_entity_member WHERE user_id = `+args.add(user), args...); err != nil {
			return fmt.Errorf("move a user to the default legal entity: %w", err)
		}
		return nil
	}
	values := []any{user, id, by}
	if _, err := tx.Exec(ctx,
		`INSERT INTO legal_entity_member (user_id, legal_entity_id, assigned_by) VALUES (`+storekit.Placeholders(values)+`)
		 ON CONFLICT (user_id) DO UPDATE
		    SET legal_entity_id = EXCLUDED.legal_entity_id, assigned_at = now(), assigned_by = EXCLUDED.assigned_by`,
		values...); err != nil {
		return fmt.Errorf("assign a user to a legal entity: %w", err)
	}
	return nil
}

func requireUserExists(ctx context.Context, tx pgx.Tx, user ids.UserID) error {
	var exists bool
	var args legalEntityArgs
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user WHERE id = `+args.add(user)+`)`, args...).Scan(&exists); err != nil {
		return fmt.Errorf("check the user being assigned: %w", err)
	}
	if !exists {
		return &values.ParseError{Field: auditKeyUser, Code: "unknown_user", Message: "user_id names no user"}
	}
	return nil
}
