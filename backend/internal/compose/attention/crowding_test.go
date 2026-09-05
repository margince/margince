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
