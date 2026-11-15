// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// roleFromSignatureTitleTx sets the role of the contact's current primary
// employment to the title a signature just landed. A role a human set stays.
// The company's contact list shows the role and the contact page the title.
// Both name the same job only when the role follows the title.
func (s *Store) roleFromSignatureTitleTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, fields []SignatureField, applied []string,
) error {
	if !slices.Contains(applied, fieldTitle) {
		return nil
	}
	// A seat that may not edit relationships keeps the title it got; the role
	// is left alone, not refused.
	if err := auth.Require(ctx, "relationship", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	idx := slices.IndexFunc(fields, func(f SignatureField) bool { return f.Name == fieldTitle })
	if idx < 0 {
		return nil
	}
	title := fields[idx].Value
	var job ids.UUID
	err := tx.QueryRow(ctx, `SELECT r.id FROM relationship r
		 WHERE r.contact_id = $1 AND `+employment.LiveCurrentPrimarySQL("r")+`
		   AND r.role IS DISTINCT FROM $2
		   AND NOT EXISTS (SELECT 1 FROM audit_log decided WHERE decided.entity_type = 'relationship'
		     AND decided.entity_id = r.id AND decided.actor_type = 'human'
		     AND btrim(coalesce(decided.after->>'role', '')) <> '')
		 LIMIT 1`, contactID.UUID, title).Scan(&job)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("contacts: reading the employment a signature's title names: %w", err)
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	_, err = s.updateRelationshipInTx(ctx, tx, job, UpdateRelationshipInput{Role: &title}, capturedBy)
	return err
}
