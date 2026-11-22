// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RetractOwnAddressContactTx archives the contact capture made for an address
// its owner has since claimed. It answers false and changes nothing unless the
// owner alone holds the record and nobody has worked on it.
//
// The contact must be machine-made for this owner, owner-scoped, with no live
// grant and no audit row from a user, their agent or an approval. Capture's
// own writes name the owner in on_behalf_of, so a connector row is no evidence.
// The owner's right to archive it is asked first, under the row lock.
func (s *Store) RetractOwnAddressContactTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, owner ids.UUID) (bool, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return false, leftWhenRefused(err)
	}
	if err := lockThenEnsureArchivable(ctx, tx, contactEntity, auth.ContactArchive, id.UUID); err != nil {
		return false, leftWhenRefused(err)
	}
	var eligible bool
	err := tx.QueryRow(ctx, `
		SELECT coalesce(c.owner_id = $2 AND c.visibility = $3
		   AND (c.captured_by LIKE 'connector:%' OR c.captured_by LIKE 'agent:%')
		   AND NOT EXISTS (
		         SELECT 1 FROM record_grant rg
		          WHERE rg.record_type = $4 AND rg.record_id = c.id
		            AND (rg.expires_at IS NULL OR rg.expires_at > now()))
		   AND NOT EXISTS (
		         SELECT 1 FROM audit_log a
		          WHERE a.entity_type = $4 AND a.entity_id = c.id
		            AND (`+storekit.AuditForAUser("a")+`
		                 OR (a.on_behalf_of IS NOT NULL AND a.actor_type <> 'connector'))), false)
		  FROM contact c WHERE c.id = $1 AND c.archived_at IS NULL
		   FOR UPDATE OF c`, id.UUID, owner, visibilityOwner, entityContact).Scan(&eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading whether a claimed address's contact may be archived: %w", err)
	}
	if !eligible {
		return false, nil
	}
	if err := archiveContactRows(ctx, tx, id, time.Now().UTC(), nil); err != nil {
		return false, fmt.Errorf("contacts: archiving the contact of a claimed address: %w", err)
	}
	return true, nil
}

// leftWhenRefused turns the refusal of the member's own right to the record
// into "leave it": a claim never fails on the member's grants. Any other error
// is a fault and stays one.
func leftWhenRefused(err error) error {
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
}
