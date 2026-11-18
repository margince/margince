// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// observeEveryCategory puts the engine in observe mode, where the legacy
// verdict decides whether a message goes.
func (e *resolveEnv) observeEveryCategory(t *testing.T) {
	t.Helper()
	modes := map[string]string{}
	for _, c := range commsauthz.Categories() {
		modes[string(c)] = string(commsauthz.ModeObserve)
	}
	value, err := json.Marshal(modes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO setting (key, value, updated_at) VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = now()`,
		AuthorizationModes.Key(), string(value)); err != nil {
		t.Fatalf("observing every category: %v", err)
	}
}

// Under observe the legacy verdict decides, and it judges under the country
// the sender stated, as the engine does. The record's market keeps a month-old
// inbound only for a day; the stated one keeps it for the default year.
func TestTheLegacyVerdictFollowsTheStatedCountry(t *testing.T) {
	short := testMessagingRules(t, messagingrules.Rules{Version: 1, ReplyWindow: 24 * time.Hour})
	long := testMessagingRules(t, messagingrules.Rules{Version: 1})
	for name, c := range map[string]struct {
		stated  []commsauthz.CountryOverride
		allowed bool
	}{
		"the record's country": {nil, false},
		"a stated country":     {[]commsauthz.CountryOverride{{Address: "dana@buyer.test", Country: string(long)}}, true},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			e.livesIn(t, string(short))
			e.seedPurpose(t, "business_correspondence", "business_correspondence")
			e.inboundFrom(t, "older-thread", e.address, clocktest.Now(t).Add(-30*24*time.Hour))
			e.observeEveryCategory(t)
			delivery := e.plantDelivery(t)
			recipients := []connector.Recipient{{Email: e.address}}
			if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
				_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
					Recipients: recipients, LegacyPurposeKey: "business_correspondence",
					Subject: "Hello", Body: "Following up.", Evidence: commsauthz.Evidence{RecipientCountries: c.stated},
				})
				return err
			}); err != nil {
				t.Fatalf("staging: %v", err)
			}
			ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
				DeliveryID: delivery, Attempt: 1, Recipients: recipients,
				PurposeKey: "business_correspondence", Subject: "Hello", Body: "Following up.",
			})
			if err != nil {
				t.Fatalf("transmit: %v", err)
			}
			if ticket.Allowed != c.allowed {
				t.Errorf("transmit allowed = %v, want %v", ticket.Allowed, c.allowed)
			}
		})
	}
}
