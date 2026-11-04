// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// liveAcquisitionRequestSQL selects the request rows a send may still rest on,
// for the contact in @contact. The read and the retraction share it.
const liveAcquisitionRequestSQL = `contact_id = @contact AND kind = ANY(@kinds::text[]) AND request_retracted_at IS NULL`

// liveAcquisitionRequest reads the request the contact's acquisition still
// records, or nil when none stands.
func liveAcquisitionRequest(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (*crmcontracts.ContactAcquisitionRequest, error) {
	var kinds []string
	var occurredAt *time.Time
	err := tx.QueryRow(ctx, `
		SELECT coalesce(array_agg(DISTINCT kind ORDER BY kind), '{}'), min(occurred_at)
		  FROM contact_acquisition_evidence WHERE `+liveAcquisitionRequestSQL,
		pgx.NamedArgs{"contact": contactID, "kinds": requestKinds}).Scan(&kinds, &occurredAt)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the acquisition request: %w", err)
	}
	if len(kinds) == 0 {
		return nil, nil
	}
	wire := make([]crmcontracts.ContactAcquisitionRequestKinds, len(kinds))
	for i, kind := range kinds {
		wire[i] = crmcontracts.ContactAcquisitionRequestKinds(kind)
	}
	return &crmcontracts.ContactAcquisitionRequest{Kinds: wire, OccurredAt: occurredAt}, nil
}

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
				   SET request_retracted_at = now(), request_retracted_by = @by
				 WHERE `+liveAcquisitionRequestSQL+`
				RETURNING kind)
			SELECT coalesce(array_agg(DISTINCT kind ORDER BY kind), '{}') FROM retracted`,
			pgx.NamedArgs{"contact": contactID, "by": by, "kinds": requestKinds}).Scan(&kinds)
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
