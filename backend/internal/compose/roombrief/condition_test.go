// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSetsConditionReadsAnEnglishSigningCondition(t *testing.T) {
	for body, want := range map[string]bool{
		"Clause 11.2 caps liability at fees paid. Our legal team will not sign until the cap is twelve months of fees.": true,
		"We can't approve this without a data processing addendum.":                                                     true,
		"We are not going to proceed unless the SLA changes.":                                                           true,
		"Legal needs the cap raised before we can sign.":                                                                true,
		"Could you resend the order form? The link still shows the old version.":                                        false,
		"Who from your side joins the kickoff call?":                                                                    false,
		"We will sign on Friday.":                                                                                       false,
		"Reviewing now, will come back to you.":                                                                         false,
	} {
		if got := setsCondition(body); got != want {
			t.Errorf("setsCondition(%q) = %v, want %v", body, got, want)
		}
	}
}

// A buyer's condition is marked on a waiting thread and nowhere else. The
// thread keeps its place: the floor reads the same order.
func TestOnlyAWaitingBuyersConditionIsMarked(t *testing.T) {
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	says := func(side, body string, ago time.Duration, state string) crmcontracts.DealRoomThread {
		return crmcontracts.DealRoomThread{
			Id: openapi_types.UUID(ids.NewV7()), State: state, CreatedAt: at.Add(-ago),
			Comments: []crmcontracts.DealRoomComment{{
				Author: crmcontracts.DealRoomAuthor{Name: "Someone", Side: side}, Body: body, CreatedAt: at.Add(-ago),
			}},
		}
	}
	const condition = "We will not sign until the cap is raised."
	in := FromRoom(crmcontracts.DealRoom{Title: "Room", State: "live"}, nil, nil, []crmcontracts.DealRoomThread{
		says("buyer", "Could you resend the order form?", time.Hour, "open"),
		says("buyer", condition, 48*time.Hour, "open"),
		says("seller", condition, 2*time.Hour, "open"),
		says("buyer", condition, 3*time.Hour, "resolved"),
	})
	if in.Threads[0].BuyerSetsCondition || !in.Threads[1].BuyerSetsCondition {
		t.Errorf("the condition moved or went unmarked: %+v", in.Threads)
	}
	marked := 0
	for _, thread := range in.Threads {
		if thread.BuyerSetsCondition {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("%d threads are marked, want only the waiting buyer's", marked)
	}
}
