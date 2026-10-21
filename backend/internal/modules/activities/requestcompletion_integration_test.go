// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"
)

func TestAssertedInboundNeverOffersARequestForSettlement(t *testing.T) {
	for _, test := range []struct {
		name, sender string
		delay        time.Duration
		want         bool
	}{
		{"matching correspondent", requestCounterparty, time.Hour, false},
		{"other correspondent", "other@customer.test", time.Hour, false},
		{"future evidence", requestCounterparty, 72 * time.Hour, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := setupLoad(t)
			store := storeKnowing(e)
			request := seedEmailRequest(t, e, "Signature requested", "commitment", OwedVerdictAsksUs)
			var key string
			if err := e.owner.QueryRow(e.as(), `SELECT thread_key FROM activity WHERE id = $1`, request).Scan(&key); err != nil {
				t.Fatal(err)
			}
			at := requestInstant.Add(test.delay)
			message, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
				Kind: "email", Subject: new("Signature completed"), Body: new("Envelope alpha is fully signed."),
				Direction: new("inbound"), ThreadKey: key, Source: "test", OccurredAt: &at,
				CounterpartyEmail: test.sender,
			})
			if err != nil {
				t.Fatal(err)
			}
			candidate, found := repliedIDs(t, e, requestInstant.Add(48*time.Hour))[request]
			if found != test.want {
				t.Fatalf("request eligible=%v, want %v", found, test.want)
			}
			if found && candidate.NewestAnswerID.String() != message.Id.String() {
				t.Fatal("settlement watermark did not name the later inbound evidence")
			}
		})
	}
}
