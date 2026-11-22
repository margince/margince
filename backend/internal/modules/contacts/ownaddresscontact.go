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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The evidence an own-address archive records, so the claim's removal can find
// the contact again.
const (
	ownAddressReasonKey = "reason"
	ownAddressReason    = "own_address_claimed"
	ownAddressClaimKey  = "claim"
)

// RetractOwnAddressContactTx archives the contact capture made for an address
// its owner has since proven theirs. It answers false and changes nothing
// unless the owner alone holds the record and nobody has worked on it.
//
// The contact must be machine-made for this owner and owner-scoped. It must
// have no live grant and no audit row from a user, agent or approval. It must
// have no relationship a person or a deal made. The owner's right to archive it is asked first, under
// the row lock. The archive records claim, so the claim's removal finds it.
func (s *Store) RetractOwnAddressContactTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, owner, claim ids.UUID) (bool, error) {
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
		   AND `+contactNotWorkedOnSQL("c")+`
		   AND NOT EXISTS (
		         SELECT 1 FROM relationship r
		          WHERE (r.contact_id = c.id OR r.counterparty_contact_id = c.id) AND r.archived_at IS NULL
		            AND (r.kind <> 'employment' OR NOT (r.captured_by LIKE 'connector:%'
		                 OR r.captured_by LIKE 'agent:%' OR r.captured_by LIKE 'system:%'))), false)
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
	extra := map[string]any{ownAddressReasonKey: ownAddressReason, ownAddressClaimKey: claim}
	if err := archiveContactRows(ctx, tx, id, time.Now().UTC(), nil, extra); err != nil {
		return false, fmt.Errorf("contacts: archiving the contact of a claimed address: %w", err)
	}
	return true, nil
}

// ContactArchivedByClaimTx answers the contact a claim's archive retired, while
// it is still archived. The claim's removal asks it to put the contact back.
func (s *Store) ContactArchivedByClaimTx(ctx context.Context, tx pgx.Tx, claim ids.UUID) (ids.ContactID, bool, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return ids.ContactID{}, false, leftWhenRefused(err)
	}
	var id ids.ContactID
	err := tx.QueryRow(ctx, `
		SELECT c.id FROM audit_log a JOIN contact c ON c.id = a.entity_id AND c.archived_at IS NOT NULL
		 WHERE a.entity_type = $1 AND a.action = 'archive'
		   AND a.evidence ->> $2::text = $3 AND a.evidence ->> $4::text = $5
		 ORDER BY a.id DESC LIMIT 1`,
		contactEntity, ownAddressReasonKey, ownAddressReason, ownAddressClaimKey, claim.String()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ContactID{}, false, nil
	}
	if err != nil {
		return ids.ContactID{}, false, fmt.Errorf("contacts: reading the contact a claim archived: %w", err)
	}
	return id, true, nil
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
