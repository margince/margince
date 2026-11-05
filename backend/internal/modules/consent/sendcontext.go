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
	entity, err := sendingEntityTx(ctx, tx, senderOf(ctx))
	if err != nil {
		return commsauthz.Request{}, err
	}
	req.SendingEntity = entity
	return req, nil
}

// canonicalPurpose is the one purpose key a message is judged, unsubscribed
// and stored under. A legacy key and a marketing purpose that name two
// different purposes are one message claiming two things, and it is refused.
func canonicalPurpose(req commsauthz.Request) (commsauthz.Request, bool) {
	switch {
	case req.MarketingPurpose == "":
	case req.LegacyPurposeKey == "":
		req.LegacyPurposeKey = req.MarketingPurpose
	case normalizedPurposeKey(req.LegacyPurposeKey) != normalizedPurposeKey(req.MarketingPurpose):
		return req, false
	}
	return req, true
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
