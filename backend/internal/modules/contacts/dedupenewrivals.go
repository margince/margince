// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// newRivals keeps the best dedupeRivalLimit rivals the queue holds no pair
// with yet. A re-check names its subject, and a rival already paired with it,
// open or decided, would only be a no-op filing. Taking the cap after that
// keeps five dismissed look-alikes from hiding a real duplicate ranked sixth.
// A create names no subject, so nothing is paired with it yet.
func newRivals[T any](ctx context.Context, tx pgx.Tx, entityType string, subject *ids.UUID,
	ranked []T, rivalOf func(T) ids.UUID,
) ([]T, error) {
	if subject != nil && len(ranked) > 0 {
		paired, err := pairedWith(ctx, tx, entityType, *subject, ranked, rivalOf)
		if err != nil {
			return nil, err
		}
		kept := ranked[:0:0]
		for _, rival := range ranked {
			if !paired[rivalOf(rival)] {
				kept = append(kept, rival)
			}
		}
		ranked = kept
	}
	return ranked[:min(len(ranked), dedupeRivalLimit)], nil
}

// heldPairSQL is the condition that dedupe_candidate row d is a pair the queue
// holds between subject and rival, open or decided. A pair the system archived
// undecided is not held: filing it again reopens it (refileArchivedCandidateTx).
// The coalesce expressions are uq_dedupe_candidate_pair's, so the index serves
// the lookup. rival may be `ANY($n)`.
func heldPairSQL(entityType, subject, rival string) string {
	return storekit.SQLf(`d.entity_type = %[1]s AND (d.archived_at IS NULL OR d.disposition <> 'open')
		AND ((coalesce(d.left_contact_id, d.left_company_id, d.left_lead_id) = %[2]s
		      AND coalesce(d.right_contact_id, d.right_company_id, d.right_lead_id) = %[3]s)
		  OR (coalesce(d.right_contact_id, d.right_company_id, d.right_lead_id) = %[2]s
		      AND coalesce(d.left_contact_id, d.left_company_id, d.left_lead_id) = %[3]s))`,
		entityType, subject, rival)
}

// pairedWith answers the rivals the queue holds a pair with the subject for.
func pairedWith[T any](ctx context.Context, tx pgx.Tx, entityType string, subject ids.UUID,
	ranked []T, rivalOf func(T) ids.UUID,
) (map[ids.UUID]bool, error) {
	rivals := make([]ids.UUID, 0, len(ranked))
	for _, rival := range ranked {
		rivals = append(rivals, rivalOf(rival))
	}
	rows, err := tx.Query(ctx, `
		SELECT CASE WHEN l = $2 THEN r ELSE l END FROM (
		  SELECT coalesce(d.left_contact_id, d.left_company_id, d.left_lead_id) AS l,
		         coalesce(d.right_contact_id, d.right_company_id, d.right_lead_id) AS r
		    FROM dedupe_candidate d
		   WHERE `+heldPairSQL("$1", "$2", "ANY($3)")+`) pairs`,
		entityType, subject, rivals)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the pairs the queue holds: %w", err)
	}
	held, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the pairs the queue holds: %w", err)
	}
	paired := make(map[ids.UUID]bool, len(held))
	for _, id := range held {
		paired[id] = true
	}
	return paired, nil
}

// keepNewRivals is newRivals over a company ladder's ranked rivals. A match
// left with none is no match.
func keepNewRivals(ctx context.Context, tx pgx.Tx, c CompanyCandidate, match CompanyMatch) (CompanyMatch, error) {
	var subject *ids.UUID
	if c.ExcludeID != nil {
		subject = &c.ExcludeID.UUID
	}
	ranked, err := newRivals(ctx, tx, entityCompany, subject, match.Ranked,
		func(r CompanyCandidateScore) ids.UUID { return r.CompanyID.UUID })
	if err != nil {
		return CompanyMatch{}, err
	}
	if len(ranked) == 0 {
		return CompanyMatch{Decision: DecisionNoMatch}, nil
	}
	match.Ranked, match.CompanyID, match.Confidence = ranked, ranked[0].CompanyID, ranked[0].Confidence
	return match, nil
}
