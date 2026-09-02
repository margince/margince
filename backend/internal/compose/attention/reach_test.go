// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the queue says about how much of each source it looked at. A page over a
// bounded read is two different cuts, and these hold that neither is ever
// reported as the other.

import (
	"context"
	"strconv"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A bound is not a total. A lane read to its limit knows only that more may
// exist, so it says so rather than reporting the number it happened to read.
func TestABoundedSourceSaysMoreRatherThanATotal(t *testing.T) {
	notices := []crmcontracts.AttentionItem{}
	for i := 0; i < doneCap; i++ {
		notices = append(notices, item("n"+string(rune('a'+i)), "notice"))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Notices: &notices}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, leadRead{}, worklistCursor{})

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

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, leadRead{}, worklistCursor{})

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

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, leadRead{}, worklistCursor{})

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

	first := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, leadRead{}, worklistCursor{})
	second := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, leadRead{}, worklistCursor{})

	if len(first.Reach) != len(second.Reach) {
		t.Fatalf("two reads gave %d and %d sources", len(first.Reach), len(second.Reach))
	}
	for i := range first.Reach {
		if first.Reach[i].Source != second.Reach[i].Source {
			t.Fatalf("source order differs at %d: %s then %s", i, first.Reach[i].Source, second.Reach[i].Source)
		}
	}
}

// A count of rows the reader cannot see is the same leak as showing them: it
// says a colleague's deal exists. Reach is taken AFTER the scope narrowing, and
// this holds that order — counting first would report the colleague's row in a
// number without ever drawing it.
func TestReachUnderMineCountsNothingOfAColleaguesWork(t *testing.T) {
	reader := ids.MustParse("01a05500-0000-7000-8000-000000000001")
	colleague := ids.MustParse("01a05500-0000-7000-8000-0000000000ff")
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: reader,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	theirID := openapi_types.UUID(colleague)
	theirs := item("theirs", "deal_at_risk")
	theirs.Deal = &crmcontracts.AttentionDealFacts{OwnerId: &theirID}
	atRisk := []crmcontracts.AttentionItem{theirs}
	day := crmcontracts.Attention{AsOf: rankInstant, AtRisk: &atRisk}

	got := (&Service{}).worklistFrom(ctx, day, "mine", "", 50, waitingRead{}, leadRead{}, worklistCursor{})

	for _, reach := range got.Reach {
		if reach.Source == "deal_at_risk" && reach.Considered > 0 {
			t.Fatalf("reach counted %d of a colleague's deals under `mine`", reach.Considered)
		}
	}
}

// Under-reporting is the one way these figures must not fail. A lane that came
// back exactly full says so, whatever its own bound happens to be — a source
// silently marked complete tells a rep there is no more work of that kind, and
// nothing fails to say otherwise.
func TestEveryBoundedLaneReportsItsTruncation(t *testing.T) {
	full := func(n int, source crmcontracts.AttentionItemSource) []crmcontracts.AttentionItem {
		rows := make([]crmcontracts.AttentionItem, 0, n)
		for i := 0; i < n; i++ {
			rows = append(rows, item(string(source)+strconv.Itoa(i), source))
		}
		return rows
	}
	planned := full(plannedCap, "task")
	atRisk := full(quietDealBound, "deal_at_risk")
	decay := full(decayBound, "relationship_decay")
	day := crmcontracts.Attention{
		AsOf: rankInstant, Planned: planned, AtRisk: &atRisk, RelationshipDecay: &decay,
	}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 500, waitingRead{}, leadRead{}, worklistCursor{})

	marked := map[crmcontracts.WorklistReachSource]bool{}
	for _, reach := range got.Reach {
		marked[reach.Source] = reach.MoreAvailable
	}
	for _, source := range []crmcontracts.WorklistReachSource{"task", "deal_at_risk", "relationship_decay"} {
		if !marked[source] {
			t.Errorf("%s filled its lane and still reports itself complete", source)
		}
	}
}

// A source read and found empty is not a source that was never read. One says
// "nothing today", the other says nothing at all, and a reader cannot tell the
// two apart from an absence.
func TestASourceReadAndFoundEmptyStillAppears(t *testing.T) {
	none := []crmcontracts.AttentionItem{}
	day := crmcontracts.Attention{AsOf: rankInstant, Notices: &none}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "", 50, waitingRead{}, leadRead{}, worklistCursor{})

	for _, reach := range got.Reach {
		if reach.Source == "notice" {
			if reach.Considered != 0 || reach.MoreAvailable {
				t.Fatalf("an empty notice lane reports considered=%d more=%v", reach.Considered, reach.MoreAvailable)
			}
			return
		}
	}
	t.Fatal("a lane that was read and found empty is missing from reach — it reads as never asked")
}

// Narrowing to one category hides the other sources' ROWS, not the fact that
// they had any. A rep filtering to meetings should read "tasks, not shown"
// rather than "no tasks", and a filtered-out source that hit its bound must not
// take its truncation signal out of the payload with it.
func TestNarrowingKeepsTheOtherSourcesInReach(t *testing.T) {
	planned := []crmcontracts.AttentionItem{item("t1", "task")}
	notices := []crmcontracts.AttentionItem{item("n1", "notice")}
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: planned, Notices: &notices}

	got := (&Service{}).worklistFrom(t.Context(), day, "all", "system", 50, waitingRead{}, leadRead{}, worklistCursor{})

	for _, reach := range got.Reach {
		if reach.Source != "task" {
			continue
		}
		if reach.Considered != 1 {
			t.Fatalf("the filtered-out task source considered %d, wanted the one it read", reach.Considered)
		}
		if reach.Shown != 0 {
			t.Fatalf("a filtered-out source reports %d shown", reach.Shown)
		}
		return
	}
	t.Fatal("narrowing erased the task source from reach — it reads as no tasks at all")
}
