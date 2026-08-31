// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the queue says about how much of each source it looked at. A page over a
// bounded read is two different cuts, and these hold that neither is ever
// reported as the other.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A bound is not a total. A lane read to its limit knows only that more may
// exist, so it says so rather than reporting the number it happened to read.
func TestABoundedSourceSaysMoreRatherThanATotal(t *testing.T) {
	notices := []crmcontracts.AttentionItem{}
	for i := 0; i < doneCap; i++ {
		notices = append(notices, item("n"+string(rune('a'+i)), "notice"))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Notices: &notices}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, nil)

	for _, reach := range got.Reach {
		if reach.Source != "notice" {
			continue
		}
		if !reach.MoreAvailable {
			t.Fatal("a lane read to its bound reports no more available — the number reads as a total")
		}
		if reach.Considered != doneCap {
			t.Fatalf("considered = %d, want the %d this read actually weighed", reach.Considered, doneCap)
		}
		return
	}
	t.Fatal("the notice source is absent from reach")
}

// A source under its bound is complete, and saying "more available" about it
// would send a reader looking for work that does not exist.
func TestASourceUnderItsBoundClaimsNoMore(t *testing.T) {
	notices := []crmcontracts.AttentionItem{item("only", "notice")}
	day := crmcontracts.Attention{AsOf: rankInstant, Notices: &notices}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, nil)

	for _, reach := range got.Reach {
		if reach.Source == "notice" && reach.MoreAvailable {
			t.Fatal("a complete source claims more work exists")
		}
	}
}

// A folded group stands for its members. Counting it as one row of source
// `batch` would report every folded source as shown zero, which a reader reads
// as "nothing from this source" rather than "folded into one row".
func TestAFoldedGroupIsCountedAgainstTheSourcesItStandsFor(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 6; i++ {
		row := item("f"+string(rune('a'+i)), "automation_run")
		cause := "automation_run:one-rule"
		row.CauseRef = &cause
		failures = append(failures, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AutomationHealth: &failures}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, nil)

	for _, reach := range got.Reach {
		if reach.Source != "automation_run" {
			continue
		}
		if reach.Considered != 6 {
			t.Fatalf("considered = %d, want the six failures read", reach.Considered)
		}
		if reach.Shown != 6 {
			t.Fatalf("shown = %d, want the six the folded row stands for", reach.Shown)
		}
		return
	}
	t.Fatal("the folded source is absent from reach — it reads as nothing from that source")
}

// Two reads of one unchanged day produce the same bytes. Map order is not an
// order, and a client diffing the payload would see a change that is not one.
func TestReachIsOrderedTheSameWayTwice(t *testing.T) {
	notices := []crmcontracts.AttentionItem{item("n", "notice")}
	bounces := []crmcontracts.AttentionItem{item("b", "bounce")}
	day := crmcontracts.Attention{AsOf: rankInstant, Notices: &notices, Bounces: &bounces}

	first := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, nil)
	second := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, nil)

	if len(first.Reach) != len(second.Reach) {
		t.Fatalf("two reads gave %d and %d sources", len(first.Reach), len(second.Reach))
	}
	for i := range first.Reach {
		if first.Reach[i].Source != second.Reach[i].Source {
			t.Fatalf("source order differs at %d: %s then %s", i, first.Reach[i].Source, second.Reach[i].Source)
		}
	}
}
