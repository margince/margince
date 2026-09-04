// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The order ACROSS sources, which is the page's whole promise.
//
// Every other ordering test in this package compares rows of one kind, or rows
// built by hand with the deciding field set directly. Neither can catch the
// failure a reader would actually report: a buyer waiting on a reply sitting
// below a broken automation. The comparison that matters is between a waiting
// customer, a promise the rep made, a meeting about to start, a material deal
// at risk and a routine decision — five sources, five classifiers, one order.
//
// Built through the real classifiers rather than through `candidate`, because
// what is under test is which level and which reasons each SOURCE produces. A
// fixture that set the levels itself would assert the ranking against the
// answer it was handed.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// aWholeDay classifies one of each kind the way worklist.go does — the lane
// feed through classifyDay, the waiting seam through classifyWaiting — and
// ranks the union.
func aWholeDay(waits []WaitingCustomer, day crmcontracts.Attention) []crmcontracts.WorklistItem {
	rows := classifyDay(day, rankInstant, dayMoney{})
	for _, wait := range waits {
		rows = append(rows, classifyWaiting(wait, rankInstant))
	}
	return rankAll(stampAsOf(rows, rankInstant))
}

// The order a rep would call right, over the five kinds at once.
//
// A meeting inside the horizon and a customer waiting share the top level, and
// that is deliberate: both are somebody else's clock running. The meeting leads
// because it carries a DEADLINE and the wait does not — it happens at a stated
// minute whether or not the reader acts, while a reply can be sent at any point
// today. Then the promise the rep made, then revenue at risk, then hygiene.
//
// Nothing here sets a level by hand: each row's place is its classifier's own
// answer, which is what makes this a test of the ranking rather than of the
// fixture.
func TestTheDayIsOrderedAcrossItsSourcesAndNotOnlyWithinThem(t *testing.T) {
	t.Parallel()

	soon := rankInstant.Add(90 * time.Minute)
	got := aWholeDay(
		[]WaitingCustomer{{
			ActivityID: waitActivity,
			Subject:    "Can you confirm the retrofit price?",
			Since:      rankInstant.Add(-72 * time.Hour),
			PersonID:   waitPerson,
			// A thread the workspace has written on before. Without it the row
			// is an UNPROVEN wait, which classifyWaiting demotes to routine on
			// purpose — a correct answer to a different question, and one that
			// would leave this test asserting the cross-source order of rows
			// that no longer span the levels it is about.
			Engaged: true,
		}},
		crmcontracts.Attention{
			AsOf:        rankInstant,
			Commitments: lane(item("promise", "conversation_claim", withDue(soon))),
			Meetings:    lane(item("meeting", "meeting", withDue(soon))),
			AtRisk:      lane(item("risk", "deal_at_risk", withDeal(900_000_00))),
			Notices:     lane(item("notice", "notice")),
		},
	)

	assertOrder(t, got,
		"meeting",             // somebody else's clock, at a stated minute
		waitActivity.String(), // somebody else's clock, all day
		"promise",             // a promise the rep made
		"risk",                // revenue at risk
		"notice",              // hygiene
	)
}

// The claim the ordering rests on, asserted where a reader would notice it
// breaking: hygiene never outranks a customer, however loud the hygiene is.
//
// A broken automation reporting twelve failures is the row most likely to look
// urgent to whoever wrote the lane, and it is the one that must not lead.
func TestSystemNewsNeverLeadsACustomerWaiting(t *testing.T) {
	t.Parallel()

	got := aWholeDay(
		[]WaitingCustomer{{
			ActivityID: waitActivity,
			Subject:    "Still waiting on that quote",
			Since:      rankInstant.Add(-2 * time.Hour),
			PersonID:   waitPerson,
			// See the sibling above: an unproven wait is routine by design, and
			// the claim under test is that hygiene never outranks a customer we
			// are actually in conversation with.
			Engaged: true,
		}},
		crmcontracts.Attention{
			AsOf:             rankInstant,
			AutomationHealth: lane(item("automation", "automation_failed")),
			SyncHealth:       lane(item("sync", "sync_health")),
			CaptureHealth:    lane(item("capture", "capture_health")),
		},
	)

	// The customer leads, and the three system rows follow in whatever order the
	// tie-breaks give them — which is not this test's subject.
	if len(got) == 0 || got[0].Id != waitActivity.String() {
		t.Fatalf("the page led with %v over a customer waiting two hours",
			idsOf(got))
	}
}

// A wait that has gone STALE loses the top band, and this is where that rule
// meets the others: a fortnight-old silence must not sit above a meeting
// starting within the hour.
func TestAStaleWaitDoesNotOutrankAMeetingAboutToStart(t *testing.T) {
	t.Parallel()

	got := aWholeDay(
		[]WaitingCustomer{{
			ActivityID: waitActivity,
			Subject:    "Asked a month ago",
			Since:      rankInstant.Add(-time.Duration(waitingStaleDays+16) * 24 * time.Hour),
			PersonID:   waitPerson,
		}},
		crmcontracts.Attention{
			AsOf:     rankInstant,
			Meetings: lane(item("meeting", "meeting", withDue(rankInstant.Add(45*time.Minute)))),
		},
	)

	assertOrder(t, got, "meeting", waitActivity.String())
}

// And a wait with MONEY still on it keeps the top band, which is the exception
// the staleness rule carries: there the silence is the problem rather than a
// closed chapter. Without this case the rule above could be a blanket demotion.
//
// It still sits below the meeting, for the reason the first case gives — the
// meeting has a deadline and the wait has none. What this asserts is that the
// open deal lifted it back ABOVE the routine work a stale wait falls among.
func TestAStaleWaitWithAnOpenDealKeepsItsPlace(t *testing.T) {
	t.Parallel()

	got := aWholeDay(
		[]WaitingCustomer{{
			ActivityID:  waitActivity,
			Subject:     "Asked a month ago, deal still open",
			Since:       rankInstant.Add(-time.Duration(waitingStaleDays+16) * 24 * time.Hour),
			PersonID:    waitPerson,
			HasOpenDeal: true,
		}},
		crmcontracts.Attention{
			AsOf:     rankInstant,
			Meetings: lane(item("meeting", "meeting", withDue(rankInstant.Add(45*time.Minute)))),
			// The floor the stale wait would have fallen to without its deal.
			Notices: lane(item("notice", "notice")),
		},
	)

	assertOrder(t, got, "meeting", waitActivity.String(), "notice")
}

// Fixed ids, so a failure names the row rather than a fresh uuid.
var (
	waitActivity = ids.MustParse("00000000-0000-7000-8000-00000000a001")
	waitPerson   = ids.MustParse("00000000-0000-7000-8000-00000000b001")
)
