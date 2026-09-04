// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// When many broken things are ONE broken thing.
//
// Eight rows saying a recap did not generate are one incident, and the real
// workspace held 163 failures of a single AI task — repeating it 163 times is
// aggregation failure wearing urgency's clothes. These are the cases that draw
// the line: what folds, what refuses to, and what must never be folded away
// because a customer is on the other end of it.

import (
	"fmt"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Eight rows saying a recap did not generate are ONE thing that is broken.
// Repeating it eight times is aggregation failure rather than urgency — the
// concept's words, and the real workspace holds 163 failures of one AI task.
func TestAlikeSystemFailuresAreOneIncident(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 8; i++ {
		failures = append(failures, aiFailure(i, "site_triage"))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 1 {
		t.Fatalf("eight failures of one task drew %d rows", len(got))
	}
	if got[0].Batch == nil || got[0].Batch.Count != 8 {
		t.Fatal("the incident does not say how many times it happened")
	}
	if got[0].Batch.Cause == nil || *got[0].Batch.Cause != "ai_work_health:site_triage" {
		t.Fatal("the incident does not name WHAT is broken")
	}
}

// Two causes are two incidents. Grouping by source alone would tell a reader
// two things are broken and name neither.
func TestTwoBrokenThingsAreTwoIncidents(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		for _, task := range []string{"site_triage", "signal_extract"} {
			failures = append(failures, aiFailure(i*2+len(task)%2, task))
		}
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 2 {
		t.Fatalf("two broken tasks drew %d rows", len(got))
	}
	causes := map[string]bool{}
	for _, row := range got {
		if row.Batch != nil && row.Batch.Cause != nil {
			causes[*row.Batch.Cause] = true
		}
	}
	if !causes["ai_work_health:site_triage"] || !causes["ai_work_health:signal_extract"] {
		t.Fatalf("the incidents name %v, wanted both broken tasks", causes)
	}
}

// An incident is not hygiene: while something is broken, every quiet claim on
// the page is suspect, so it keeps its own band rather than being filed with
// the routine tidying.
func TestAnIncidentIsNotFiledAsRoutineTidying(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		failures = append(failures, item("m"+string(rune('a'+i)), "capture_health", withKind("reauth_required")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if got[0].Category != "system" {
		t.Fatalf("the incident is filed under %q, wanted system", got[0].Category)
	}
	if got[0].Consequence == "data_drifts" {
		t.Fatal("a broken mailbox says the records drift, which is not what it costs")
	}
}

// A bounced email is a customer CONSEQUENCE, not a system condition. Three
// bounces are three customers who did not get their message, and folding them
// by the provider's reason would hide two of them.
func TestBouncesAreNeverFoldedIntoAnIncident(t *testing.T) {
	bounces := []crmcontracts.AttentionItem{}
	for i := 0; i < 4; i++ {
		bounces = append(bounces, item("b"+string(rune('a'+i)), "bounce", withKind("hard")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Bounces: &bounces}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 4 {
		t.Fatalf("four bounced customers drew %d rows — some were hidden", len(got))
	}
}

// WHICH field names the cause depends on the producer. An AI run's title is
// its own summary, written per run, so grouping on it would draw a hundred and
// sixty-three incidents for one broken task — the workspace's real number.
func TestAIFailuresGroupByWhatRanNotByEachRunsOwnWords(t *testing.T) {
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		row := aiFailure(i, "site_triage")
		summary := "reading acme" + string(rune('a'+i)) + ".com failed"
		row.Title = &summary
		failures = append(failures, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AiWorkHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 1 {
		t.Fatalf("five failures of one task drew %d incidents", len(got))
	}
	if got[0].Batch.Cause == nil || *got[0].Batch.Cause != "ai_work_health:site_triage" {
		t.Fatalf("the incident names %v, wanted the task that broke", got[0].Batch.Cause)
	}
}

// Two broken mailboxes are two things to reconnect. A heading that says
// "disconnected" once sends the reader to fix one and silently loses the other.
func TestTwoBrokenMailboxesAreTwoIncidents(t *testing.T) {
	rows := []crmcontracts.AttentionItem{}
	for _, mailbox := range []string{"sales@acme.test", "lena@acme.test"} {
		for i := 0; i < 4; i++ {
			row := item(mailbox+string(rune('a'+i)), "capture_health", withKind("disconnected"))
			cause := "capture_health:disconnected:" + mailbox
			row.CauseRef = &cause
			rows = append(rows, row)
		}
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &rows}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 2 {
		t.Fatalf("two broken mailboxes drew %d rows, wanted one incident each", len(got))
	}
}

// Capture and overlay sync both name a condition `sync_failing`. Grouping on
// the condition word alone merges two unrelated failures under one heading
// that names neither.
func TestTwoSourcesSharingAConditionWordAreNotOneIncident(t *testing.T) {
	capture := []crmcontracts.AttentionItem{}
	for i := 0; i < 3; i++ {
		row := item("c"+string(rune('a'+i)), "capture_health", withKind("sync_failing"))
		cause := "capture_health:sync_failing:sales@acme.test"
		row.CauseRef = &cause
		capture = append(capture, row)
	}
	ai := []crmcontracts.AttentionItem{}
	for i := 0; i < 3; i++ {
		row := item("a"+string(rune('a'+i)), "ai_work_health", withKind("sync_failing"))
		cause := "ai_work_health:sync_failing"
		row.CauseRef = &cause
		ai = append(ai, row)
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &capture, AiWorkHealth: &ai}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 2 {
		t.Fatalf("two sources sharing a condition word drew %d rows, wanted one each", len(got))
	}
}

// A row that names no condition never groups. An ungrouped row is one row too
// many; a wrongly grouped one hides a failure the reader never learns about.
func TestASystemRowWithNoNamedConditionNeverGroups(t *testing.T) {
	rows := []crmcontracts.AttentionItem{}
	for i := 0; i < 5; i++ {
		rows = append(rows, item("s"+string(rune('a'+i)), "capture_health", withKind("disconnected")))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, CaptureHealth: &rows}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 5 {
		t.Fatalf("rows naming no condition folded into %d — a failure was hidden", len(got))
	}
}

// aiFailure builds a troubled-AI row through the PRODUCTION renderer, so a test
// about grouping is a test about how the product derives the grouping key. A
// test that hand-set the marker would pass while the renderer set nothing.
func aiFailure(seq int, taskKind string) crmcontracts.AttentionItem {
	return aiWorkItem(TroubledRun{
		ID:         ids.MustParse(fmt.Sprintf("01a05500-0000-7000-8000-0000000%05x", seq)),
		Kind:       taskKind,
		State:      "failed",
		OccurredAt: rankInstant,
	})
}

// The words the group is drawn from, through the whole path a row travels.
//
// The renderer mints the label, base() forwards it, and batchRow lifts it onto
// the group. Every step is a place it can be dropped or replaced, and the
// per-renderer test cannot see any of them: it stops at the item. A gate that
// judged only the renderer stayed green while batchRow assigned the identity.
//
// The automation lane, because it is the one that HAS a name to mint — the
// rule's own — and it is the case the whole split exists for: the identity must
// be the immutable id, so without the label the group can only be drawn from
// "automation_run:<uuid>".
func TestAnIncidentGroupIsDrawnFromTheRulesNameNotItsIdentity(t *testing.T) {
	automationID := ids.New[ids.AutomationKind]()
	failures := []crmcontracts.AttentionItem{}
	for i := 0; i < 6; i++ {
		failures = append(failures, automationItem(TroubledAutomationRun{
			ID:           ids.MustParse(fmt.Sprintf("01a05500-0000-7000-8000-0000000%05x", i)),
			AutomationID: automationID,
			Name:         "Notify sales on a new lead",
			Outcome:      "failed",
			Reason:       "The webhook target answered 500.",
			OccurredAt:   rankInstant,
		}))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, AutomationHealth: &failures}

	got := rankAll(foldRoutineDecisions(classifyDay(day, rankInstant, dayMoney{})))

	if len(got) != 1 || got[0].Batch == nil {
		t.Fatalf("six failures of one rule drew %d rows, want one group", len(got))
	}
	// The identity still groups them — that is what it is for, and asserting it
	// here keeps the label from being "fixed" by making the cause readable.
	if got[0].Batch.Cause == nil || *got[0].Batch.Cause != "automation_run:"+automationID.String() {
		t.Fatalf("the group's identity is %v, want the rule's id", got[0].Batch.Cause)
	}
	if got[0].Batch.Label == nil {
		t.Fatal("the group carries no words, so a client can only draw it from the identity — " +
			"which is a uuid in front of a rep")
	}
	if *got[0].Batch.Label != "Notify sales on a new lead" {
		t.Fatalf("the group is named %q, want the rule's own name", *got[0].Batch.Label)
	}
}

// A group belongs to ONE screen, and rows that would sit on different ones do
// not fold however alike they read.
//
// The system CATEGORY spans destinations — a stopped mailbox is an
// administrator's job, a notice is a judgement, a bounce is a seller's
// customer — so grouping by a shared condition can reach across them. Folding
// there would take a row off the screen that counts it and hide it under a
// heading that means something else, with nothing left on the page to notice.
// The members go back as themselves instead: more rows, nothing lost.
func TestRowsBoundForDifferentScreensDoNotFold(t *testing.T) {
	mixed := []ranked{
		systemRowWithCause("notice", "one"),
		systemRowWithCause("capture_health", "two"),
		systemRowWithCause("capture_health", "three"),
	}
	// The fixture has to be foldable but for the destination, or this test
	// passes on a floor it never reached.
	if len(mixed) < batchFloor {
		t.Fatalf("the fixture holds %d rows, below the fold floor of %d", len(mixed), batchFloor)
	}
	alike := []ranked{
		systemRowWithCause("capture_health", "one"),
		systemRowWithCause("capture_health", "two"),
		systemRowWithCause("capture_health", "three"),
	}
	if folded := foldRoutineDecisions(alike); len(folded) != 1 {
		t.Fatalf("three rows of one screen drew %d rows, so this fixture cannot show a refusal", len(folded))
	}

	got := foldRoutineDecisions(mixed)

	if len(got) != len(mixed) {
		t.Fatalf("rows bound for different screens folded into %d rows: a judgement and a broken "+
			"mailbox were counted as one thing", len(got))
	}
	for _, row := range got {
		if row.item.Batch != nil {
			t.Error("a mixed group produced a batch row")
		}
	}
}

// systemRowWithCause is one system row naming a shared condition, built through
// the classifier production uses so the fold sees the shape it really gets.
//
// ONE condition for every caller, because that is what makes the rows foldable:
// the fold groups on the cause, so a test about what happens WITHIN a group
// needs its rows to reach one.
func systemRowWithCause(source crmcontracts.AttentionItemSource, id string) ranked {
	cause := "sync_failing"
	at := item(fmt.Sprintf("%s-%s", source, id), source)
	at.CauseRef = &cause
	return classifySystem(at, rankInstant)
}

// A folded group's screen comes from its MEMBERS, not from the guard that
// happens to run before it.
//
// foldRoutineDecisions refuses to group rows bound for different screens, so in
// production batchRow only ever sees members that agree. That makes the claim
// true and unheld at the same time: the guard is in another function, and a
// second caller reaching batchRow without it would file a group wherever its
// first member happened to sit. This calls batchRow directly, which is the
// shape that guard cannot protect.
func TestAGroupTakesItsScreenFromItsMembers(t *testing.T) {
	members := []ranked{
		systemRowWithCause("capture_health", "one"),
		systemRowWithCause("capture_health", "two"),
		systemRowWithCause("capture_health", "three"),
	}
	if at := destinationOf(members[0]); at != destinationSystemHealth {
		t.Fatalf("the fixture's members belong to %q, so this test cannot show a group "+
			"taking system_health from them", at)
	}

	group := batchRow(keySystemIncident, "sync_failing", members, false)

	if group.item.Destination == nil {
		t.Fatal("a folded group says nothing about which screen it belongs on")
	}
	if *group.item.Destination != destinationSystemHealth {
		t.Errorf("a group of broken mailboxes belongs to %q, want %q — its members' screen",
			*group.item.Destination, destinationSystemHealth)
	}
	// And members that disagree are filed for review rather than taking the
	// first one's answer. Driven through batchRow, not through the helper it
	// calls: the helper on its own proves nothing about what the group row
	// actually carries, and a batchRow that went back to reading members[0]
	// passed a version of this test that asked destinationOfGroup directly.
	mixed := append([]ranked{systemRowWithCause("notice", "four")}, members...)
	disagreeing := batchRow(keySystemIncident, "sync_failing", mixed, false)
	if disagreeing.item.Destination == nil {
		t.Fatal("a group of disagreeing members says nothing about its screen")
	}
	if *disagreeing.item.Destination != destinationReview {
		t.Errorf("a group whose members disagree answered %q, want %q rather than the "+
			"first member's screen", *disagreeing.item.Destination, destinationReview)
	}
}
