// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The anti-monopoly rule, asked of the lanes that never had one.
//
// It used to live inside two lane readers, so the other fifteen producers could
// each own the page unchecked: six overdue commitments, or six bounces, gave a
// reader a morning with one shape and nothing demoted them.
//
// The band is what the rule moves, so the band is what these read. `bounce` and
// `commitment` are chosen because both classify to a level that bands `now`
// uncrowded — the demotion is then visible as a band change rather than hidden
// under a level that was going to band that way anyway.

import (
	"context"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// laneOf builds n rows of one source, staggered so their order is decided
// rather than incidental.
func laneOf(source crmcontracts.AttentionItemSource, n int) []crmcontracts.AttentionItem {
	out := make([]crmcontracts.AttentionItem, 0, n)
	for i := range n {
		out = append(out, item(fmt.Sprintf("%s-%02d", source, i), source,
			withDue(rankInstant.Add(-time.Duration(n-i)*time.Hour))))
	}
	return out
}

// pageOfDay projects a day the way a reader receives it.
func pageOfDay(day crmcontracts.Attention) crmcontracts.Worklist {
	return (&Service{}).worklistFrom(
		context.Background(), day, scopeAll, "", 100, waitingRead{}, leadRead{}, worklistCursor{}, nil)
}

// bandsBySource counts how many rows of one source landed under each heading.
func bandsBySource(t *testing.T, out crmcontracts.Worklist, source crmcontracts.WorklistItemSource) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, row := range out.Queue {
		if row.Source != source {
			continue
		}
		if row.Band == nil {
			t.Fatalf("a %s row carries no band at all", source)
		}
		counts[string(*row.Band)]++
	}
	return counts
}

// A lane that never carried the rule. Past the lead the rest are demoted, which
// is what stops one producer owning the page.
func TestALaneThatNeverHadACapNowKeepsALead(t *testing.T) {
	const commitments = crowdLead + 4
	lane := laneOf("conversation_claim", commitments)

	out := pageOfDay(crmcontracts.Attention{AsOf: rankInstant, Commitments: &lane})

	if got := len(out.Queue); got != commitments {
		t.Fatalf("the page carried %d rows, want all %d — the rule reorders, it never drops",
			got, commitments)
	}
	bands := bandsBySource(t, out, "conversation_claim")
	if bands[bandNow] != crowdLead {
		t.Errorf("%d commitments led, want the %d the lead allows: %v", bands[bandNow], crowdLead, bands)
	}
	if bands[bandKeepMomentum] != commitments-crowdLead {
		t.Errorf("%d commitments were demoted, want the %d past the lead: %v",
			bands[bandKeepMomentum], commitments-crowdLead, bands)
	}
}

// A lane inside its lead is left alone entirely. Without this the case above
// would be satisfied by a rule that demoted everything.
func TestALaneInsideItsLeadIsNotCrowdedAtAll(t *testing.T) {
	lane := laneOf("conversation_claim", crowdLead)

	out := pageOfDay(crmcontracts.Attention{AsOf: rankInstant, Commitments: &lane})

	bands := bandsBySource(t, out, "conversation_claim")
	if bands[bandKeepMomentum] != 0 {
		t.Errorf("%d commitments were demoted while the lane was inside its lead: %v",
			bands[bandKeepMomentum], bands)
	}
}

// Two lanes past their leads are capped independently: the rule is per SOURCE,
// so one producer's backlog cannot spend another's allowance — and a rule that
// counted rows rather than sources would demote the second lane entirely.
func TestEachSourceKeepsItsOwnLead(t *testing.T) {
	claims := laneOf("conversation_claim", crowdLead+3)
	bounces := laneOf("bounce", crowdLead+3)

	out := pageOfDay(crmcontracts.Attention{
		AsOf: rankInstant, Commitments: &claims, Bounces: lane(bounces...),
	})

	for _, source := range []crmcontracts.WorklistItemSource{"conversation_claim", "bounce"} {
		bands := bandsBySource(t, out, source)
		if bands[bandNow] != crowdLead {
			t.Errorf("%s: %d led, want %d — each source leads with its own: %v",
				source, bands[bandNow], crowdLead, bands)
		}
		if bands[bandKeepMomentum] != 3 {
			t.Errorf("%s: %d demoted, want 3: %v", source, bands[bandKeepMomentum], bands)
		}
	}
}

// The reason the rule exists, as the page a reader gets: a second kind of work
// comes back within reach instead of sitting under the whole backlog.
func TestASecondKindOfWorkIsNotBuriedUnderALaneMonopoly(t *testing.T) {
	claims := laneOf("conversation_claim", crowdLead+6)
	dsr := laneOf("dsr", 1)

	out := pageOfDay(crmcontracts.Attention{AsOf: rankInstant, Commitments: &claims, Dsr: lane(dsr...)})

	position := -1
	for i, row := range out.Queue {
		if row.Source == "dsr" {
			position = i
		}
	}
	if position < 0 {
		t.Fatal("the subject request is not on the page at all")
	}
	// It leads outright — a legal deadline outranks a promise — but what this
	// pins is that the six crowded commitments are no longer above it, which is
	// what "below the whole backlog" meant.
	if position >= crowdLead {
		t.Errorf("the subject request sat at position %d, under the commitment backlog — the rule "+
			"exists so a reader meets their day rather than one lane of it", position+1)
	}
}

// A source whose own band draws BELOW keep_momentum is demoted, never promoted.
//
// Crowding used to answer keep_momentum for every crowded row, which was a
// demotion while the only crowded rows were waiting customers. Once the rule
// widened onto the source every row carries, the same answer moved a crowded
// decision UP: `review` draws last, so past the eighth pending decision the
// extra ones sorted above the first eight and the reader met the newest first.
// The eight that had waited longest — and, carrying their expiry as a
// deadline, expire soonest — went to the bottom of the queue.
func TestCrowdingNeverPromotesASourceThatBandsBelowTheDemotion(t *testing.T) {
	const staged = crowdLead + 4
	decisions := laneOf("approval", staged)

	out := pageOfDay(crmcontracts.Attention{AsOf: rankInstant, NeedsYou: decisions})

	bands := bandsBySource(t, out, "approval")
	if bands[bandReview] != staged {
		t.Fatalf("%d of %d staged decisions kept the review heading: crowding moved the rest "+
			"to a band that draws ABOVE their own: %v", bands[bandReview], staged, bands)
	}
}

// And the ordering that heading decides: the lead group still leads.
//
// The band sorts above crowding, so the promotion turned up as the reader's
// actual complaint — the oldest decisions last. This is that page, asserted as
// positions rather than as headings, because a band that was right and an order
// that was wrong would be the same defect wearing a fix's clothes.
func TestTheDecisionsThatHaveWaitedLongestStillLeadTheQueue(t *testing.T) {
	const staged = crowdLead + 4
	decisions := laneOf("approval", staged)

	out := pageOfDay(crmcontracts.Attention{AsOf: rankInstant, NeedsYou: decisions})

	order := make([]string, 0, staged)
	for _, row := range out.Queue {
		if row.Source == "approval" {
			order = append(order, row.Id)
		}
	}
	if len(order) != staged {
		t.Fatalf("the page carried %d of %d staged decisions", len(order), staged)
	}
	// laneOf staggers the lane so row 0 is the one that has waited longest.
	for i := range crowdLead {
		want := decisions[i].Id
		if order[i] != want {
			t.Errorf("position %d holds %s, and %s has waited longer — the reader meets "+
				"the newest decisions first and the oldest behind the rest",
				i+1, order[i], want)
		}
	}
}

// The rule itself, asked of every band the page draws: crowding moves a row
// DOWN or leaves it where it is, and never up.
//
// Derived from the draw order rather than from the two bands that showed the
// defect, so a band added below the demotion inherits the rule by existing —
// which is how this one arrived: `review` was appended to the order after
// crowding had widened off the single lane it was written for.
func TestCrowdingNeverMovesARowUpThePage(t *testing.T) {
	for _, band := range bandOrder {
		if got := crowdedBelow(band); bandRank(got) < bandRank(band) {
			t.Errorf("a crowded %q row is drawn under %q, which is further up the page — "+
				"crowding is the anti-monopoly rule, and promoting is the one thing it may not do",
				band, got)
		}
	}
}
