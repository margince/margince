// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A merge joins two records of one real contact, company or lead, so a share
// given on the retired record keeps working on the survivor. identity owns
// record_grant and the audit shape of a grant change; contacts only asks,
// inside the merge's transaction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GrantCarrier moves the grants on a retiring record onto its survivor, folding
// two grants of one grantee into one.
type GrantCarrier interface {
	CarryGrantsTx(ctx context.Context, tx pgx.Tx, recordType string, from, to ids.UUID) error
}

// WithGrantCarrier wires the identity-side grant carry. Compose binds it.
func (s *Store) WithGrantCarrier(carrier GrantCarrier) *Store {
	s.grantCarrier = carrier
	return s
}

// carryGrantsTx refuses only when an unwired seam would strand a grant, the
// rule carryIntrosTx spells for introduction asks.
func (s *Store) carryGrantsTx(ctx context.Context, tx pgx.Tx, recordType string, from, to ids.UUID) error {
	if s.grantCarrier != nil {
		return s.grantCarrier.CarryGrantsTx(ctx, tx, recordType, from, to)
	}
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM record_grant WHERE record_type = $1 AND record_id = $2)`,
		recordType, from).Scan(&held); err != nil {
		return fmt.Errorf("contacts: checking whether the retiring %s is shared with anyone: %w", recordType, err)
	}
	if held {
		return &SatelliteCarrierNotWiredError{Holding: "shares with colleagues"}
	}
	return nil
}
