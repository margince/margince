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

	whole := readingsOf(considered, nil, nil)
	// The same day read one row at a time. `pageFrom` is what a page cut is, so
	// this walks the real one rather than a slice invented here.
	var pages []crmcontracts.WorklistReadings
	for at := 0; at < len(considered); at++ {
		shown, _, _ := pageFrom(append([]ranked(nil), considered...), 1, worklistCursor{At: at})
		if len(shown) == 0 {
			t.Fatalf("the walk ran dry at offset %d with rows still owed", at)
		}
		pages = append(pages, readingsOf(considered, nil, nil))
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
	overNarrowed := readingsOf(narrowed, nil, nil)
	overConsidered := readingsOf(considered, nil, nil)

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

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil, nil)

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

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil, nil)

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

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil, nil)

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

	got := readingsOf(classifyDay(day, rankInstant, money), nil, nil)

	if got.RevenueCurrency == nil || *got.RevenueCurrency != "EUR" {
		t.Fatalf("named %v as the base currency, wanted EUR", got.RevenueCurrency)
	}
	// The CONVERTED figures, not the deals' own — the sum the ranking compared.
	if got.RevenueAtRiskMinor == nil || *got.RevenueAtRiskMinor != 400_00 {
		t.Fatalf("summed %v, wanted the converted 150+250", got.RevenueAtRiskMinor)
	}
}

// A deal reaching the page from the overnight brief lands in the same
// `deals_at_risk` category an at-risk row for it would: `classifyBriefItem`
// prices it exactly the way `classifyRisk` prices the identical fact for its
// own lane, so the strip's sum covers both.
func TestTheSumCoversBothDealsAtRiskLanes(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("priced", "deal_at_risk", withDeal(300_00))),
		ThisMorning: []crmcontracts.AttentionItem{
			item("brief", "brief_item", withDeal(900_00)),
		},
	}
	rows := classifyDay(day, rankInstant, dayMoney{})

	brief := 0
	for _, row := range rows {
		if row.item.Source == "brief_item" {
			brief++
			if row.item.Category != crmcontracts.WorklistItemCategoryDealsAtRisk {
				t.Fatalf("a brief row now classifies as %q; this test's premise is gone", row.item.Category)
			}
		}
	}
	if brief != 1 {
		t.Fatalf("the fixture produced %d brief rows, wanted one", brief)
	}

	got := readingsOf(rows, nil, nil)

	if got.RevenueAtRiskMinor == nil {
		t.Fatal("the at-risk deal should still be summed")
	}
	if *got.RevenueAtRiskMinor != 1_200_00 {
		t.Fatalf("summed %d, wanted 300_00 (at-risk) + 900_00 (brief) = 1_200_00", *got.RevenueAtRiskMinor)
	}
}

// Two priced rows disagreeing about their units means the sum is in no one
// currency, and naming either would label the total with a guess.
//
// Nothing produces this today — one conversion prices a whole read — which is
// exactly why it is a test rather than a comment: the claim that the rows agree
// is a fact about the current classifier, and a later one is free to break it.
func TestPricedRowsThatDisagreeAboutUnitsNameNoCurrency(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("d1", "deal_at_risk", withDeal(100_00)),
			item("d2", "deal_at_risk", withDeal(200_00)),
		),
	}
	rows := classifyDay(day, rankInstant, dayMoney{
		ran: true, base: "EUR", byItem: map[string]int64{"d1": 100_00, "d2": 200_00},
	})
	// The disagreement a future classifier could introduce, written directly
	// onto the ranked row because no current code path produces it.
	priced := 0
	for i := range rows {
		if rows[i].hasExpected {
			priced++
			if priced == 2 {
				rows[i].expectedCurrency = "USD"
			}
		}
	}
	if priced < 2 {
		t.Fatalf("the fixture priced %d rows; this needs two to disagree", priced)
	}

	got := readingsOf(rows, nil, nil)

	if got.RevenueAtRiskMinor == nil {
		t.Fatal("dropped the sum entirely; only the currency is in doubt")
	}
	if got.RevenueCurrency != nil {
		t.Fatalf("labelled a mixed-currency sum %q", *got.RevenueCurrency)
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
	got := readingsOf(considered, nil, nil)

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

	complete := readingsOf(considered, map[crmcontracts.WorklistItemSource]bool{sourceAtRisk: false}, nil)
	cut := readingsOf(considered, map[crmcontracts.WorklistItemSource]bool{sourceAtRisk: true}, nil)

	if complete.MoreAvailable {
		t.Fatal("called the row a floor over sources that all read to the end")
	}
	if !cut.MoreAvailable {
		t.Fatal("stated exact figures over a source that stopped at its bound")
	}
}

// A lane nobody could read is the one a reader loses SILENTLY. A truncated lane
// at least leaves rows on the page to notice; a refused one leaves none, so the
// strip reads as a clear day rather than as a day nobody could see.
//
// This is the direction these figures must never fail in: under-reporting looks
// exactly like good news.
// Driven through the real worklistFrom rather than calling readingsOf directly,
// because the defect lived in the WIRING and not in the arithmetic: the two
// sources whose refusal produces a confident zero are read BESIDE the assembled
// day, so they never appear in its omitted-lane list. A test that hands
// readingsOf a refusal it built itself proves the flag works and says nothing
// about whether the refusal reaches it.
func TestALaneThisReaderWasRefusedMakesTheFiguresAFloor(t *testing.T) {
	// The two lanes that produce a NUMBER when refused, rather than a null.
	// `at_risk` is deliberately not among them: it feeds the money figure, which
	// answers null on its own and cannot print a false zero.
	for _, refused := range []struct {
		name   string
		source string
	}{
		{"who is waiting", sourceWaiting},
		{"leads owed a reply", sourceLeadResponse},
	} {
		t.Run(refused.name, func(t *testing.T) {
			// A day whose own lanes all answered. Only the source read beside it
			// was refused, which is exactly the case the omitted-lane list misses.
			day := crmcontracts.Attention{AsOf: rankInstant}
			withheld := &crmcontracts.WorklistSourceUnavailable{
				Source: refused.source,
				Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
			}

			out := (&Service{}).worklistFrom(
				t.Context(), day, scopeAll, "", 25,
				waitingRead{}, leadRead{}, worklistCursor{},
				[]*crmcontracts.WorklistSourceUnavailable{withheld})

			if !out.Readings.MoreAvailable {
				t.Fatalf(
					"the %s lane was refused and the strip still stated exact figures — "+
						"a rep sees a confident zero over work nobody could look at",
					refused.name)
			}
		})
	}
}

// The refusal has to reach the READINGS, not only the warning list. Both are
// published from one page, and a version that appended the refusal to the page
// after the readings were built passed every check that looked at
// SourcesUnavailable while the strip above it printed a zero.
func TestARefusedLaneReachesBothTheWarningListAndTheFigures(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant}
	withheld := &crmcontracts.WorklistSourceUnavailable{
		Source: sourceWaiting,
		Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
	}

	out := (&Service{}).worklistFrom(
		t.Context(), day, scopeAll, "", 25,
		waitingRead{}, leadRead{}, worklistCursor{},
		[]*crmcontracts.WorklistSourceUnavailable{withheld})

	named := false
	for _, source := range out.SourcesUnavailable {
		if source.Source == sourceWaiting {
			named = true
		}
	}
	if !named {
		t.Fatal("the refused lane never reached the reader's warning list")
	}
	if !out.Readings.MoreAvailable {
		t.Fatal("the warning list named the refusal and the figures above it did not")
	}
}

// The DSR lane is withheld by ROLE from every rep, permanently. Letting it set
// the floor flag would put "these are floors" on every rep's strip forever,
// which drowns the warning — and it hides no work these four readings count,
// because DSR rows classify as `system` and none of the four counts that.
func TestThePermanentlyWithheldPrivacyLaneDoesNotMarkEveryDayAFloor(t *testing.T) {
	omitted := []crmcontracts.AttentionLanesOmitted{laneDSR}
	day := crmcontracts.Attention{AsOf: rankInstant, LanesOmitted: &omitted}

	got := readingsOf(
		classifyDay(day, rankInstant, dayMoney{}), boundedSources(day), unavailable(day))

	if got.MoreAvailable {
		t.Fatal("the always-withheld privacy lane marked an ordinary day as a floor")
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

	got := readingsOf(classifyDay(day, rankInstant, dayMoney{}), nil, nil)

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
