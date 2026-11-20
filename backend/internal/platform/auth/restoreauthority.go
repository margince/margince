// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EnsureRestorable admits bringing one archived row back under r. A delete
// holder may restore any archive. A seat with edit rights alone may restore
// only an archive it made itself, of a row the archive rule admits it to. So
// a rep cannot overturn an archive a manager made. The caller still asks the
// row write gate, and RequireArchiveGrant up front.
func EnsureRestorable(ctx context.Context, tx pgx.Tx, r ArchiveRule, id ids.UUID) error {
	if probe(ctx, r.object, principal.ActionDelete) == nil {
		return nil
	}
	deleteCheck := Check{Object: r.object, Actions: []principal.Action{principal.ActionDelete}}
	if err := EnsureArchivable(ctx, tx, r, id); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return refusedOn(ctx, deleteCheck, &RestoreAuthorityError{})
		}
		return err
	}
	self, err := archivedBySelf(ctx, tx, r.table, id)
	if err != nil {
		return err
	}
	if !self {
		return refusedOn(ctx, deleteCheck, &RestoreAuthorityError{})
	}
	return nil
}

// archivedBySelf answers whether the row's latest archive was made by the
// caller, or by an agent on the caller's behalf.
func archivedBySelf(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) (bool, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return false, err
	}
	if p.UserID.IsZero() {
		return false, nil
	}
	var self bool
	err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT coalesce((
		SELECT made.on_behalf_of IN (%[1]s)
		    OR (split_part(made.actor_id, ':', 1) = '%[2]s'
		        AND split_part(made.actor_id, ':', 2) IN (SELECT member.id::text FROM (%[1]s) member))
		  FROM audit_log made
		 WHERE made.entity_type = $1 AND made.entity_id = $2 AND made.action = 'archive'
		 ORDER BY made.occurred_at DESC, made.id DESC LIMIT 1), false)`,
		memberIdentitySQL(3), principal.PrincipalHuman), table, id, p.UserID).Scan(&self)
	if err != nil {
		return false, fmt.Errorf("auth: reading who archived %s %s: %w", table, id, err)
	}
	return self, nil
}

// RestoreAuthorityError names a restore the archive rule refuses, on every
// transport.
type RestoreAuthorityError struct{}

func (e *RestoreAuthorityError) Error() string {
	code, message := e.MessageFault()
	return code + ": " + message
}

func (e *RestoreAuthorityError) Unwrap() error { return apperrors.ErrPermissionDenied }

// MessageFault keeps restore refusals identical across HTTP and tool transports.
func (e *RestoreAuthorityError) MessageFault() (string, string) {
	return "restore_archiver_required", "Only the colleague who archived this record, or a manager, can restore it."
}
