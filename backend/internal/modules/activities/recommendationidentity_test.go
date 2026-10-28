// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAcceptedObservationIdentityUsesTheInstantAndTheActingUser(t *testing.T) {
	user := ids.NewV7()
	reader := principal.WithActor(context.Background(), principal.Principal{UserID: user})
	receipt := crmcontracts.RecommendationReceipt{CheckedAt: time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)}
	before, err := acceptedRecommendationID(reader, receipt)
	if err != nil {
		t.Fatal(err)
	}
	receipt.CheckedAt = receipt.CheckedAt.In(time.FixedZone("demo", 7*60*60))
	same, err := acceptedRecommendationID(reader, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if same != before {
		t.Fatal("the wire timezone changed the observation's idempotency identity")
	}
	other := principal.WithActor(context.Background(), principal.Principal{UserID: ids.NewV7()})
	different, err := acceptedRecommendationID(other, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if different == before {
		t.Fatal("different users shared an accepted observation identity")
	}
}
