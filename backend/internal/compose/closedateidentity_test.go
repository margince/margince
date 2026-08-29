// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The staging identity names fields the payload actually carries.
//
// canonicalIdentity refuses an identity field the proposed change does not
// carry with the same value, so a JSON tag renamed on one side and not the other
// turns every close-date staging into a runtime error — and it is the NIGHTLY
// sweep that would discover it, in a worker log, after the change shipped.
// Marshalling the real struct is what makes this a fact about the payload rather
// than about two string literals agreeing with each other.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheCloseDateIdentityNamesFieldsThePayloadCarries(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(deals.CloseDateCorrection{
		DealID:            ids.From[ids.DealKind](ids.NewV7()),
		ExpectedCloseDate: "2026-12-01",
		StandingCloseDate: "2026-08-19",
		Basis:             "the date has passed",
	})
	if err != nil {
		t.Fatalf("marshalling a correction: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decoding the payload: %v", err)
	}
	for _, field := range []string{closeDateIdentityDeal, closeDateIdentityStanding} {
		value, carried := payload[field]
		if !carried {
			t.Errorf("the identity names %q, which CloseDateCorrection does not carry — "+
				"every staging would fail in the nightly sweep", field)
			continue
		}
		if _, isString := value.(string); !isString {
			t.Errorf("the identity names %q, which the payload carries as %T — "+
				"an identity field must be a string", field, value)
		}
	}
}

// A deal holding no date at all still has one identity value.
//
// The `missing` flag exists for that deal, and an absent or null key never
// matches by containment: the proposals raised about it would carry no rejection
// memory, and the rep would be asked again every night with nothing recording
// that they had answered.
func TestADealWithNoCloseDateStillHasAStandingValue(t *testing.T) {
	t.Parallel()
	if got := deals.StandingCloseDate(nil); got == "" {
		t.Fatal("a deal with no close date spells its standing date as empty, " +
			"which reads as an absent identity field")
	}
	held := "2026-08-19"
	if got := deals.StandingCloseDate(&held); got != held {
		t.Fatalf("a deal holding %q spells its standing date %q", held, got)
	}
}
