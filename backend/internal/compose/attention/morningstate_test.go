// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// this_morning_state and the at-risk card's facts, as specs: the state must
// separate the three morning answers an empty lane conflates, and the risk
// card must state the deal's value and ownership without inventing a facts
// object where it has none.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// assembleMorning builds the feed around one briefing stub and returns the day.
func assembleMorning(t *testing.T, briefing stubBriefing) crmcontracts.Attention {
	t.Helper()
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{},
		stubReceipts{}, briefing, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	return out
}

// The three states name why the lane holds what it holds. Empty-because-no-run
// and empty-because-finished render identically as zero rows, and only one has
// earned a tick — so the server says which, instead of leaving the client to
// infer a claim the data cannot carry.
func TestTheMorningStateSeparatesNoRunFromAllAnswered(t *testing.T) {
	for _, tc := range []struct {
		name     string
		briefing stubBriefing
		want     crmcontracts.AttentionThisMorningState
	}{
		{"no run produced overnight", stubBriefing{}, crmcontracts.NoRunToday},
		{"a run whose every item is answered", stubBriefing{ran: true}, crmcontracts.AllAnswered},
		{"items still waiting", stubBriefing{rows: []BriefEntry{
			{ID: ids.NewV7(), DealID: ids.NewV7(), Rank: 1},
		}}, crmcontracts.ItemsWaiting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := assembleMorning(t, tc.briefing)
			if out.ThisMorningState == nil {
				t.Fatal("this_morning_state is absent — the morning's emptiness is ambiguous again")
			}
			if *out.ThisMorningState != tc.want {
				t.Errorf("state = %q, want %q", *out.ThisMorningState, tc.want)
			}
		})
	}
}

// The at-risk card carries the deal's own facts so the client draws value,
// stage and ownership without a second read per row — and carries the `open`
// verb the surface now routes through the subject.
func TestARiskCardCarriesTheDealsFacts(t *testing.T) {
	stage, owner := ids.NewV7(), ids.NewV7()
	amount := int64(250000)
	currency := "EUR"
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil,
		stubAtRisk{rows: []RiskyDeal{{
			DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19,
			StageID: &stage, OwnerID: &owner, AmountMinor: &amount, Currency: &currency,
		}}}, nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	item := (*out.AtRisk)[0]
	if item.Deal == nil {
		t.Fatal("the card carries no deal facts — the client is back to one read per row")
	}
	if item.Deal.AmountMinor == nil || *item.Deal.AmountMinor != amount {
		t.Errorf("amount_minor = %v, want %d", item.Deal.AmountMinor, amount)
	}
	if item.Deal.Currency == nil || *item.Deal.Currency != currency {
		t.Errorf("currency = %v, want %s", item.Deal.Currency, currency)
	}
	if item.Deal.StageId == nil || ids.UUID(*item.Deal.StageId) != stage {
		t.Errorf("stage_id = %v, want the deal's stage", item.Deal.StageId)
	}
	if item.Deal.OwnerId == nil || ids.UUID(*item.Deal.OwnerId) != owner {
		t.Errorf("owner_id = %v, want the deal's owner", item.Deal.OwnerId)
	}
	if len(item.Actions) != 1 || item.Actions[0] != "open" {
		t.Errorf("actions = %v, want [open]", item.Actions)
	}
}

// A deal with no facts sends no facts object: an empty object would say less
// than its absence and still cost every client a nil-vs-empty branch.
func TestARiskCardWithNoFactsSendsNoFactsObject(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{}, nil,
		stubAtRisk{rows: []RiskyDeal{{DealID: ids.NewV7(), Name: "Bare deal", QuietDays: 5}}},
		nil, nil, nil, nil, nil, nil, nil, nil,
		fixedClock)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if item := (*out.AtRisk)[0]; item.Deal != nil {
		t.Errorf("deal facts = %+v, want none for a deal that has none", item.Deal)
	}
}

// The unbound commitments lane is ABSENT, and absent is not withheld: nothing
// names it in lanes_omitted, because nothing was hidden from this reader —
// the installation simply serves no such lane until its writer exists.
func TestAnUnboundCommitmentsLaneIsAbsentNotWithheld(t *testing.T) {
	out := assembleMorning(t, stubBriefing{})
	if out.Commitments != nil {
		t.Errorf("commitments = %v, want no lane at all", *out.Commitments)
	}
	if out.LanesOmitted != nil {
		for _, lane := range *out.LanesOmitted {
			if lane == "commitments" {
				t.Error("lanes_omitted names commitments — absent must not read as withheld")
			}
		}
	}
}
