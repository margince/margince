// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const overrideCarryCandidates = `SELECT DISTINCT ON (live.category)
 live.id, live.category, live.reason, live.decided_by_level
 FROM communication_override live
 WHERE ((@from_contact::uuid IS NOT NULL AND live.contact_id=@from_contact)
 OR (@from_lead::uuid IS NOT NULL AND live.lead_id=@from_lead))
 AND live.revoked_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM communication_override held
 WHERE ((@to_contact::uuid IS NOT NULL AND held.contact_id=@to_contact)
 OR (@to_lead::uuid IS NOT NULL AND held.lead_id=@to_lead))
 AND held.category=live.category AND held.revoked_at IS NULL
 AND coalesce(array_position(@ladder::text[],held.decided_by_level),array_length(@ladder::text[],1)+1)
 >= coalesce(array_position(@ladder::text[],live.decided_by_level),array_length(@ladder::text[],1)+1))
 ORDER BY live.category,
 coalesce(array_position(@ladder::text[],live.decided_by_level),array_length(@ladder::text[],1)+1) DESC,
 live.recorded_at DESC, live.id`

// PreviewOverridesTx shares carry candidates with the writer so review preserves authority precedence.
func (s *Store) PreviewOverridesTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) ([]commsauthz.CarriedOverride, error) {
	if err := admitCarryPreview(ctx, tx, from, to); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id,category,reason,decided_by_level FROM ("+overrideCarryCandidates+") candidates ORDER BY category, id LIMIT 501", carryAuthorityArgs(from, to, ""))
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (commsauthz.CarriedOverride, error) {
		var out commsauthz.CarriedOverride
		err := row.Scan(&out.ID, &out.Category, &out.Reason, &out.Authority)
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
