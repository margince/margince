// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordParts is change_record_parts' seam: offer lines from the deals
// provider, the company and project parts from contacts.
type recordParts struct {
	offers *Provider
	contacts.RecordParts
}

// AddOfferLine answers the offer with its new line, as the route does.
func (r recordParts) AddOfferLine(ctx context.Context, offerID ids.UUID, line json.RawMessage) (json.RawMessage, error) {
	offer, err := r.offers.deals.AddOfferLine(ctx, offerID, line)
	if err != nil {
		return nil, err
	}
	return json.Marshal(offer)
}

// UpdateOfferLine answers the offer with the line changed.
func (r recordParts) UpdateOfferLine(ctx context.Context, offerID, lineID ids.UUID, line json.RawMessage) (json.RawMessage, error) {
	offer, err := r.offers.deals.UpdateOfferLine(ctx, offerID, lineID, line)
	if err != nil {
		return nil, err
	}
	return json.Marshal(offer)
}

// RemoveOfferLine answers the offer without the line.
func (r recordParts) RemoveOfferLine(ctx context.Context, offerID, lineID ids.UUID) (json.RawMessage, error) {
	offer, err := r.offers.deals.RemoveOfferLine(ctx, offerID, lineID)
	if err != nil {
		return nil, err
	}
	return json.Marshal(offer)
}
