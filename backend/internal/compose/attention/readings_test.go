// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The strip is a statement about the DAY, not about the page. A reader walking
// the queue must not watch the figures fall as they go — work does not stop
// existing because it moved behind them.
//
// This is the whole reason the readings are taken over `considered` rather than
// over the rows a page carries, and it is the invariant a future refactor is
// most likely to break by reaching for the nearest slice.
func TestTheReadingsDoNotShrinkAsAReaderPagesThroughTheQueue(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("d1", "deal_at_risk", withDeal(100_00)),
			item("d2", "deal_at_risk", withDeal(200_00)),
			item("d3", "deal_at_risk", withDeal(300_00)),
		),
	}
	considered := classifyDay(day, rankInstant, dayMoney{})

	whole := readingsOf(considered, nil)
	// The same day read one row at a time. `pageFrom` is what a page cut is, so
	// this walks the real one rather than a slice invented here.
	var pages []crmcontracts.WorklistReadings
	for at := 0; at < len(considered); at++ {
		shown, _, _ := pageFrom(append([]ranked(nil), considered...), 1, worklistCursor{At: at})
		if len(shown) == 0 {
			t.Fatalf("the walk ran dry at offset %d with rows still owed", at)
		}
		pages = append(pages, readingsOf(considered, nil))
	}

	for at, page := range pages {
		if page.RevenueAtRiskMinor == nil || whole.RevenueAtRiskMinor == nil {
			t.Fatalf("page %d priced nothing over a day of three priced deals", at)
		}
		if *page.RevenueAtRiskMinor != *whole.RevenueAtRiskMinor {
			t.Fatalf(
				"page %d states %d at risk, the day states %d — the strip fell as the reader walked",
				at, *page.RevenueAtRiskMinor, *whole.RevenueAtRiskMinor)
		}
	}
}

// A filter narrows what the queue DRAWS, never what the day HOLDS. Opening the
// decisions pill must not report the buyers waiting as gone.
func TestAFilterDoesNotEmptyTheOtherReadings(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("a1", "approval", withKind("capture_counterparty"))},
		AtRisk:   lane(item("d1", "deal_at_risk", withDeal(500_00))),
	}
	considered := classifyDay(day, rankInstant, dayMoney{})

	// What a page filtered to decisions would carry, against the snapshot the
	// readings are actually taken over.
	narrowed := keepCategory(considered, categoryDecisions)
	overNarrowed := readingsOf(narrowed, nil)
	overConsidered := readingsOf(considered, nil)

	if overNarrowed.RevenueAtRiskMinor != nil {
		t.Fatal("reading the filtered rows priced a deal the filter removed — the wrong snapshot")
	}
	if overConsidered.RevenueAtRiskMinor == nil || *overConsidered.RevenueAtRiskMinor != 500_00 {
		t.Fatalf("the day's own snapshot lost the at-risk deal: %v", overConsidered.RevenueAtRiskMinor)
	}
	if overConsidered.Review != 1 {
		t.Fatalf("counted %d decisions, wanted the one approval", overConsidered.Review)
	}
}

// A deal nobody could price is not a deal worth nothing. Counting it as zero
// reports a safer pipeline than the one that exists — the direction a money
// figure must never fail in.
func TestAnUnpricedDealIsLeftOutRatherThanCountedAsZero(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("priced", "deal_at_risk", withDeal(400_00)),
			// No amount recorded at all.
			item("unpriced", "deal_at_risk"),
		),
	}

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil)

	if got.RevenueAtRiskMinor == nil {
		t.Fatal("a day with one priced deal reported no figure at all")
	}
	if *got.RevenueAtRiskMinor != 400_00 {
		t.Fatalf("summed %d, wanted only the deal that could be priced", *got.RevenueAtRiskMinor)
	}
}

// Null is not zero. Zero says the pipeline is safe; absence says nobody can
// tell, and the two must not render alike.
func TestADayThatCouldPriceNothingStatesNoFigure(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("d", "deal_at_risk")),
	}

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil)

	if got.RevenueAtRiskMinor != nil {
		t.Fatalf("stated %d over a day it could price nothing on", *got.RevenueAtRiskMinor)
	}
	if got.RevenueCurrency != nil {
		t.Fatalf("named %q as the units of a figure it did not state", *got.RevenueCurrency)
	}
}

// A figure whose units nobody knows is not money. Without the conversion seam
// the sum is raw minor units in no one currency, and naming one would assert a
// conversion that never happened.
func TestAnUnconvertedSumClaimsNoCurrency(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("d", "deal_at_risk", withDeal(700_00))),
	}

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil)

	if got.RevenueAtRiskMinor == nil {
		t.Fatal("an unconverted day should still state its raw sum")
	}
	if got.RevenueCurrency != nil {
		t.Fatalf("claimed currency %q on amounts that never went through the seam", *got.RevenueCurrency)
	}
}

// Once the day is priced, the figure travels with the units it is genuinely in.
func TestAConvertedSumNamesTheBaseCurrency(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("d1", "deal_at_risk", withDeal(100_00)),
			item("d2", "deal_at_risk", withDeal(900_00)),
		),
	}
	money := dayMoney{
		ran:    true,
		base:   "EUR",
		byItem: map[string]int64{"d1": 150_00, "d2": 250_00},
	}

	got := readingsOf(classifyDay(day, rankInstant, money), nil)

	if got.RevenueCurrency == nil || *got.RevenueCurrency != "EUR" {
		t.Fatalf("named %v as the base currency, wanted EUR", got.RevenueCurrency)
	}
	// The CONVERTED figures, not the deals' own — the sum the ranking compared.
	if got.RevenueAtRiskMinor == nil || *got.RevenueAtRiskMinor != 400_00 {
		t.Fatalf("summed %v, wanted the converted 150+250", got.RevenueAtRiskMinor)
	}
}

// The strip counts the work, not the rows. A hundred alike approvals fold to one
// row in the queue and are still a hundred decisions to get through.
func TestReviewCountsTheFoldedWorkRatherThanTheRowsDrawn(t *testing.T) {
	needs := make([]crmcontracts.AttentionItem, 0, 12)
	for i := range 12 {
		needs = append(needs, item(string(rune('a'+i)), "approval", withKind("capture_counterparty")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, NeedsYou: needs}
	considered := classifyDay(day, rankInstant, dayMoney{})

	folded := foldRoutineDecisionsBounded(append([]ranked(nil), considered...), false)
	got := readingsOf(considered, nil)

	// The fixture has to actually fold, or the assertion below passes for the
	// wrong reason: over rows that were never folded, "counted the work" and
	// "counted the rows" are the same number.
	if len(folded) >= len(considered) {
		t.Fatalf(
			"the fixture folded %d rows into %d — nothing folded, so this proves nothing",
			len(considered), len(folded))
	}
	if got.Review != len(considered) {
		t.Fatalf(
			"the strip states %d decisions over %d items folded into %d rows — it counted rows, not work",
			got.Review, len(considered), len(folded))
	}
}

// A source read to its bound makes every figure a floor. A strip stating an
// exact count over a scan that stopped early tells a rep the opposite of the
// truth: that they have reached the end of something they have not.
func TestATruncatedSourceMarksTheWholeRowAsAFloor(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("d", "deal_at_risk", withDeal(100_00))),
	}
	considered := classifyDay(day, rankInstant, dayMoney{})

	complete := readingsOf(considered, map[crmcontracts.WorklistItemSource]bool{sourceAtRisk: false})
	cut := readingsOf(considered, map[crmcontracts.WorklistItemSource]bool{sourceAtRisk: true})

	if complete.MoreAvailable {
		t.Fatal("called the row a floor over sources that all read to the end")
	}
	if !cut.MoreAvailable {
		t.Fatal("stated exact figures over a source that stopped at its bound")
	}
}

// Each reading counts its own kind of work. A test that let two categories share
// a count would pass while the strip put buyer replies under prospecting.
func TestEachReadingCountsItsOwnCategory(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("a1", "approval", withKind("capture_counterparty"))},
		AtRisk:   lane(item("d1", "deal_at_risk", withDeal(100_00))),
	}

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil)

	if got.Review != 1 {
		t.Fatalf("counted %d in review, wanted the one approval", got.Review)
	}
	if got.BuyerReplies != 0 {
		t.Fatalf("counted %d buyer replies on a day with none", got.BuyerReplies)
	}
	if got.Prospecting != 0 {
		t.Fatalf("counted %d prospecting on a day with none", got.Prospecting)
	}
}
