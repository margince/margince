// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// FooterGap is what one live legal entity's mail owes and nobody stated.
type FooterGap struct {
	Entity    ids.UUID
	IsDefault bool
	// Missing lists the owed kinds with no text, in the order the send names
	// them. Empty when the entity's footer is complete.
	Missing []string
	// HeldByPerson and HeldInBackground count the sends held for
	// footer_incomplete since the details last changed.
	HeldByPerson, HeldInBackground int
}

// FooterGapsTx answers each live legal entity. A missing kind is an
// obligation DisclosuresFor reports missing for a first message from that
// entity, or a kind a held send recorded since its details last changed. The
// send asks DisclosuresFor too, so the two answers cannot differ.
func (s *Store) FooterGapsTx(ctx context.Context, tx pgx.Tx) ([]FooterGap, error) {
	gaps, err := footerHoldsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	for i, gap := range gaps {
		lines, err := s.DisclosuresFor(ctx, tx, DisclosureQuery{Entity: gap.Entity})
		if err != nil {
			return nil, err
		}
		missing := []string{}
		for _, line := range lines {
			if line.Missing() && !slices.Contains(missing, line.Kind) {
				missing = append(missing, line.Kind)
			}
		}
		for _, kind := range gap.Missing {
			if !slices.Contains(missing, kind) {
				missing = append(missing, kind)
			}
		}
		gaps[i].Missing = missing
	}
	return gaps, nil
}

// footerHoldsTx reads each live entity with the holds recorded for it since
// its own row or the installation's particulars last changed. Missing holds
// the kinds those holds recorded.
func footerHoldsTx(ctx context.Context, tx pgx.Tx) ([]FooterGap, error) {
	rows, err := tx.Query(ctx, `
		SELECT le.id, le.is_default, held.person, held.background, held.kinds
		  FROM legal_entity le
		 CROSS JOIN LATERAL (
		       SELECT count(DISTINCT l.id) FILTER (WHERE l.detail->>'background' = 'false') AS person,
		              count(DISTINCT l.id) FILTER (WHERE l.detail->>'background' = 'true') AS background,
		              coalesce(array_agg(DISTINCT k.kind) FILTER (WHERE k.kind IS NOT NULL), '{}') AS kinds
		         FROM system_log l
		         LEFT JOIN LATERAL jsonb_array_elements_text(l.detail->'kinds') AS k(kind) ON true
		        WHERE l.action = $1 AND l.detail->>'legal_entity_id' = le.id::text
		          AND l.occurred_at > greatest(le.updated_at,
		              (SELECT updated_at FROM setting WHERE key = $2))) AS held
		 WHERE le.archived_at IS NULL
		 ORDER BY le.is_default DESC, lower(le.display_name), le.id`,
		FooterHeldAction, ControllerIdentity.Key())
	if err != nil {
		return nil, fmt.Errorf("consent: reading the legal entities a footer may lack details for: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FooterGap, error) {
		var gap FooterGap
		err := row.Scan(&gap.Entity, &gap.IsDefault, &gap.HeldByPerson, &gap.HeldInBackground, &gap.Missing)
		return gap, err
	})
}
