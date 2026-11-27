// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// FooterGap is what one live legal entity's mail owes and nobody stated.
type FooterGap struct {
	Entity    ids.UUID
	IsDefault bool
	// Missing lists the owed kinds with no text, in the order the send names
	// them. Empty when the entity's footer is complete.
	Missing []string
	// HeldByUser and HeldInBackground count the messages held for
	// footer_incomplete since the details last changed.
	HeldByUser, HeldInBackground int
}

// footerHoldLookback bounds how far back a hold still names a missing kind.
// A kind only a recipient's pack owes is known from the holds alone.
const footerHoldLookback = "90 days"

// FooterGapsTx answers each live legal entity. A missing kind is one that
// DisclosuresFor reports missing for a first message from that entity. It is
// also a kind a recent held send recorded, while its particular is empty. The
// send asks the same two questions, so the answers cannot differ.
// Only a seat that may change the particulars is told.
func (s *Store) FooterGapsTx(ctx context.Context, tx pgx.Tx) ([]FooterGap, error) {
	if err := auth.Require(ctx, controllerParticularsObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	gaps, err := footerHoldsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	installation, err := config.StoredApplyTx(ctx, tx, config.Bound(ctx), ControllerIdentity)
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
		facts, err := sendingEntityFactsTx(ctx, tx, gap.Entity)
		if err != nil {
			return nil, err
		}
		particulars := controllerOf(installation, facts)
		for _, kind := range gap.Missing {
			if particularFor(messagingrules.DisclosureKind(kind), particulars) == "" && !slices.Contains(missing, kind) {
				missing = append(missing, kind)
			}
		}
		gaps[i].Missing = missing
	}
	return gaps, nil
}

// footerHoldsTx reads each live entity with the kinds its recent holds
// recorded. It counts the messages held since the entity's row or the
// installation's particulars last changed. A message pressed twice counts once.
func footerHoldsTx(ctx context.Context, tx pgx.Tx) ([]FooterGap, error) {
	rows, err := tx.Query(ctx, `
		SELECT le.id, le.is_default, held.by_user, held.background, held.kinds
		  FROM legal_entity le
		 CROSS JOIN LATERAL (
		       SELECT count(DISTINCT coalesce(l.detail->>'intent_id', l.id::text))
		                FILTER (WHERE l.detail->>'background' = 'false' AND l.occurred_at > changed.at) AS by_user,
		              count(DISTINCT coalesce(l.detail->>'intent_id', l.id::text))
		                FILTER (WHERE l.detail->>'background' = 'true' AND l.occurred_at > changed.at) AS background,
		              coalesce(array_agg(DISTINCT k.kind) FILTER (WHERE k.kind IS NOT NULL), '{}') AS kinds
		         FROM (SELECT greatest(le.updated_at, (SELECT updated_at FROM setting WHERE key = $2)) AS at) AS changed
		         LEFT JOIN system_log l
		           ON l.action = $1 AND l.detail->>'legal_entity_id' = le.id::text
		          AND l.occurred_at > now() - $3::interval
		         LEFT JOIN LATERAL jsonb_array_elements_text(l.detail->'kinds') AS k(kind) ON true) AS held
		 WHERE le.archived_at IS NULL
		 ORDER BY le.is_default DESC, lower(le.display_name), le.id`,
		FooterHeldAction, ControllerIdentity.Key(), footerHoldLookback)
	if err != nil {
		return nil, fmt.Errorf("consent: reading the legal entities a footer may lack details for: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (FooterGap, error) {
		var gap FooterGap
		err := row.Scan(&gap.Entity, &gap.IsDefault, &gap.HeldByUser, &gap.HeldInBackground, &gap.Missing)
		return gap, err
	})
}
