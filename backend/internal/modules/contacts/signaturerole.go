// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s *Store) roleFromSignatureTitleTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID, source string) error {
	if err := auth.Require(ctx, tableRelationship, principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contact.String()); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `SELECT r.id FROM relationship r WHERE r.contact_id=@contact AND r.kind='employment'
 AND `+employment.LiveCurrentPrimarySQL("r")+`
 AND (coalesce(r.role,'')='' OR NOT coalesce((
 SELECT edited.actor_type IN ('human','agent') AND edited.evidence->>'source' IS DISTINCT FROM @source
 AND NOT (coalesce(edited.evidence,'{}'::jsonb) ? 'provider')
 FROM audit_log edited WHERE edited.entity_type='relationship' AND edited.entity_id=r.id
 AND edited.after ? 'role' AND (edited.action='create' OR edited.before->'role' IS DISTINCT FROM edited.after->'role')
 ORDER BY edited.occurred_at DESC, edited.id DESC LIMIT 1),r.source='manual'))`, pgx.NamedArgs{contactKey: contact, auditKeySource: enrichSource})
	if err != nil {
		return err
	}
	idsToFill, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	var title *string
	if err := tx.QueryRow(ctx, `SELECT title FROM contact WHERE id=@contact`, pgx.NamedArgs{contactKey: contact}).Scan(&title); err != nil {
		return err
	}
	if title == nil || *title == "" {
		return nil
	}
	for _, id := range idsToFill {
		_, err := s.updateRelationshipInTx(ctx, tx, id, UpdateRelationshipInput{Role: title, Evidence: map[string]any{auditKeySource: enrichSource, auditKeySourceRef: source}}, enrichCapturedBy)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}
