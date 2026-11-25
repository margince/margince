// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InitiativeCarrier keeps a merged-away lead in its sales initiatives, as the
// survivor, in the merge's transaction. Compose binds it.
type InitiativeCarrier func(ctx context.Context, tx pgx.Tx, from, to ids.UUID) error

// WithInitiativeCarrier wires the initiatives-side lead carry.
func (s *Store) WithInitiativeCarrier(carry InitiativeCarrier) *Store {
	s.initiativeCarrier = carry
	return s
}

// WithInitiativeCarrier wires the initiatives-side lead carry.
func (h Handlers) WithInitiativeCarrier(carry InitiativeCarrier) Handlers {
	h.store.WithInitiativeCarrier(carry)
	return h
}

// WithInitiativeCarrier wires the initiatives-side lead carry.
func (p *Provider) WithInitiativeCarrier(carry InitiativeCarrier) *Provider {
	p.store.WithInitiativeCarrier(carry)
	return p
}

// carryInitiativePlacesTx refuses only when an unwired seam would strand a
// lead's place in an initiative, the rule carryStopsTx spells for stops.
func (s *Store) carryInitiativePlacesTx(ctx context.Context, tx pgx.Tx, from, to ids.LeadID) error {
	if s.initiativeCarrier != nil {
		return s.initiativeCarrier(ctx, tx, from.UUID, to.UUID)
	}
	return refuseUnwiredInitiativeCarry(ctx, tx, from)
}

func refuseUnwiredInitiativeCarry(ctx context.Context, tx pgx.Tx, from ids.LeadID) error {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM sales_initiative_member WHERE record_type = 'lead' AND record_id = $1)
		    OR EXISTS (SELECT 1 FROM sales_initiative_enrolment WHERE record_type = 'lead' AND record_id = $1)`, from).Scan(&held); err != nil {
		return fmt.Errorf("contacts: checking whether the retiring lead belongs to an initiative: %w", err)
	}
	if held {
		return &SatelliteCarrierNotWiredError{Holding: "places in sales initiatives"}
	}
	return nil
}
