// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// OfferLapsedError maps to 422: an offer past its valid-until date is no longer
// a price the seller stands behind, so it cannot go out or re-price the deal.
// Lapse is computed here rather than stored as the offer's `expired` status,
// because nothing sweeps offers and a status nobody sets would read as
// "still valid".
type OfferLapsedError struct{ ValidUntil string }

func (e *OfferLapsedError) Error() string {
	return "the offer was valid until " + e.ValidUntil + "; regenerate it with a later date"
}

// FieldFault names the date that has passed.
func (e *OfferLapsedError) FieldFault() (field, code, message string) {
	return "valid_until", "offer_lapsed", e.Error()
}

// refuseLapsedOffer refuses an offer whose valid_until day is before the
// installation's today, the same day INV-CLOSE-PAST reads.
func (s *Store) refuseLapsedOffer(ctx context.Context, tx pgx.Tx, current crmcontracts.Offer) error {
	if current.ValidUntil == nil {
		return nil
	}
	today, err := s.installationToday(ctx, tx)
	if err != nil {
		return err
	}
	return lapsedOn(current.ValidUntil.Time, today)
}

// lapsedOn is the comparison alone: the offer is dead from the day after its
// valid_until day.
func lapsedOn(validUntil, today time.Time) error {
	if validUntil.Before(today) {
		return &OfferLapsedError{ValidUntil: validUntil.Format("2006-01-02")}
	}
	return nil
}
