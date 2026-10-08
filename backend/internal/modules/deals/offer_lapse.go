// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// OfferLapsedError maps to 422: an offer past its valid-until date is no longer
// a price the seller stands behind, so it cannot go out or re-price the deal.
type OfferLapsedError struct{ ValidUntil string }

func (e *OfferLapsedError) Error() string {
	return "the offer was valid until " + e.ValidUntil + "; regenerate it with a later date"
}

// FieldFault names the date that has passed.
func (e *OfferLapsedError) FieldFault() (field, code, message string) {
	return "valid_until", "offer_lapsed", e.Error()
}

// refuseLapsedOffer is the one rule for "past its date": the day after
// valid_until, in UTC, is the first day the offer is dead.
func (s *Store) refuseLapsedOffer(current crmcontracts.Offer) error {
	if current.ValidUntil == nil {
		return nil
	}
	today := s.clock().UTC().Truncate(24 * time.Hour)
	if current.ValidUntil.Time.Before(today) {
		return &OfferLapsedError{ValidUntil: current.ValidUntil.Format("2006-01-02")}
	}
	return nil
}
