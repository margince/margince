// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// PromotionWitness records, inside a lead's promotion and before the lead is
// archived, what the lead belonged to. at is the promotion instant the lead
// row is stamped with. Compose binds the sales initiative enrolment.
type PromotionWitness interface {
	WitnessLeadPromotionTx(ctx context.Context, tx pgx.Tx, lead ids.UUID, at time.Time) error
}

// WithPromotionWitness wires the witness every promotion runs.
func (s *Store) WithPromotionWitness(witness PromotionWitness) *Store {
	s.promotionWitness = witness
	return s
}

// WithPromotionWitness mirrors the store wiring for the HTTP door.
func (h Handlers) WithPromotionWitness(witness PromotionWitness) Handlers {
	h.store = h.store.WithPromotionWitness(witness)
	return h
}

// WithPromotionWitness mirrors the store wiring for the agent door.
func (p *Provider) WithPromotionWitness(witness PromotionWitness) *Provider {
	p.store = p.store.WithPromotionWitness(witness)
	return p
}

func (s *Store) witnessPromotionTx(ctx context.Context, tx pgx.Tx, lead ids.LeadID, at time.Time) error {
	if s.promotionWitness == nil {
		return nil
	}
	return s.promotionWitness.WitnessLeadPromotionTx(ctx, tx, lead.UUID, at)
}
