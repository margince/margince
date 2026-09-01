// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a wait is worth as it ages.
//
// The live page put eight half-year-old threads at the top of a working rep's
// day, because age raised urgency forever and nothing ever aged out. These are
// the rules that ended that, each with the case that would bring it back.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A fortnight of silence with no money behind it is not today's work. The row
// stays — somebody did write — but it stops claiming the top band.
func TestAStaleUnfundedWaitLeavesTheTopBand(t *testing.T) {
	waiting := WaitingCustomer{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000c1"),
		Subject:    "Re: the brochure",
		Since:      rankInstant.Add(-40 * 24 * time.Hour),
	}

	row := classifyWaiting(waiting, rankInstant)

	if row.item.Level == levelWaiting {
		t.Fatal("a forty-day unanswered thread still led the day as a live wait")
	}
	if row.item.Source != sourceWaiting {
		t.Fatalf("the row changed source to %q — it is still somebody waiting", row.item.Source)
	}
	var said bool
	for _, because := range row.item.Because {
		if because.Kind == "stale" {
			said = true
		}
	}
	if !said {
		t.Fatal("the row was demoted without saying why, which reads as a ranking bug to whoever finds it")
	}
}

// Money keeps a long wait in the day. Where a deal is open the silence IS the
// problem, and demoting it would hide the case the rep most needs.
func TestAnOpenDealKeepsALongWaitInTheTopBand(t *testing.T) {
	waiting := WaitingCustomer{
		ActivityID:  ids.MustParse("01a05500-0000-7000-8000-0000000000c2"),
		Subject:     "Re: the contract",
		Since:       rankInstant.Add(-40 * 24 * time.Hour),
		HasOpenDeal: true,
	}

	row := classifyWaiting(waiting, rankInstant)

	if row.item.Level != levelWaiting {
		t.Fatalf("a forty-day wait on an OPEN deal was demoted to level %d", row.item.Level)
	}
}

// The boundary itself, from both sides. A rule stated in days is wrong by a day
// exactly as often as it is right, and only the pair catches it.
func TestTheStaleBoundaryIsWhereItSaysItIs(t *testing.T) {
	at := func(days int) ranked {
		return classifyWaiting(WaitingCustomer{
			ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000c3"),
			Since:      rankInstant.Add(-time.Duration(days) * 24 * time.Hour),
		}, rankInstant)
	}

	if got := at(waitingStaleDays); got.item.Level != levelWaiting {
		t.Fatalf("a wait of exactly %d days was already stale (level %d)", waitingStaleDays, got.item.Level)
	}
	if got := at(waitingStaleDays + 1); got.item.Level == levelWaiting {
		t.Fatalf("a wait of %d days was still live", waitingStaleDays+1)
	}
}

// Age stops buying precedence at the ceiling. Two ancient waits tie, and what
// separates them is the next tie-break — not which was ignored longer.
func TestAgeStopsRaisingUrgencyAtTheCeiling(t *testing.T) {
	older := classifyWaiting(WaitingCustomer{
		ActivityID:  ids.MustParse("01a05500-0000-7000-8000-0000000000d1"),
		Since:       rankInstant.Add(-time.Duration(waitingDaysCeiling+150) * 24 * time.Hour),
		HasOpenDeal: true,
	}, rankInstant)
	newer := classifyWaiting(WaitingCustomer{
		ActivityID:  ids.MustParse("01a05500-0000-7000-8000-0000000000d2"),
		Since:       rankInstant.Add(-time.Duration(waitingDaysCeiling+10) * 24 * time.Hour),
		HasOpenDeal: true,
	}, rankInstant)

	if older.waitingRank != newer.waitingRank {
		t.Fatalf("waits of %d and %d ordering days past the ceiling did not tie",
			older.waitingRank, newer.waitingRank)
	}
	if older.waitingRank != waitingDaysCeiling {
		t.Fatalf("the ordering age was %d, wanted the ceiling %d", older.waitingRank, waitingDaysCeiling)
	}
	// And the two still SAY different ages, because they waited different
	// lengths of time. Bounding the order must not reach the reader.
	if older.waitingDays == newer.waitingDays {
		t.Fatalf("both rows reported %d days, so the bound reached what the reader is told",
			older.waitingDays)
	}
}

// The comparison a reader is shown reports the TRUE ages. A row saying "waiting
// 183 days" whose own explanation compares 30 against 20 is a reason nobody can
// check — the failure the comparator exists to avoid, reintroduced by bounding
// the field it reads.
func TestTheComparisonReportsTheAgeTheRowClaims(t *testing.T) {
	older := classifyWaiting(WaitingCustomer{
		ActivityID:  ids.MustParse("01a05500-0000-7000-8000-0000000000f1"),
		Since:       rankInstant.Add(-183 * 24 * time.Hour),
		HasOpenDeal: true,
	}, rankInstant)
	newer := classifyWaiting(WaitingCustomer{
		ActivityID:  ids.MustParse("01a05500-0000-7000-8000-0000000000f2"),
		Since:       rankInstant.Add(-20 * 24 * time.Hour),
		HasOpenDeal: true,
	}, rankInstant)

	got := compare(older, newer)

	if got.Comparator != crmcontracts.WorklistComparisonComparatorWaitingDays {
		t.Fatalf("the ages decided this pair, and the comparison named %q", got.Comparator)
	}
	if got.Mine == nil || got.Mine.Days == nil || *got.Mine.Days != 183 {
		t.Fatalf("the comparison reported %v days, and the row says it waited 183", got.Mine)
	}
}

// The row still tells the truth about its age. Capping the SORT must not cap
// what the reader is told, or the page says a customer has waited a month when
// they have waited half a year.
func TestTheRowStillReportsTheRealWait(t *testing.T) {
	const realDays = waitingDaysCeiling + 153

	row := classifyWaiting(WaitingCustomer{
		ActivityID:  ids.MustParse("01a05500-0000-7000-8000-0000000000d3"),
		Since:       rankInstant.Add(-realDays * 24 * time.Hour),
		HasOpenDeal: true,
	}, rankInstant)

	var reported *crmcontracts.WorklistValue
	for _, because := range row.item.Because {
		if because.Kind == "waiting_days" {
			reported = because.Value
		}
	}
	if reported == nil || reported.Days == nil {
		t.Fatal("the row never said how long they had waited")
	}
	if *reported.Days != realDays {
		t.Fatalf("the row reported %d days of waiting, and they have waited %d",
			*reported.Days, realDays)
	}
}

// The defect this whole change exists to end: an ancient thread outranking a
// deal that closes today, purely because nobody had answered it for longer.
func TestAnAncientWaitNoLongerOutranksTodaysDeal(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("closing", "deal_at_risk", withDeal(80_000_00))),
	}
	waiting := []WaitingCustomer{{
		ActivityID: ids.MustParse("01a05500-0000-7000-8000-0000000000e1"),
		Subject:    "Re: your enquiry",
		Since:      rankInstant.Add(-182 * 24 * time.Hour),
	}}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, waiting)

	if out.Queue[0].Source == sourceWaiting {
		t.Fatal("a 182-day unanswered thread still led the day over a deal at risk")
	}
}

// The page states the bar a deal had to clear.
//
// Every "material" and "below material" reason is a verdict, and a verdict
// whose threshold is withheld cannot be checked: a reader sees that a deal was
// called big and has no way to ask compared to what. The contract has promised
// this figure since the queue shipped.
func TestTheSummaryStatesTheMaterialBar(t *testing.T) {
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("small", "deal_at_risk", withDeal(10_000_00)),
			item("big", "deal_at_risk", withDeal(200_000_00)),
		),
	}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, nil)

	if out.Summary.MaterialThresholdMinor == nil {
		t.Fatal("a day with priced deals stated no material bar, so every material reason on it is unverifiable")
	}
	// The LOWER median of the two, which is the smaller — the rule the bar's
	// own doc states, asserted here so a change to it fails a test rather than
	// silently moving what counts as a big deal.
	if *out.Summary.MaterialThresholdMinor != 10_000_00 {
		t.Fatalf("the bar was %d, wanted the lower median 1000000", *out.Summary.MaterialThresholdMinor)
	}
}

// A pipeline with no priced deals states no bar rather than zero. Zero would
// say every deal is material, which is the opposite of what an absent median
// means.
func TestADayWithNoPricedDealsStatesNoBar(t *testing.T) {
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: lane(item("unpriced", "deal_at_risk"))}

	out := (&Service{}).worklistFrom(context.Background(), day, scopeAll, "", 25, nil)

	if out.Summary.MaterialThresholdMinor != nil {
		t.Fatalf("a pipeline with no prices stated a bar of %d", *out.Summary.MaterialThresholdMinor)
	}
}
