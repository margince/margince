// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// requestKinds are the acquisition kinds that record the contact asking to hear
// from us. The send check reads them as a request with no age limit, so they
// are the ones a request retraction reaches.
var requestKinds = []string{AcquiredRequestedQuoteOrMeeting, AcquiredInPersonPermission}

// RetractAcquisitionRequest takes back the request this contact's acquisition
// records. The rows stay as the record of how the contact was obtained; no send
// rests on them any more. Human-only, as recording a request by hand is.
func (s *Store) RetractAcquisitionRequest(ctx context.Context, contactID ids.ContactID) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		var kinds []string
		err := tx.QueryRow(ctx, `
			WITH retracted AS (
				UPDATE contact_acquisition_evidence
				   SET request_retracted_at = now(), request_retracted_by = $2
				 WHERE contact_id = $1 AND kind = ANY($3::text[]) AND request_retracted_at IS NULL
				RETURNING kind)
			SELECT coalesce(array_agg(DISTINCT kind ORDER BY kind), '{}') FROM retracted`,
			contactID, by, requestKinds).Scan(&kinds)
		if err != nil {
			return fmt.Errorf("contacts: retracting the acquisition request: %w", err)
		}
		if len(kinds) == 0 {
			return fmt.Errorf("contacts: this contact holds no live acquisition request: %w", apperrors.ErrNotFound)
		}
		// Audit-only: the public-event catalog has no type for a lawful basis,
		// and the trail answers who withdrew which request.
		if _, err := storekit.AuditEvent(ctx, tx, "update", "contact", contactID.UUID,
			map[string]any{"acquisition_request_retracted": kinds}); err != nil {
			return fmt.Errorf("contacts: auditing the retraction: %w", err)
		}
		return nil
	})
}
