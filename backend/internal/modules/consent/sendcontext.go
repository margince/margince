// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// withSendFacts completes a message about to be staged or previewed. It names
// the entity it goes out for, the caller's or else the sender's, and reads
// whether its authored words make an offer. Transmit reads both from staging.
func withSendFacts(ctx context.Context, tx pgx.Tx, req commsauthz.Request) (commsauthz.Request, error) {
	req = withAuthoredOffer(req)
	if !req.SendingEntity.IsZero() {
		return req, nil
	}
	entity, err := sendingEntityOf(ctx, tx)
	if err != nil {
		return commsauthz.Request{}, err
	}
	req.SendingEntity = entity
	return req, nil
}

// groundNotice, groundOffer and groundSoldAt are the existing-customer ground a
// decision row stores, NULL when the message did not go out on it.
func groundNotice(d commsauthz.Decision) *ids.UUID {
	if d.ExistingCustomer == nil {
		return nil
	}
	return &d.ExistingCustomer.NoticeID
}

func groundOffer(d commsauthz.Decision) *ids.UUID {
	if d.ExistingCustomer == nil {
		return nil
	}
	return &d.ExistingCustomer.OfferID
}

func groundSoldAt(d commsauthz.Decision) *time.Time {
	if d.ExistingCustomer == nil {
		return nil
	}
	return &d.ExistingCustomer.SoldAt
}
