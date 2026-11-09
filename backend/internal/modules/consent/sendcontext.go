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

// withSendingEntity names the entity a message about to be staged or previewed
// goes out for: the one the caller named, else the sender's. Transmit never
// calls it; it reads the entity staging stored.
func withSendingEntity(ctx context.Context, tx pgx.Tx, req commsauthz.Request) (commsauthz.Request, error) {
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
