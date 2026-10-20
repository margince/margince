// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// The writer runs after email ownership has moved. The preview supplies the
// merge's planned email IDs; only live rows still owned by its source qualify.
const preferenceCarryRetires = `p.revoked_at IS NULL AND
 (EXISTS (SELECT 1 FROM preference_token h WHERE h.contact_id=@target AND h.revoked_at IS NULL
 AND h.contact_email_id IS NOT DISTINCT FROM p.contact_email_id)
 OR EXISTS (SELECT 1 FROM contact_email e WHERE e.id=p.contact_email_id
 AND (e.archived_at IS NOT NULL OR (e.contact_id<>@target
 AND NOT (e.contact_id=@source AND e.id=ANY(@moving_emails::uuid[]))))))`

func (s *Store) PreviewPreferenceLinksTx(ctx context.Context, tx pgx.Tx, from, to ids.ContactID, movingEmails []ids.UUID) ([]commsauthz.CarriedPreferenceLink, error) {
	if err := admitCarryPreview(ctx, tx, commsauthz.ContactStopSubject(from), commsauthz.ContactStopSubject(to)); err != nil {
		return nil, err
	}
	if err := requireCarryEmailRead(ctx, commsauthz.ContactStopSubject(from)); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT p.id,p.contact_email_id,p.created_at,p.expires_at,p.revoked_at,p.revoked_reason,
 `+preferenceCarryRetires+` FROM preference_token p WHERE p.contact_id=@source ORDER BY p.id LIMIT 501`,
		pgx.NamedArgs{"source": from, "target": to, "moving_emails": movingEmails})
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedPreferenceLink, error) {
		var out commsauthz.CarriedPreferenceLink
		err := row.Scan(&out.ID, &out.EmailID, &out.CreatedAt, &out.ExpiresAt, &out.RevokedAt, &out.RevokedReason, &out.WillRetire)
		return out, err
	})
	if err != nil {
		return nil, err
	}
	if len(out) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	return out, nil
}
