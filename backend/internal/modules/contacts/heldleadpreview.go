// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The same candidate scope serves an existing eligible contact and the emails
// of a contact being proposed. Actual promotion still checks each lead's guards.
func heldLeadIDs(ctx context.Context, tx pgx.Tx, contact ids.ContactID, emails []string) ([]ids.UUID, error) {
	if contact.IsZero() {
		if len(emails) == 0 {
			return []ids.UUID{}, nil
		}
		if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
			return nil, err
		}
	} else {
		// Existing-contact candidates remain internal to the promotion writer.
		allowed, err := mayPromoteHeldLeads(ctx)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, apperrors.ErrPermissionDenied
		}
	}
	addresses := `SELECT unnest(@emails::text[]) AS email`
	args := pgx.NamedArgs{"emails": emails}
	if !contact.IsZero() {
		addresses = `SELECT ce.email FROM contact c
   JOIN contact_email ce ON ce.contact_id = c.id AND ce.archived_at IS NULL
   WHERE c.id = @contact AND c.archived_at IS NULL AND c.merged_into_id IS NULL
   AND c.visibility = 'workspace' AND c.quarantined_at IS NULL`
		args = pgx.NamedArgs{"contact": contact}
	}
	rows, err := tx.Query(ctx, `WITH addresses AS (`+addresses+`)
  SELECT l.id FROM lead l WHERE l.email IN (SELECT email FROM addresses)
  AND l.archived_at IS NULL AND l.merged_into_id IS NULL AND l.status <> 'promoted'
  ORDER BY l.created_at, l.id`, args)
	if err != nil {
		return nil, fmt.Errorf("read leads holding contact addresses: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

func previewContactHeldLeads(ctx context.Context, tx pgx.Tx, emails []ContactEmailInput) ([]ids.LeadID, error) {
	addresses := make([]string, 0, len(emails))
	for _, email := range emails {
		addresses = append(addresses, email.Email)
	}
	matches, err := heldLeadIDs(ctx, tx, ids.ContactID{}, addresses)
	if err != nil {
		return nil, err
	}
	if len(matches) == 0 {
		return []ids.LeadID{}, nil
	}
	out := make([]ids.LeadID, 0, len(matches))
	for _, id := range matches {
		if err := auth.EnsureVisible(ctx, tx, "lead", id); err != nil {
			return nil, err
		}
		out = append(out, ids.From[ids.LeadKind](id))
	}
	return out, nil
}
