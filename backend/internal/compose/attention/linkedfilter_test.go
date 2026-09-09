// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two filter values a count links to.
//
// Both exist for one reason: a Brief surface counted a population this
// vocabulary could not then ask for, so its "N more" link opened a queue holding
// a different N. The rules below are what make the count and the door one
// answer, and each test states the wrong number it stops.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestTheChangedFilterReachesThePageThroughWorklist is the wiring test, and it
// is the one that matters most.
//
// keepFiltered can be right while the endpoint is wrong, because the flag it
// reads used to be stamped AFTER the narrowing ran — over the cut page, where a
// filter could never see it. Every existing test of that stamping calls
// markChangedSinceBrief directly, so moving the call left the whole package
// green: the unit tests above passed against a Worklist that answered the entire
// queue to `filter=changed_since_brief`.
//
// This drives the real entry point. Two bounces either side of the night's data
// cutoff, ASYMMETRIC in nothing but their moment, so the boundary is what
// decides the answer and not a count.
func TestTheChangedFilterReachesThePageThroughWorklist(t *testing.T) {
	t.Parallel()

	cutoff := readInstant.Add(-6 * time.Hour)
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{ran: true, asOf: cutoff},
		nil, nil, nil, nil, nil, nil, nil, nil, nil,
		&stubBounces{rows: []BouncedSend{
			{ID: ids.NewV7(), Subject: "after the night", BouncedAt: cutoff.Add(time.Hour)},
			{ID: ids.NewV7(), Subject: "before the night", BouncedAt: cutoff.Add(-time.Hour)},
		}},
		nil, nil, nil, fixedClock)

	// The control first: unfiltered, the page holds both. Without it a filter
	// that answered nothing at all would pass the assertion below.
	whole, err := svc.Worklist(pageReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("unfiltered worklist: %v", err)
	}
	if len(whole.Queue) != 2 {
		t.Fatalf("the unfiltered page holds %d rows, wanted both bounces — the "+
			"fixture, not the filter, is what this test would otherwise prove",
			len(whole.Queue))
	}

	narrowed, err := svc.Worklist(
		pageReader(), "", string(filterChangedSinceBrief), ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("narrowed worklist: %v", err)
	}
	if len(narrowed.Queue) != 1 {
		t.Fatalf("the narrowed page holds %d rows, wanted only the send that "+
			"bounced after the night read", len(narrowed.Queue))
	}
	if title := narrowed.Queue[0].Title; title == nil || *title != "after the night" {
		t.Fatalf("the page kept %v, wanted the row the night did not see", title)
	}
	// The echo, so a client can tell which narrowing it got back.
	if narrowed.Filter == nil || *narrowed.Filter != crmcontracts.WorklistFilter(filterChangedSinceBrief) {
		t.Fatalf("the page reported filter %v, wanted the one it was asked for",
			narrowed.Filter)
	}
}

// TestExceptDecisionsKeepsEveryKindButDecisions holds the feed footer's cut.
//
// The footer counts `waitingRows`, which drops the approvals the Decisions deck
// above already draws. Sent to a bare queue, a rep told "11 more" landed on a
// list of 14 — the three decisions they had just been shown as cards.
func TestExceptDecisionsKeepsEveryKindButDecisions(t *testing.T) {
	t.Parallel()

	day := crmcontracts.Attention{
		AsOf: rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{
			item("decision", "approval", withKind("send_email")),
		},
		AtRisk:  lane(item("deal", "deal_at_risk")),
		Planned: []crmcontracts.AttentionItem{item("task", "task")},
	}

	kept := keepFiltered(
		classifyDay(day, rankInstant, dayMoney{}), filterExceptDecisions)

	if ids := rankedIDs(kept); len(ids) != 2 {
		t.Fatalf("kept %v, wanted the deal and the task and not the decision", ids)
	}
	for _, row := range kept {
		if row.item.Category == categoryDecisions {
			t.Fatalf("kept a %q row: the complement of decisions holds none",
				row.item.Category)
		}
	}
}

// TestChangedSinceBriefKeepsOnlyWhatTheNightMissed holds the notice's cut.
//
// The overnight notice names rows the run did not see and used to link to the
// whole queue, so "3 things changed" opened a page of forty with no way to tell
// which three.
func TestChangedSinceBriefKeepsOnlyWhatTheNightMissed(t *testing.T) {
	t.Parallel()

	cutoff := rankInstant.Add(-time.Hour)
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		AtRisk: lane(
			item("fresh", "deal_at_risk", withOccurred(cutoff.Add(time.Minute))),
			item("seen", "deal_at_risk", withOccurred(cutoff.Add(-time.Minute))),
		),
	}

	rows := markChangedSinceBrief(classifyDay(day, rankInstant, dayMoney{}), cutoff)
	kept := keepFiltered(rows, filterChangedSinceBrief)

	if ids := rankedIDs(kept); len(ids) != 1 || ids[0] != "fresh" {
		t.Fatalf("kept %v, wanted only the row that moved after the cutoff", ids)
	}
}

// TestChangedSinceBriefKeepsNothingWithoutARun is the absent-is-not-false case.
//
// A zero cutoff leaves every flag unset, and an unset flag is "there was no
// night" rather than "the night saw this". Treating absent as a match would
// answer the whole queue to a reader asking what changed — reporting a morning
// with no overnight run as one where everything is new.
func TestChangedSinceBriefKeepsNothingWithoutARun(t *testing.T) {
	t.Parallel()

	day := crmcontracts.Attention{
		AsOf:   rankInstant,
		AtRisk: lane(item("r1", "deal_at_risk", withOccurred(rankInstant))),
	}

	rows := markChangedSinceBrief(
		classifyDay(day, rankInstant, dayMoney{}), time.Time{})
	if kept := keepFiltered(rows, filterChangedSinceBrief); len(kept) != 0 {
		t.Fatalf("kept %v with no run to compare against, wanted nothing",
			rankedIDs(kept))
	}
}

// TestChangedSinceBriefDropsDecisionsTheDeckAlreadyDrew keeps the two cuts
// agreeing with each other.
//
// The strip counting this population counts it over the same rows the feed is
// answerable for, so a decision it deliberately did not name must not appear
// behind its link either. Without this the door held one more row than the
// number that sent the reader — the original defect, one layer down.
func TestChangedSinceBriefDropsDecisionsTheDeckAlreadyDrew(t *testing.T) {
	t.Parallel()

	cutoff := rankInstant.Add(-time.Hour)
	fresh := cutoff.Add(time.Minute)
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{
			item("decision", "approval", withKind("send_email"), withOccurred(fresh)),
		},
		AtRisk: lane(item("deal", "deal_at_risk", withOccurred(fresh))),
	}

	rows := markChangedSinceBrief(classifyDay(day, rankInstant, dayMoney{}), cutoff)
	kept := keepFiltered(rows, filterChangedSinceBrief)

	if ids := rankedIDs(kept); len(ids) != 1 || ids[0] != "deal" {
		t.Fatalf("kept %v, wanted the deal alone — the decision is already a card",
			ids)
	}
}

// TestAnIntroductionRequestIsNotOneOfTheDecksCards holds the two sides on one
// spelling of the exclusion.
//
// The client drops `source === "approval"`; the obvious server reading is
// category `decisions`, and that is a WIDER set — an introduction request
// classifies as a decision and is not an approval. Reading the category dropped
// from the door a row the count had included, which is the original defect
// reappearing where nobody looks for it. Both versions pass every unit test on
// their own side, because each side is self-consistent.
func TestAnIntroductionRequestIsNotOneOfTheDecksCards(t *testing.T) {
	t.Parallel()

	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Introductions: lane(
			item("intro", "introduction_request", withDue(rankInstant.Add(time.Hour))),
		),
		NeedsYou: []crmcontracts.AttentionItem{
			item("decision", "approval", withKind("send_email")),
		},
	}
	rows := classifyDay(day, rankInstant, dayMoney{})

	// The premise: both rows land in the same category, so a category test
	// cannot tell them apart and this test would prove nothing without it.
	for _, row := range rows {
		if row.item.Category != categoryDecisions {
			t.Fatalf("row %q classifies as %q — the fixture no longer models two "+
				"rows one category cannot separate", row.item.Id, row.item.Category)
		}
	}

	kept := keepFiltered(rows, filterExceptDecisions)
	if ids := rankedIDs(kept); len(ids) != 1 || ids[0] != "intro" {
		t.Fatalf("kept %v, wanted the introduction alone: the deck draws "+
			"approvals as cards and an introduction is not one", ids)
	}
}

// TestAFoldedGroupReachesTheChangedPageThroughWorklist is the wiring half.
//
// groupChangedSinceBrief can be right while batches.go never calls it, and the
// unit test below cannot tell: the fold MINTS a row, so a missing assignment
// looks exactly like a group the night had seen. Reverting the assignment left
// the whole package green until this test existed.
//
// Three failures of ONE rule, which is what batchFloor takes to fold, all after
// the night's cutoff. The group must arrive on a page asking for what changed.
func TestAFoldedGroupReachesTheChangedPageThroughWorklist(t *testing.T) {
	t.Parallel()

	cutoff := readInstant.Add(-6 * time.Hour)
	fired := cutoff.Add(time.Hour)
	rule := ids.New[ids.AutomationKind]()
	runs := make([]TroubledAutomationRun, 0, batchFloor)
	for at := range batchFloor {
		runs = append(runs, TroubledAutomationRun{
			ID: ids.NewV7(), AutomationID: rule, Name: "Route new leads",
			Outcome: "failed", Reason: "the assignee seat is gone",
			// Spread, so the group's own sort moment is the OLDEST — the value
			// a freshness rule reading `occurred` would wrongly judge it by.
			OccurredAt: fired.Add(time.Duration(at) * time.Minute),
		})
	}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{ran: true, asOf: cutoff},
		nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		&stubAutomations{rows: runs},
		nil, nil, fixedClock)

	// The premise: unfiltered, these three really did fold into one row. Without
	// it the assertion below would pass over three unfolded rows and prove
	// nothing about a minted one.
	whole, err := svc.Worklist(pageReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("unfiltered worklist: %v", err)
	}
	if len(whole.Queue) != 1 || whole.Queue[0].Batch == nil {
		t.Fatalf("the unfiltered page holds %d rows and the first carries batch "+
			"%v — the fixture no longer folds, so this test would not be about a "+
			"minted row", len(whole.Queue), whole.Queue[0].Batch)
	}

	narrowed, err := svc.Worklist(
		pageReader(), "", string(filterChangedSinceBrief), ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("narrowed worklist: %v", err)
	}
	if len(narrowed.Queue) != 1 {
		t.Fatalf("the changed page holds %d rows, wanted the incident group: a "+
			"minted row inherits no flag, so the group was dropped from a notice "+
			"its members belonged in", len(narrowed.Queue))
	}
}

// TestAFoldedGroupCarriesItsMembersFreshness holds the minted row's flag.
//
// A fold MINTS a WorklistItem, so it inherits nothing its members carried unless
// it is told to. The freshness stamping now runs before the fold — it has to,
// because the filter reads it — so a group reached the changed strip with an
// absent flag and was silently dropped from a notice its members belonged in.
//
// Any member, not the oldest: the batch takes its sort moment from the oldest
// member, and judging freshness by that would report a failure that arrived this
// morning as old news.
func TestAFoldedGroupCarriesItsMembersFreshness(t *testing.T) {
	t.Parallel()

	old, fresh := false, true
	members := []ranked{
		{item: crmcontracts.WorklistItem{ChangedSinceBrief: &old}},
		{item: crmcontracts.WorklistItem{ChangedSinceBrief: &fresh}},
	}

	if answer := groupChangedSinceBrief(members); answer == nil || !*answer {
		t.Fatalf("a group holding one fresh member answered %v, wanted true — the "+
			"reader has not read it", answer)
	}
	if answer := groupChangedSinceBrief(members[:1]); answer == nil || *answer {
		t.Fatalf("a group whose every member is old answered %v, wanted false",
			answer)
	}
	// Absent stays absent: a night that never ran leaves every member unflagged,
	// and false would claim it had seen them.
	unflagged := []ranked{{item: crmcontracts.WorklistItem{}}}
	if answer := groupChangedSinceBrief(unflagged); answer != nil {
		t.Fatalf("a group of unflagged members answered %v, wanted no answer",
			*answer)
	}
}

// TestANamedCategoryStillFiltersByEquality is the control.
//
// keepFiltered took over from a function that only ever compared categories, so
// the seven ordinary lanes must still answer exactly what they did. A refusal
// test needs an admit case, and this is it: without it the three above would
// pass against a filter that kept nothing at all.
func TestANamedCategoryStillFiltersByEquality(t *testing.T) {
	t.Parallel()

	day := crmcontracts.Attention{
		AsOf:     rankInstant,
		NeedsYou: []crmcontracts.AttentionItem{item("d1", "approval", withKind("send_email"))},
		AtRisk:   lane(item("r1", "deal_at_risk")),
	}
	rows := classifyDay(day, rankInstant, dayMoney{})

	for _, want := range []struct {
		filter crmcontracts.WorklistFilter
		id     string
	}{
		{"deals_at_risk", "r1"},
		{crmcontracts.WorklistFilter(categoryDecisions), "d1"},
	} {
		kept := keepFiltered(rows, want.filter)
		if ids := rankedIDs(kept); len(ids) != 1 || ids[0] != want.id {
			t.Fatalf("filter %q kept %v, wanted just %q", want.filter, ids, want.id)
		}
	}
}

// TestEveryFoldableCategoryOpensItsOwnGroup derives the unfold set from the
// fold itself.
//
// foldableCategories is a second spelling of batchKeyOf's own branches, and a
// category that learns to fold without arriving there gets a Review verb that
// returns the group the reader pressed it on. That is not hypothetical: the
// frontend's reviewFilter used to send every group to `decisions`, so pressing
// Review on a broken automation filtered its own failures out of view, and this
// change reintroduced the same defect for `system` by keying the unfold on
// `decisions` alone.
//
// The census drives real rows through batchKeyOf rather than reading the map
// back to itself, so it fails in the direction that matters: a category that
// folds and is missing here.
func TestEveryFoldableCategoryOpensItsOwnGroup(t *testing.T) {
	t.Parallel()

	routine := levelRoutine
	dupe := ranked{item: crmcontracts.WorklistItem{
		Category: categoryDecisions, Level: routine, Source: "dedupe_candidate",
	}}
	cause := "one-broken-rule"
	incident := ranked{item: crmcontracts.WorklistItem{
		Category: categorySystem, Source: "automation_run", CauseRef: &cause,
	}}

	for _, row := range []ranked{dupe, incident} {
		if _, folds := batchKeyOf(row); !folds {
			t.Fatalf("a %q row did not fold — the fixture no longer models the "+
				"case this census is about", row.item.Category)
		}
		if !opensTheDeck(string(row.item.Category)) {
			t.Fatalf("category %q folds but a filter naming it does not open the "+
				"group: pressing Review on one of these rows returns the group "+
				"the reader pressed it on", row.item.Category)
		}
	}
}

// TestOnlyAFoldableCategoryOpensTheFoldedDeck holds which narrowings unfold.
//
// foldAndRepin draws a pile of alike routine decisions as one row and is skipped
// for a reader who asked to see inside it. The two link-only values are not that
// request: answering "what changed overnight" with a hundred rows the unfiltered
// page draws as one makes the door hold more than the count that sent the
// reader, which is the same disagreement in the other direction.
func TestOnlyAFoldableCategoryOpensTheFoldedDeck(t *testing.T) {
	t.Parallel()

	folded := map[string]bool{
		string(categoryDecisions):              true,
		string(categorySystem):                 true,
		string(filterExceptDecisions):          false,
		string(filterChangedSinceBrief):        false,
		string(crmcontracts.WorklistFilterAll): false,
		"deals_at_risk":                        false,
		"":                                     false,
	}
	for filter, want := range folded {
		if got := opensTheDeck(filter); got != want {
			t.Fatalf("opensTheDeck(%q) = %v, wanted %v", filter, got, want)
		}
	}
}

func rankedIDs(rows []ranked) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.item.Id)
	}
	return out
}
