// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// verdictEvidenceHolds asks, under the deal row lock, whether the evidence
// the verdict was decided on is still the deal's. The owner must still be a
// member and the checksum must not have moved. No offer may have been
// accepted since. Every cited activity must still be linked, live and
// readable by everyone.
func verdictEvidenceHolds(
	ctx context.Context, tx pgx.Tx, c verdictCandidate, read dealVerdictRead, changes []deals.VerdictChange,
) (bool, error) {
	if _, _, err := identity.EffectiveAuthorityTx(ctx, tx, c.owner); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	cited := citedActivities(changes)
	var checksum string
	var accepted, eligible int
	err := tx.QueryRow(ctx, `
		SELECT `+dealVerdictChecksumSQL("d")+`,
		       (SELECT count(*) FROM offer o WHERE o.deal_id = d.id AND o.archived_at IS NULL AND o.accepted_at IS NOT NULL),
		       (SELECT count(DISTINCT a.id) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		         WHERE l.deal_id = d.id AND a.id::text = ANY($2) AND a.archived_at IS NULL AND `+visibleToAllSQL("a")+`)
		  FROM deal d WHERE d.id = $1`, c.deal, cited).Scan(&checksum, &accepted, &eligible)
	if err != nil {
		return false, fmt.Errorf("compose: rechecking the verdict's evidence: %w", err)
	}
	return checksum == read.checksum && accepted == read.acceptedOffers && eligible == len(cited), nil
}

// citedActivities lists the activity ids the changes cite, once each.
func citedActivities(changes []deals.VerdictChange) []string {
	var out []string
	for _, change := range changes {
		ids, ok := change.Evidence["cited_activity_ids"].([]string)
		if !ok {
			continue
		}
		for _, id := range ids {
			if !slices.Contains(out, id) {
				out = append(out, id)
			}
		}
	}
	return out
}
