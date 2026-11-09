// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The second look at the wire keeps the issued decision set only while
// everything it was decided on is unchanged.

import (
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

var reviewableRecheckCountry = sync.OnceValue(func() string {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zl", Version: 2})
	return "zl"
})

func TestTheRecheckKeepsTheSetWhileNothingChanged(t *testing.T) {
	e := setupResolve(t)
	delivery, authorize := e.stagedMarketingDelivery(t)
	issued := authorize(1)

	again, err := e.gate.RecheckTransmit(e.ctx, transmitRequestForTest(e, delivery, 1), issued)
	if err != nil {
		t.Fatalf("rechecking: %v", err)
	}
	if again.DecisionSetID != issued.DecisionSetID {
		t.Errorf("an unchanged recheck recorded a new set %s, want the issued %s", again.DecisionSetID, issued.DecisionSetID)
	}
}

// A recipient country that changes between issue and wire selects other rules.
// The answer stays a refusal, and the recheck still records a new set. The
// requirement check reads the rule packs of the set it is given.
func TestACountryChangedBeforeTheWireRecordsANewSet(t *testing.T) {
	e := setupResolve(t)
	delivery, authorize := e.stagedMarketingDelivery(t)
	issued := authorize(1)
	e.livesIn(t, reviewableRecheckCountry())

	fresh, err := e.gate.RecheckTransmit(e.ctx, transmitRequestForTest(e, delivery, 1), issued)
	if err != nil {
		t.Fatalf("rechecking: %v", err)
	}
	if fresh.Allowed != issued.Allowed || fresh.ConsentRefused != issued.ConsentRefused {
		t.Fatalf("the answer itself changed (%v/%v → %v/%v); this case is about a change the answer hides",
			issued.Allowed, issued.ConsentRefused, fresh.Allowed, fresh.ConsentRefused)
	}
	if fresh.DecisionSetID == issued.DecisionSetID {
		t.Fatal("a rule pack changed before the wire and the recheck kept the stale decision set")
	}
	var rows int
	if err := e.owner.QueryRow(e.ctx, `
		SELECT count(*) FROM communication_decision WHERE decision_set_id = $1`, fresh.DecisionSetID).Scan(&rows); err != nil {
		t.Fatalf("reading the new set: %v", err)
	}
	if rows != 1 {
		t.Errorf("the new set holds %d decision rows, want the one recipient recorded again", rows)
	}
}

// transmitRequestForTest is the request stagedMarketingDelivery authorizes.
func transmitRequestForTest(e *resolveEnv, delivery ids.UUID, attempt int) commsauthz.TransmitRequest {
	return commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: attempt, Recipients: []connector.Recipient{{Email: e.address}},
		PurposeKey: "newsletter", Subject: "S", Body: "b",
	}
}
