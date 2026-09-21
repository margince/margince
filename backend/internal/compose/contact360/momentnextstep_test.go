// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The missing-next-step rung: the card this page opens on when a live deal has
// nothing agreed with the contact who sits on it.
//
// It lives apart from the other rungs because its condition is shared. The deal
// card asks the same question of the same kernel predicate, and the two pages
// said opposite things about one deal until they did — so the cases that pin
// what counts as an agreed step belong together and under their own heading.

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASystemMintedTaskDoesNotCountAsTheNextStep(t *testing.T) {
	// A check-in reminder is the product nudging the reader, not a step anybody
	// agreed. Counting one silenced the rung on exactly the contact who most
	// needed it: a deal going quiet is what mints the reminder in the first
	// place, so the symptom was suppressing the finding.
	page := func(capturedBy string) *crmcontracts.Contact360 {
		task := crmcontracts.Activity{
			Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ActivityKindTask,
			OccurredAt: at(1), CapturedBy: &capturedBy,
		}
		return &crmcontracts.Contact360{
			Commercial: &crmcontracts.Contact360Commercial{
				Deal: &crmcontracts.Contact360CommercialDeal{Title: "Expansion"},
				Role: ptr("champion"),
			},
			NextSteps: &struct {
				Data []crmcontracts.Activity `json:"data"`
				Page crmcontracts.PageInfo   `json:"page"`
			}{Data: []crmcontracts.Activity{task}},
		}
	}

	if got := deriveMoment(readerCtx(), now, page("system:time-scan")); got.Rule != crmcontracts.ContactMomentRuleMissingNextStep {
		t.Errorf("rule = %q: a system reminder was read as an agreed next step", got.Rule)
	}
	if got := deriveMoment(readerCtx(), now, page("human:"+ids.NewV7().String())); got.Rule == crmcontracts.ContactMomentRuleMissingNextStep {
		t.Error("a task a colleague filed was not counted as the next step")
	}
}

func TestTheNextStepRungNamesTheRecordedSeat(t *testing.T) {
	for _, tc := range []struct {
		name string
		role *string
		want string
	}{
		{"an influencer", ptr("influencer"), "The deal is open and nothing is scheduled with them. They're the recorded influencer on it."},
		{"an economic buyer", ptr("economic_buyer"), "The deal is open and nothing is scheduled with them. They're the recorded economic buyer on it."},
		// A stakeholder edge may carry no role at all, and a sentence naming
		// one anyway would invent the fact the rung exists to report.
		{"a seat with no role recorded", nil, "The deal is open and nothing is scheduled with them. They're a stakeholder on it."},
		{"a seat whose role is blank", ptr("  "), "The deal is open and nothing is scheduled with them. They're a stakeholder on it."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page := &crmcontracts.Contact360{
				Commercial: &crmcontracts.Contact360Commercial{
					Deal: &crmcontracts.Contact360CommercialDeal{Title: "Expansion"},
					Role: tc.role,
				},
			}
			got := deriveMoment(readerCtx(), now, page)
			if got.Rule != crmcontracts.ContactMomentRuleMissingNextStep {
				t.Fatalf("rule = %q, want missing_next_step", got.Rule)
			}
			if got.WhyNow != tc.want {
				t.Errorf("why now = %q, want %q", got.WhyNow, tc.want)
			}
			if strings.Contains(got.WhyNow, "decides it") {
				t.Error("the sentence still claims this seat decides the deal, which the role vocabulary does not say")
			}
		})
	}
}
