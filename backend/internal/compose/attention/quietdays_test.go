// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The idle count, from the lane to everything that uses it.
//
// It travels typed now, and that is only worth doing if something downstream
// actually depends on the number. Three things do — the sentence the row says,
// the figure the deal card draws, and the age the ordering carries — and none of
// them was held: `quietDaysOf` could be made to answer zero for every row and
// the whole package stayed green, which is a defect that would reach a rep as
// "quiet 0 days" beside a deal nobody has touched since spring.
//
// These run the classifiers directly rather than through a lane, because the
// question is what the number becomes, not how it was measured.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func quietDealItem(days int) crmcontracts.AttentionItem {
	dealID := ids.NewV7()
	name := "Fleet retrofit"
	item := crmcontracts.AttentionItem{
		Id:      dealID.String(),
		Source:  sourceAtRisk,
		Title:   &name,
		Subject: subjectOf(subjectDeal, dealID),
		Actions: []crmcontracts.AttentionItemActions{},
	}
	if days > 0 {
		quiet := days
		item.QuietDays = &quiet
	}
	return item
}

func quietPersonItem(days int) crmcontracts.AttentionItem {
	personID := ids.NewV7()
	name := "Dana Weiss"
	item := crmcontracts.AttentionItem{
		Id:      personID.String(),
		Source:  "relationship_decay",
		Title:   &name,
		Subject: subjectOf("person", personID),
		Actions: []crmcontracts.AttentionItemActions{},
	}
	if days > 0 {
		quiet := days
		item.QuietDays = &quiet
	}
	return item
}

func quietReasonOf(t *testing.T, row crmcontracts.WorklistItem) *crmcontracts.WorklistValue {
	t.Helper()
	for _, reason := range row.Because {
		if reason.Kind == "quiet_days" {
			return reason.Value
		}
	}
	return nil
}

// The sentence the row says. A rep reads "quiet 90 days" and checks it against
// what they know about the account, so the figure has to be the one the lane
// measured rather than a default that survived a lost field.
func TestAQuietDealsOwnCountReachesTheReasonItStates(t *testing.T) {
	row := classifyRisk(quietDealItem(90), rankInstant, materialBar{}, dayMoney{})

	value := quietReasonOf(t, row.item)
	if value == nil {
		t.Fatal("a deal quiet for 90 days says nothing about how long — the reason is absent")
	}
	if value.Days == nil || *value.Days != 90 {
		t.Fatalf("the row says %v days, want the 90 the lane measured", value.Days)
	}
}

// The same count on a decay row, which has no deal facts to carry it: this is
// the source that would lose the number entirely if it rode on the deal.
func TestALapsedRelationshipsCountReachesTheReasonItStates(t *testing.T) {
	row := classifyDecay(quietPersonItem(63), rankInstant)

	value := quietReasonOf(t, row.item)
	if value == nil {
		t.Fatal("a relationship quiet for 63 days says nothing about how long")
	}
	if value.Days == nil || *value.Days != 63 {
		t.Fatalf("the row says %v days, want the 63 the lane measured", value.Days)
	}
}

// The figure the deal's own card draws, beside the sentence. Both come from one
// number, so a reader comparing the card against the line cannot find them
// disagreeing.
func TestTheDealCardsFigureIsTheSameCountTheRowStates(t *testing.T) {
	// A deal whose lane resolved its figures, which is what makes there be a
	// card at all: a row naming a deal and carrying none of its numbers draws
	// no card, and the idle count has nowhere to sit.
	item := quietDealItem(45)
	amount := int64(160_100_00)
	currency := "EUR"
	item.Deal = &crmcontracts.AttentionDealFacts{AmountMinor: &amount, Currency: &currency}
	row := classifyRisk(item, rankInstant, materialBar{}, dayMoney{})

	if row.item.Deal == nil || row.item.Deal.QuietDays == nil {
		t.Fatalf("the deal card carries no idle figure: %+v", row.item.Deal)
	}
	if *row.item.Deal.QuietDays != 45 {
		t.Fatalf("the card says %d and the lane measured 45", *row.item.Deal.QuietDays)
	}
}

// The AGE each row carries into the ordering. The hardest use to see, and the
// reason the count is typed: `ranked.waitingDays` is not drawn anywhere, so a
// wrong value here is invisible on screen.
//
// The count feeds BOTH halves of that step: waitingDays is what it reports —
// the "12 against 30" a reader checks the order against — and waitingRank is
// what it decides on. Asserting only the reported half was how both these
// sources shipped with waitingRank left at its zero value: every drifting deal
// tied on age, the step fell through to the next tie-break, and each row went
// on printing the true count it had not been ordered by.
func TestTheIdleCountBecomesTheAgeTheOrderingCarries(t *testing.T) {
	for _, c := range []struct {
		name string
		row  ranked
	}{
		{"a drifting deal", classifyRisk(quietDealItem(90), rankInstant, materialBar{}, dayMoney{})},
		{"a lapsed relationship", classifyDecay(quietPersonItem(90), rankInstant)},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.row.waitingDays != 90 {
				t.Fatalf("the ordering carries an age of %d, want the 90 days the lane measured — "+
					"this value is never drawn, so a wrong one silently reorders the queue",
					c.row.waitingDays)
			}
			if c.row.waitingRank != waitingDaysCeiling {
				t.Fatalf("the ordering DECIDES on %d, want the ceiling %d — a row that "+
					"leaves this at zero ties with every other and its age decides nothing",
					c.row.waitingRank, waitingDaysCeiling)
			}
		})
	}
}

// And the same age under the ceiling travels whole, so the ordering can tell
// two drifting deals apart rather than only distinguishing "old" from "new".
func TestAnIdleCountUnderTheCeilingDecidesAtItsTrueSize(t *testing.T) {
	row := classifyRisk(quietDealItem(9), rankInstant, materialBar{}, dayMoney{})

	if row.waitingRank != 9 {
		t.Fatalf("a deal quiet 9 days orders at %d, want 9", row.waitingRank)
	}
}

// And the order a rep actually sees, which is the only place any of this shows.
//
// Two drifting deals alike in every other respect: same level, no amount, no
// close date, and — the reason this case is the sharp one — no occurrence
// either, because the risk lane sets none and `occurredOf` falls back to the
// shared read instant for both. So every step above and below waiting-days ties,
// and the age is the whole of what decides. With waitingRank unset they tied
// there too and the input order stood, which meant a deal quiet ninety days sat
// below one quiet three while printing "quiet 90 days" on its own row.
//
// The ids are chosen so the LAST tie-break — `a.item.Id < b.item.Id`, which
// exists to keep a complete tie stable — would put the fresh deal first. A
// fixture whose older row also sorted first by id passed with waitingRank
// unset, proving only that "aged" precedes "fresh" in the alphabet.
func TestTheDealQuietLongestLeadsTheOnesQuietLess(t *testing.T) {
	fresh := quietDealItem(3)
	fresh.Id = "a-quiet-three-days"
	aged := quietDealItem(90)
	aged.Id = "b-quiet-ninety-days"
	// Fresh FIRST too, so a stable sort that decides nothing leaves it there.
	rows := []ranked{
		classifyRisk(fresh, rankInstant, materialBar{}, dayMoney{}),
		classifyRisk(aged, rankInstant, materialBar{}, dayMoney{}),
	}

	assertOrder(t, rankAll(stampAsOf(rows, rankInstant)),
		"b-quiet-ninety-days", "a-quiet-three-days")
}

// A lane that measured no idle time says nothing about it. Zero is a real
// answer here — "not measured" — and printing "quiet 0 days" would be the row
// inventing a fact rather than omitting one.
func TestARowWithNoMeasuredIdleTimeSaysNothingAboutIt(t *testing.T) {
	row := classifyRisk(quietDealItem(0), rankInstant, materialBar{}, dayMoney{})

	if value := quietReasonOf(t, row.item); value != nil {
		t.Fatalf("a deal with no measured idle time still says %v days", value.Days)
	}
	item := quietDealItem(0)
	amount := int64(160_100_00)
	item.Deal = &crmcontracts.AttentionDealFacts{AmountMinor: &amount}
	priced := classifyRisk(item, rankInstant, materialBar{}, dayMoney{})
	if priced.item.Deal == nil {
		t.Fatal("a priced deal drew no card at all")
	}
	if priced.item.Deal.QuietDays != nil {
		t.Fatalf("the card drew an idle figure of %d from nothing", *priced.item.Deal.QuietDays)
	}
}
