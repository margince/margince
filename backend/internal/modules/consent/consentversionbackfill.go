// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ConsentVersionPage advances over every inspected state, including unknown proof.
type ConsentVersionPage struct {
	Scanned []ids.UUID
	Changed int
}

// BackfillConsentVersionsTx copies only the version of the latest matching proof.
// Locking the page before reading proofs keeps a concurrent withdrawal from
// receiving the earlier grant's version. No consent decision or proof is written.
func BackfillConsentVersionsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (ConsentVersionPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ConsentVersionPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT id FROM contact_consent WHERE id > $1 ORDER BY id LIMIT $2 FOR UPDATE`, after, limit)
	if err != nil {
		return ConsentVersionPage{}, fmt.Errorf("consent: paging wording versions: %w", err)
	}
	var page ConsentVersionPage
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ConsentVersionPage{}, err
		}
		page.Scanned = append(page.Scanned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ConsentVersionPage{}, err
	}
	if len(page.Scanned) == 0 {
		return page, nil
	}
	result, err := tx.Exec(ctx, `
 WITH matches AS (
  SELECT c.id, e.policy_version
    FROM contact_consent c
    CROSS JOIN LATERAL (
      SELECT proof.* FROM consent_event proof
       WHERE proof.purpose_id = c.purpose_id
         AND (proof.contact_id = c.contact_id OR proof.lead_id = c.lead_id)
       ORDER BY proof.captured_at DESC, proof.id DESC LIMIT 1
    ) e
   WHERE c.id = ANY($1) AND c.policy_version IS NULL
     AND e.new_state = c.state AND e.captured_at = c.captured_at
     AND e.source IS NOT DISTINCT FROM c.source
     AND e.lawful_basis IS NOT DISTINCT FROM c.lawful_basis
     AND NULLIF(btrim(e.policy_text), '') IS NOT NULL AND e.policy_version IS NOT NULL
 )
 UPDATE contact_consent c SET policy_version = matches.policy_version
   FROM matches WHERE c.id = matches.id AND c.policy_version IS NULL`, page.Scanned)
	if err != nil {
		return ConsentVersionPage{}, fmt.Errorf("consent: copying proven wording versions: %w", err)
	}
	page.Changed = int(result.RowsAffected())
	return page, nil
}

// EstimateConsentVersionBackfill counts the input, including states left unknown.
func EstimateConsentVersionBackfill(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM contact_consent LIMIT $1) states`, maximum+1).Scan(&count)
	return count, err
}
