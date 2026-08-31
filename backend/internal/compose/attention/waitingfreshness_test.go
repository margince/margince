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

	if older.waitingDays != newer.waitingDays {
		t.Fatalf("waits of %d and %d ordering days past the ceiling did not tie",
			older.waitingDays, newer.waitingDays)
	}
	if older.waitingDays != waitingDaysCeiling {
		t.Fatalf("the ordering age was %d, wanted the ceiling %d", older.waitingDays, waitingDaysCeiling)
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
