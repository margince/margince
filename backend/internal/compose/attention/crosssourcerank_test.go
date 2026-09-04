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
	return aWholeDayWithLeads(waits, nil, day)
}

// aWholeDayWithLeads is the same day plus the owed-leads lane.
//
// Leads are read BESIDE the assembled day, the way waits are — the lane takes
// the scope as a query argument — so classifyDay never produces one and a
// fixture built from it alone cannot order a lead against anything.
func aWholeDayWithLeads(
	waits []WaitingCustomer, leads []OwedLead, day crmcontracts.Attention,
) []crmcontracts.WorklistItem {
	rows := classifyDay(day, rankInstant, dayMoney{})
	for _, wait := range waits {
		rows = append(rows, classifyWaiting(wait, rankInstant))
	}
	for _, lead := range leads {
		rows = append(rows, classifyLead(lead, rankInstant))
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
			// TWO deals, and the second is what makes the first material. The
			// bar is the pipeline's own median and the test is `expected >
			// bar`, so a lane holding one deal has that deal AS the median and
			// it never clears itself — the row lands at the agreed level and
			// this fixture would order five kinds of work while claiming six.
			// The small one also proves the bar CUTS: it is a deal at risk that
			// does not interrupt the day, drawn below the material one and above
			// the hygiene.
			AtRisk: lane(
				item("risk", "deal_at_risk", withDeal(900_000_00)),
				item("small-risk", "deal_at_risk", withDeal(1_000_00)),
			),
			Notices: lane(item("notice", "notice")),
		},
	)

	assertOrder(t, got,
		"meeting",             // somebody else's clock, at a stated minute
		waitActivity.String(), // somebody else's clock, all day
		"promise",             // a promise the rep made
		"risk",                // revenue at risk, past the pipeline's median
		"small-risk",          // a deal below that bar: agreed work, not urgent
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
	leadOwed     = ids.MustParse("00000000-0000-7000-8000-00000000c001")
)

// The whole fixture the campaign's plan names: six kinds at once.
//
// The test above orders five, and two of the six it does not reach are the two
// a rep would most likely dispute. A LEAD whose first-reply target has already
// been missed sits at the waiting level beside a customer, and is read from a
// lane BESIDE the assembled day — so a fixture built from classifyDay alone
// could not have ordered one at all. A routine DECISION is the hygiene the
// queue exists to keep out of the way, and `notice` stood in for it: system
// news rather than a judgement somebody owes.
//
// What it holds to: every one of the four seller obligations leads the two
// housekeeping rows, and the two clocks that have already run out lead the
// promise that has not.
func TestSixKindsOfWorkAreOrderedAgainstEachOther(t *testing.T) {
	t.Parallel()

	soon := rankInstant.Add(90 * time.Minute)
	got := aWholeDayWithLeads(
		[]WaitingCustomer{{
			ActivityID: waitActivity,
			Subject:    "Can you confirm the retrofit price?",
			Since:      rankInstant.Add(-72 * time.Hour),
			PersonID:   waitPerson,
			Engaged:    true,
		}},
		[]OwedLead{{
			ID:   leadOwed,
			Name: "Kirsten at LOXXESS",
			// Already missed, which is why it ranks with the customer rather
			// than with the promise: the clock did not merely start, it ran out.
			DeadlineAt: rankInstant.Add(-2 * time.Hour),
			State:      "breached",
		}},
		crmcontracts.Attention{
			AsOf:        rankInstant,
			Commitments: lane(item("promise", "conversation_claim", withDue(soon))),
			Meetings:    lane(item("meeting", "meeting", withDue(soon))),
			// TWO deals, and the second is what makes the first material. The bar
			// is the pipeline's own median and the test is `expected > bar`, so a
			// lane holding ONE deal has that deal as the median and it never clears
			// itself — the row lands at the agreed level, and this fixture would
			// order five kinds of work while its name claims six.
			AtRisk: lane(
				item("risk", "deal_at_risk", withDeal(900_000_00)),
				item("small-risk", "deal_at_risk", withDeal(1_000_00)),
			),
			NeedsYou: []crmcontracts.AttentionItem{
				item("pair", "dedupe_candidate"),
			},
		},
	)

	// The lead LEADS, and that surprised this test before it taught it. The
	// lead, the meeting and the wait all sit at the waiting level, so the
	// deadline step decides between them — and the lead's target ran out two
	// hours ago while the meeting's is ninety minutes off. An overdue clock
	// precedes a running one, which is the same rule that puts the meeting
	// above the wait that carries no deadline at all.
	assertOrder(t, got,
		leadOwed.String(),     // a first reply already late
		"meeting",             // a clock at a stated minute, still to come
		waitActivity.String(), // a customer waiting, with no stated minute
		"promise",             // a promise the rep made
		"risk",                // revenue at risk, past the pipeline's median
		"small-risk",          // a deal below that bar: agreed work, not urgent
		"pair",                // a judgement that blocks nobody
	)
}
