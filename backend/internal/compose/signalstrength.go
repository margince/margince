// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// signalStrength bridges contacts's §4 relationship-strength computation to
// the slice the warm room consumes (signals.StrengthSource). It carries
// only the score and its bucket across the seam — the full explainable
// decomposition stays with its owner. This is the arch-legal edge: signals
// declares its own seam type, and the cross-module dependency lives here in
// compose, never as a signals→contacts import.
type signalStrength struct{ contacts *contacts.Store }

func (s signalStrength) ContactStrength(ctx context.Context, contactID ids.ContactID, now time.Time) (signals.RelationshipStrength, error) {
	rs, err := s.contacts.ContactStrength(ctx, contactID, now)
	if err != nil {
		return signals.RelationshipStrength{}, err
	}
	return signals.RelationshipStrength{Strength: rs.Strength, Bucket: rs.Bucket}, nil
}
