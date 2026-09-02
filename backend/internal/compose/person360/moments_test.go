// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

import (
	"context"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// now is a fixed instant, so every case below reads as a claim about the data
// rather than as arithmetic against the wall clock.
var now = time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

func at(daysAgo int) time.Time { return now.AddDate(0, 0, -daysAgo) }

func ahead(hours int) time.Time { return now.Add(time.Duration(hours) * time.Hour) }

// timelineOf builds the timeline section the rules read.
func timelineOf(rows ...crmcontracts.Activity) *struct {
	Data []crmcontracts.Activity `json:"data"`
	Page crmcontracts.PageInfo   `json:"page"`
} {
	return &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: rows}
}

// meeting builds the next-meeting section at a given distance from now.
func meeting(startsAt time.Time) *crmcontracts.Person360NextMeeting {
	return &crmcontracts.Person360NextMeeting{
		ActivityId: openapi_types.UUID{},
		StartsAt:   startsAt,
		Subject:    ptr("Expansion review"),
	}
}

// The page opens on ONE moment. A reader handed five reasons has been handed
// the choosing back, which is the work the ladder exists to do.
func TestTheLadderSelectsExactlyOneMoment(t *testing.T) {
	// Three rungs could fire at once: a meeting is close, they replied after a
	// long gap, and a promise is overdue.
	replied := at(1)
	page := &crmcontracts.Person360{
		NextMeeting:    meeting(ahead(24)),
		LastInboundAt:  &replied,
		LastOutboundAt: ptr(at(40)),
		Claims: &[]crmcontracts.ConversationClaim{{
			Kind:             crmcontracts.CommitmentOurs,
			Status:           crmcontracts.ConversationClaimStatusOpen,
			Body:             "Send the revised ROI model",
			DueAt:            ptr(at(5)),
			SourceActivityId: openapi_types.UUID{},
			SourceQuote:      "I'll get you the model by Friday",
		}},
	}
	got := deriveMoment(readerCtx(), now, page)
	if got.Rule != crmcontracts.PersonMomentRuleMeetingPrep {
		t.Fatalf("meeting prep outranks every rung below it, got %q", got.Rule)
	}
}

// A meeting outside the horizon is a diary entry, not a reason to open the
// page. The rung below it must then win.
func TestAMeetingBeyondSeventyTwoHoursDoesNotWin(t *testing.T) {
	replied := at(1)
	page := &crmcontracts.Person360{
		NextMeeting:    meeting(ahead(96)),
		LastInboundAt:  &replied,
		LastOutboundAt: ptr(at(40)),
	}
	got := deriveMoment(readerCtx(), now, page)
	if got.Rule != crmcontracts.PersonMomentRuleReEngaged {
		t.Fatalf("a meeting four days out should not beat a fresh reply, got %q", got.Rule)
	}
}

// An answered conversation owes nothing. The gone-quiet rung reads direction
// rather than "last touch", because last touch cannot tell the two apart.
func TestAnAnsweredConversationIsNotGoneQuiet(t *testing.T) {
	page := &crmcontracts.Person360{
		LastOutboundAt: ptr(at(30)),
		LastInboundAt:  ptr(at(2)),
		Activities:     timelineOf(),
		Network: &struct {
			Colleagues []crmcontracts.PersonNetworkColleague `json:"colleagues"`
		}{},
	}
	got := deriveMoment(readerCtx(), now, page)
	if got.Rule == crmcontracts.PersonMomentRuleGoneQuiet {
		t.Fatal("they answered after our last message; silence is not the story")
	}
}

// Our unanswered outbound past the configured rule is the gone-quiet case, and
// the moment must name the rule that produced it — a reader who disagrees with
// a verdict has to be able to see what produced it.
func TestGoneQuietNamesTheRuleItFiredOn(t *testing.T) {
	page := &crmcontracts.Person360{
		LastOutboundAt: ptr(at(9)),
		LastInboundAt:  ptr(at(16)),
	}
	got := deriveMoment(readerCtx(), now, page)
	if got.Rule != crmcontracts.PersonMomentRuleGoneQuiet {
		t.Fatalf("nine days unanswered past a seven-day rule is gone quiet, got %q", got.Rule)
	}
	if got.WhyNow == "" || got.RecommendedAction.Destination == nil {
		t.Fatal("the moment must state its rule and name where its action goes")
	}
	if got.RecommendedAction.Destination.Surface != crmcontracts.PersonMomentDestinationSurfaceComposer {
		t.Fatalf("drafting a follow-up opens the composer, got %q", got.RecommendedAction.Destination.Surface)
	}
}

// An action a rep can press has to go somewhere. The "Ask for context" button
// shipped enabled with no destination, so it rendered as a live control and did
// nothing at all — which teaches a reader that the page has dead buttons, a
// worse outcome than the action being visibly unavailable.
//
// The rule is general: any action offered as available names a destination it
// can actually reach, and any action that cannot reach one says so with a
// reason. The frontend disables a blocked action and shows the reason; what was
// missing was the backend saying it.
//
// It walks the LADDER rather than a handful of pages, and that is the point. A
// first version drove deriveMoment with two shapes, reached two of eight rungs,
// and passed while three dead buttons sat on rungs it never ran. A rung that
// only the winning page reaches is a rung no test covers.
func TestEveryOfferedActionEitherGoesSomewhereOrSaysWhyItCannot(t *testing.T) {
	// One page per rung, each built to make that rung fire.
	pages := map[string]*crmcontracts.Person360{
		"meeting prep": {NextMeeting: &crmcontracts.Person360NextMeeting{StartsAt: ahead(24)}},
		"gone quiet":   {LastOutboundAt: ptr(at(9)), LastInboundAt: ptr(at(16))},
		"thin relationship": {
			Activities: timelineOf(),
			Network: &struct {
				Colleagues []crmcontracts.PersonNetworkColleague `json:"colleagues"`
			}{},
		},
		"role change": {
			RelationshipChanges: &[]crmcontracts.PersonRelationshipChange{{
				Kind: crmcontracts.PersonRelationshipChangeKind(relstrength.ChangeRepliedAfterGap),
				At:   at(2),
			}},
			Commercial: &crmcontracts.Person360Commercial{Deal: &crmcontracts.Person360CommercialDeal{
				DealId: openapi_types.UUID(ids.NewV7()), Title: "Dispatch integration",
			}},
		},
		// The same rung with NO deal visible, which is the reader who lacks the
		// deal grant: the action must block rather than point at a record they
		// cannot open.
		"role change, no deal": {
			RelationshipChanges: &[]crmcontracts.PersonRelationshipChange{{
				Kind: crmcontracts.PersonRelationshipChangeKind(relstrength.ChangeRepliedAfterGap),
				At:   at(2),
			}},
		},
		"missing next step": {
			Commercial: &crmcontracts.Person360Commercial{Deal: &crmcontracts.Person360CommercialDeal{
				DealId: openapi_types.UUID(ids.NewV7()), Title: "Dispatch integration",
			}},
		},
		// Rung 2 wants inbound that arrived after a long silence of ours.
		"re-engaged": {
			LastOutboundAt: ptr(at(40)),
			LastInboundAt:  ptr(at(3)),
		},
		// Rung 4 wants an open commitment OF OURS whose date has passed.
		"overdue promise": {
			Claims: &[]crmcontracts.ConversationClaim{{
				Kind:             crmcontracts.CommitmentOurs,
				Status:           crmcontracts.ConversationClaimStatusOpen,
				Body:             "Send the revised dispatch quote",
				SourceQuote:      "Ich schicke dir das Angebot bis Freitag.",
				SourceActivityId: openapi_types.UUID(ids.NewV7()),
				DueAt:            ptr(at(6)),
			}},
		},
		// Rung 5b wants an open task filed against them, and it must fire with
		// no date on it — that is what the transcript reader files.
		"open promise": {
			NextSteps: &struct {
				Data []crmcontracts.Activity `json:"data"`
				Page crmcontracts.PageInfo   `json:"page"`
			}{Data: []crmcontracts.Activity{{
				Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
				Subject: ptr("Send the MCP whitepaper"), OccurredAt: at(1),
			}}},
		},
		// The overdue rung reads BOTH sources; this page reaches it through the
		// task list, the one above it through a claim.
		"overdue task": {
			NextSteps: &struct {
				Data []crmcontracts.Activity `json:"data"`
				Page crmcontracts.PageInfo   `json:"page"`
			}{Data: []crmcontracts.Activity{{
				Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
				Subject: ptr("Send the signed contract"), OccurredAt: at(72), DueAt: ptr(at(30)),
			}}},
		},
		"nothing needed": {},
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			assertActionsAreHonest(t, deriveMoment(readerCtx(), now, page))
		})
	}

	// And every rung directly, because deriveMoment stops at the first rung that
	// fires: a page reaching rung 1 says nothing about rung 9, and the first
	// version of this test passed while three lower rungs offered dead buttons.
	//
	// Asking each rung is not enough on its own. A rung no page triggers returns
	// ok=false every time and is judged by nothing, which reads as covered and
	// is not — so a rung that never fires fails here rather than passing quietly.
	t.Run("every rung", func(t *testing.T) {
		fired := make(map[int]bool, len(momentLadder))
		for _, page := range pages {
			for i, rung := range momentLadder {
				moment, ok := rung(readerCtx(), now, page)
				if !ok {
					continue
				}
				fired[i] = true
				assertActionsAreHonest(t, moment)
			}
		}
		for i, name := range momentLadderNames {
			if !fired[i] {
				t.Errorf("ladder rung %q fires for none of these pages, so its actions are judged by nothing — add a page that reaches it", name)
			}
		}
	})
}

// dispatchedByThePersonPage is the set of destination surfaces the person page
// actually opens — the `switch` in frontend/src/screens/personpage.tsx's
// runPersonMomentAction.
//
// The contract admits more surfaces than the page handles, and the ones it does
// not handle fall to a `default` that deliberately does nothing rather than
// offering a button that is enabled, pressed, and inert. So a contract-valid
// destination is not the bar: this list is, and it is a hand-kept mirror of
// that switch — a surface added there belongs here, and until it is, offering
// it fails rather than shipping another quiet nothing.
var dispatchedByThePersonPage = map[crmcontracts.PersonMomentDestinationSurface]bool{
	crmcontracts.PersonMomentDestinationSurfaceComposer:     true,
	crmcontracts.PersonMomentDestinationSurfaceResearch:     true,
	crmcontracts.PersonMomentDestinationSurfaceMeetingBrief: true,
	crmcontracts.PersonMomentDestinationSurfaceRecord:       true,
	crmcontracts.PersonMomentDestinationSurfaceActivityLog:  true,
	crmcontracts.PersonMomentDestinationSurfaceTask:         true,
}

// assertActionsAreHonest holds the rule for one moment: available means
// reachable, blocked means explained.
func assertActionsAreHonest(t *testing.T, moment crmcontracts.PersonMoment) {
	t.Helper()
	actions := []crmcontracts.PersonMomentAction{moment.RecommendedAction}
	if moment.SecondaryActions != nil {
		actions = append(actions, *moment.SecondaryActions...)
	}
	for _, action := range actions {
		if action.State == crmcontracts.PersonMomentActionStateBlocked {
			if action.BlockedReason == nil || *action.BlockedReason == "" {
				t.Errorf("%s: %q is blocked and gives no reason, so its tooltip is empty",
					moment.Rule, action.Label)
			}
			continue
		}
		if action.Destination == nil {
			t.Errorf("%s: %q is offered as %q with no destination, so pressing it does nothing",
				moment.Rule, action.Label, action.State)
			continue
		}
		if !dispatchedByThePersonPage[action.Destination.Surface] {
			t.Errorf("%s: %q points at surface %q, which personpage.tsx does not open — pressing it does nothing",
				moment.Rule, action.Label, action.Destination.Surface)
			continue
		}
		// A record surface navigates on the entity id and on nothing else, so
		// one without an id is a destination that goes nowhere — the same dead
		// button wearing a destination.
		if action.Destination.Surface == crmcontracts.PersonMomentDestinationSurfaceRecord &&
			action.Destination.EntityId == nil {
			t.Errorf("%s: %q points at a record with no entity id, so the client cannot navigate",
				moment.Rule, action.Label)
		}
	}
}

// Rung 10 always answers. "Nothing needs you today" is a result the reader came
// for, and an empty card fails to give it.
func TestAQuietRecordStillGetsAnAnswer(t *testing.T) {
	page := &crmcontracts.Person360{
		Activities: timelineOf(crmcontracts.Activity{Id: openapi_types.UUID{}, Kind: "email", OccurredAt: at(3)}),
		Network: &struct {
			Colleagues []crmcontracts.PersonNetworkColleague `json:"colleagues"`
		}{Colleagues: []crmcontracts.PersonNetworkColleague{{}}},
	}
	got := deriveMoment(readerCtx(), now, page)
	if got.Rule != crmcontracts.PersonMomentRuleNothingNeeded {
		t.Fatalf("a record with nothing pending gets the quiet success state, got %q", got.Rule)
	}
}

// A section the caller may not read contributes no moment. Absent is not the
// same as empty, and only one of them is a fact about the relationship.
func TestAWithheldSectionDoesNotProduceAThinRelationshipClaim(t *testing.T) {
	page := &crmcontracts.Person360{}
	got := deriveMoment(readerCtx(), now, page)
	if got.Rule == crmcontracts.PersonMomentRuleThinRelationship {
		t.Fatal("activities and network were withheld, not empty; the page must not call the relationship thin")
	}
}

// The dismissal is held against the evidence, so it lifts when the evidence
// moves. A fingerprint that ignored the evidence would silence the page about
// the very thing that just changed.
func TestTheFingerprintChangesWhenTheEvidenceMoves(t *testing.T) {
	first := fingerprintOf([]crmcontracts.PersonMomentEvidence{{
		Type: crmcontracts.PersonMomentEvidenceTypeActivity, ObservedAt: ptr(at(9)),
	}})
	later := fingerprintOf([]crmcontracts.PersonMomentEvidence{{
		Type: crmcontracts.PersonMomentEvidenceTypeActivity, ObservedAt: ptr(at(2)),
	}})
	if first == later {
		t.Fatal("a newer message must re-arm a dismissed moment")
	}
}

// Rewording a headline is not the evidence changing. If prose fed the
// fingerprint, every copy edit would silently un-dismiss what readers put away.
func TestTheFingerprintIgnoresProse(t *testing.T) {
	observed := at(9)
	id := openapi_types.UUID{}
	first := fingerprintOf([]crmcontracts.PersonMomentEvidence{{
		Type: crmcontracts.PersonMomentEvidenceTypeActivity, Id: &id, Label: "Their last message", ObservedAt: &observed,
	}})
	reworded := fingerprintOf([]crmcontracts.PersonMomentEvidence{{
		Type: crmcontracts.PersonMomentEvidenceTypeActivity, Id: &id, Label: "The message they sent", ObservedAt: &observed,
	}})
	if first != reworded {
		t.Fatal("the same evidence under a different label is the same evidence")
	}
}

// A section this reader may not see comes back NIL, exactly like a section that
// is genuinely empty. A rule that reads nil as "there is nothing here" then
// states a confident fact about data the page was never allowed to look at:
// "nothing is scheduled" when the schedule was withheld, "nobody is waiting on
// a reply" when the timeline was.
//
// That is the same defect the drafting program exists to remove — asserting
// something the evidence does not carry — arriving through the record page
// instead of through a draft.
func TestARuleDoesNotClaimAbsenceForASectionItCouldNotRead(t *testing.T) {
	deal := &crmcontracts.Person360Commercial{Deal: &crmcontracts.Person360CommercialDeal{
		DealId: openapi_types.UUID(ids.NewV7()), Title: "Dispatch integration",
	}}

	// An open deal with no visible schedule. Allowed to look, the page says
	// nothing is scheduled; forbidden to look, it must not.
	visible := &crmcontracts.Person360{Commercial: deal}
	if got := deriveMoment(readerCtx(), time.Now(), visible).Rule; got != crmcontracts.PersonMomentRuleMissingNextStep {
		t.Fatalf("with the schedule readable and empty, the gap IS the finding, got %q", got)
	}

	withheldSchedule := &crmcontracts.Person360{
		Commercial: deal,
		SectionsOmitted: []crmcontracts.Person360SectionsOmitted{
			crmcontracts.Person360SectionsOmittedNextMeeting,
		},
	}
	if got := deriveMoment(readerCtx(), time.Now(), withheldSchedule).Rule; got == crmcontracts.PersonMomentRuleMissingNextStep {
		t.Error("the schedule was withheld, so 'nothing is scheduled' is not something this page knows")
	}
}

// And the quiet fall-through says so too, rather than reporting a withheld
// timeline as a clean bill of health.
func TestNothingNeededAdmitsWhenItCouldNotSeeEverything(t *testing.T) {
	full := deriveMoment(readerCtx(), time.Now(), &crmcontracts.Person360{})
	if full.Rule != crmcontracts.PersonMomentRuleNothingNeeded {
		t.Fatalf("an empty readable page is the quiet state, got %q", full.Rule)
	}

	partial := deriveMoment(readerCtx(), time.Now(), &crmcontracts.Person360{
		SectionsOmitted: []crmcontracts.Person360SectionsOmitted{
			crmcontracts.Person360SectionsOmittedActivities,
		},
	})
	if partial.WhyNow == full.WhyNow {
		t.Error("a reader shown only part of the record must not be told nobody is waiting on a reply")
	}
	if !strings.Contains(partial.WhyNow, "not yours to see") {
		t.Errorf("and it must say WHY the picture is partial, got %q", partial.WhyNow)
	}
}

// The ladder offers "log an interaction" from the page alone; whether the
// reader may actually log one is the caller's grant, and the store refuses a
// save without it. So the action a reader cannot complete is handed to them
// blocked, with the reason, rather than as a live button that fails on save.
func TestLogActivityIsWithheldFromACallerWithoutTheCreateGrant(t *testing.T) {
	quiet := nothingNeededMoment(readerCtx(), time.Now(), &crmcontracts.Person360{})
	if quiet.RecommendedAction.Kind != crmcontracts.PersonMomentActionKindLogActivity {
		t.Fatalf("the quiet moment's action is %q, want log_activity — this test needs that rung", quiet.RecommendedAction.Kind)
	}

	reader := quiet
	withholdActivityWrites((as(map[string]principal.ObjectGrant{"person": {Read: true}})), &reader)
	if reader.RecommendedAction.State != crmcontracts.PersonMomentActionStateBlocked {
		t.Errorf("state = %q for a caller without activity.create, want blocked", reader.RecommendedAction.State)
	}
	if reader.RecommendedAction.BlockedReason == nil || *reader.RecommendedAction.BlockedReason == "" {
		t.Error("a withheld action carries no reason, so the reader is refused without being told why")
	}
	if reader.RecommendedAction.Destination != nil {
		t.Error("a withheld action still names a destination, which the client would treat as reachable")
	}

	writer := quiet
	withholdActivityWrites((as(map[string]principal.ObjectGrant{"person": {Read: true}, "activity": {Create: true}})), &writer)
	if writer.RecommendedAction.State != crmcontracts.PersonMomentActionStateAvailable || writer.RecommendedAction.Destination == nil {
		t.Errorf("a caller holding activity.create got %q with destination %v, want the action untouched", writer.RecommendedAction.State, writer.RecommendedAction.Destination)
	}
}

// complete_task writes the same POST /activities as log_activity, through the
// same store and the same activity.create grant — no ladder rung offers it
// today, but the contract admits it, and withholdActivityWrites must hold the
// door for it before a rung does.
func TestCompleteTaskIsWithheldFromACallerWithoutTheCreateGrant(t *testing.T) {
	dest := crmcontracts.PersonMomentDestination{
		Surface: crmcontracts.PersonMomentDestinationSurfaceTask,
	}
	task := crmcontracts.PersonMoment{
		Rule: crmcontracts.PersonMomentRuleOverduePromise,
		RecommendedAction: crmcontracts.PersonMomentAction{
			Kind:        crmcontracts.PersonMomentActionKindCompleteTask,
			State:       crmcontracts.PersonMomentActionStateAvailable,
			Destination: &dest,
		},
	}

	reader := task
	withholdActivityWrites((as(map[string]principal.ObjectGrant{"person": {Read: true}})), &reader)
	if reader.RecommendedAction.State != crmcontracts.PersonMomentActionStateBlocked {
		t.Errorf("state = %q for a caller without activity.create, want blocked", reader.RecommendedAction.State)
	}
	if reader.RecommendedAction.Destination != nil {
		t.Error("a withheld action still names a destination, which the client would treat as reachable")
	}

	writer := task
	withholdActivityWrites((as(map[string]principal.ObjectGrant{"person": {Read: true}, "activity": {Create: true}})), &writer)
	if writer.RecommendedAction.State != crmcontracts.PersonMomentActionStateAvailable || writer.RecommendedAction.Destination == nil {
		t.Errorf("a caller holding activity.create got %q with destination %v, want the action untouched", writer.RecommendedAction.State, writer.RecommendedAction.Destination)
	}
}

// An open task with no date is still a promise. The transcript reader files
// "I'll send you the whitepaper" without one, and the card used to fall past
// every rung to "nothing is owed" while that task sat directly beneath it.
//
// The page arrives ordered by the next-steps read (byUrgency): soonest
// deadline first, then oldest. The card speaks for the first row, so these
// pages are built in that order rather than the timeline's.
func TestAnOpenUndatedTaskIsTheMoment(t *testing.T) {
	// The package's fixed noon, not time.Now(): the "due later today" case
	// below asks a CALENDAR-day question, so a real clock decides it by what
	// hour the suite happens to run. Two hours past 22:00 is tomorrow, and the
	// sentence flips to "Due in 1 days."
	filed := now.Add(-48 * time.Hour)
	page := &crmcontracts.Person360{
		NextSteps: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{
			{Id: openapi_types.UUID(ids.NewV7()), Kind: "task", Subject: ptr("Send the MCP whitepaper"), OccurredAt: filed},
			{Id: openapi_types.UUID(ids.NewV7()), Kind: "task", Subject: ptr("Book the workshop room"), OccurredAt: now.Add(-2 * time.Hour)},
		}},
	}

	moment := deriveMoment(readerCtx(), now, page)
	if moment.Rule != crmcontracts.PersonMomentRuleOpenPromise {
		t.Fatalf("rule = %q, want open_promise — an open task is owed, dated or not", moment.Rule)
	}
	if moment.Headline != "You owe them: Send the MCP whitepaper" {
		t.Errorf("headline = %q, want the section's first task named", moment.Headline)
	}
	if !strings.Contains(moment.WhyNow, "no date set") {
		t.Errorf("why_now = %q, want it to say the promise carries no date", moment.WhyNow)
	}
	if got := (*moment.RecommendedAction.Destination.Prefill)["subject"]; got != "Send the MCP whitepaper" {
		t.Errorf("composer subject = %q, want the promise itself", got)
	}
	if moment.FreshnessAt == nil || !moment.FreshnessAt.Equal(filed) {
		t.Errorf("freshness = %v, want the day the promise was filed", moment.FreshnessAt)
	}

	// A task somebody specific holds is owed by that desk, not by whoever is
	// reading the card.
	page.NextSteps.Data[0].AssigneeId = ptr(openapi_types.UUID(ids.NewV7()))
	if got := deriveMoment(readerCtx(), now, page).Headline; got != "Owed to them: Send the MCP whitepaper" {
		t.Errorf("headline for an assigned task = %q, want it attributed to its holder", got)
	}
	page.NextSteps.Data[0].AssigneeId = nil

	// A task due later today reads as due today rather than counting zero days.
	page.NextSteps.Data[0].DueAt = ptr(now.Add(2 * time.Hour))
	if got := deriveMoment(readerCtx(), now, page).WhyNow; got != "Due today." {
		t.Errorf("why_now for a task due later today = %q, want \"Due today.\"", got)
	}

	// Done tasks never reach the section, so a page whose section is empty is
	// the quiet state — and only then may the card say nothing is owed.
	page.NextSteps.Data = nil
	if got := deriveMoment(readerCtx(), now, page).Rule; got != crmcontracts.PersonMomentRuleNothingNeeded {
		t.Errorf("with no open task, rule = %q, want nothing_needed", got)
	}
}

// A promise past its date outranks a silence, and it must do so whichever
// place the promise was written down. Ranking the task rung below gone_quiet
// made the same lateness win from an email and lose from the task list, which
// is a rule about where a fact was recorded rather than about what the reader
// should do next.
func TestAnOverdueTaskOutranksASilence(t *testing.T) {
	quiet := func() *crmcontracts.Person360 {
		return &crmcontracts.Person360{
			LastOutboundAt: ptr(now.Add(-9 * 24 * time.Hour)),
			LastInboundAt:  ptr(now.Add(-16 * 24 * time.Hour)),
		}
	}
	task := func(due time.Time) []crmcontracts.Activity {
		return []crmcontracts.Activity{{
			Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
			Subject: ptr("Send the signed contract"), OccurredAt: now.Add(-72 * time.Hour), DueAt: ptr(due),
		}}
	}

	// The silence alone is the moment, so the two cases below are a change of
	// rung rather than a page that only ever had one answer.
	if got := deriveMoment(readerCtx(), now, quiet()).Rule; got != crmcontracts.PersonMomentRuleGoneQuiet {
		t.Fatalf("with no task, rule = %q, want gone_quiet — this test needs the silence to fire", got)
	}

	late := quiet()
	late.NextSteps = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: task(now.Add(-30 * time.Hour))}
	moment := deriveMoment(readerCtx(), now, late)
	if moment.Rule != crmcontracts.PersonMomentRuleOverduePromise {
		t.Errorf("rule = %q, want overdue_promise — a promise we are late on outranks their silence, "+
			"and one rung covers both places a promise is written down", moment.Rule)
	}
	if moment.Headline != "You owe them: Send the signed contract" {
		t.Errorf("headline = %q, want the overdue promise named", moment.Headline)
	}
	if !strings.Contains(moment.WhyNow, "still open") {
		t.Errorf("why_now = %q, want it to say the date has passed", moment.WhyNow)
	}

	// Not yet due, so the silence keeps the card and the promise waits below it.
	ahead := quiet()
	ahead.NextSteps = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: task(now.Add(48 * time.Hour))}
	if got := deriveMoment(readerCtx(), now, ahead).Rule; got != crmcontracts.PersonMomentRuleGoneQuiet {
		t.Errorf("rule = %q for a promise still ahead of its date, want gone_quiet", got)
	}
}

// readerCtx is a human reading their own workspace: the seat every ladder
// case below is judged from, since who the reader is decides whether an
// assigned promise is theirs to deliver.
func readerCtx() context.Context {
	return as(map[string]principal.ObjectGrant{"person": {Read: true}, "activity": {Create: true}})
}

// A dismissal is keyed on the moment's fingerprint, so the fingerprint has to
// change when the card does. Handing a colleague's task to the reader turns
// "owed to them" into "you owe them"; sharing one fingerprint would let the
// earlier dismissal suppress the new card, hiding the promise at the moment
// it became the reader's to deliver.
func TestReassigningAPromiseRearmsItsDismissal(t *testing.T) {
	now := time.Now()
	task := crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
		Subject: ptr("Send the signed contract"), OccurredAt: now.Add(-24 * time.Hour),
	}
	page := &crmcontracts.Person360{
		NextSteps: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{task}},
	}

	ours := deriveMoment(readerCtx(), now, page)

	page.NextSteps.Data[0].AssigneeId = ptr(openapi_types.UUID(ids.NewV7()))
	theirs := deriveMoment(readerCtx(), now, page)

	if theirs.Headline == ours.Headline {
		t.Fatalf("both readings say %q; this test needs the card to change", ours.Headline)
	}
	if theirs.EvidenceFingerprint == ours.EvidenceFingerprint {
		t.Error("the fingerprint is unchanged across a reassignment, so a dismissal of one card " +
			"silences the other — the promise disappears exactly when it changes hands")
	}
}

// One rung, two sources, and when both are late it shows the promise closest
// to its deadline. The reader is likeliest to be able to still act on that
// one, and which source recorded it says nothing about that.
func TestTheLatestOverduePromiseWinsWhicheverSourceHoldsIt(t *testing.T) {
	now := time.Now()
	claimDue := now.Add(-10 * 24 * time.Hour)
	overdueClaim := []crmcontracts.ConversationClaim{{
		Kind:             crmcontracts.CommitmentOurs,
		Status:           crmcontracts.ConversationClaimStatusOpen,
		Body:             "Send the revised quote",
		SourceQuote:      "Ich schicke dir das Angebot bis Freitag.",
		SourceActivityId: openapi_types.UUID(ids.NewV7()),
		DueAt:            &claimDue,
	}}
	pageWith := func(taskDue time.Time) *crmcontracts.Person360 {
		return &crmcontracts.Person360{
			Claims: &overdueClaim,
			NextSteps: &struct {
				Data []crmcontracts.Activity `json:"data"`
				Page crmcontracts.PageInfo   `json:"page"`
			}{Data: []crmcontracts.Activity{{
				Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
				Subject: ptr("Send the signed contract"), OccurredAt: now.Add(-72 * time.Hour),
				DueAt: &taskDue,
			}}},
		}
	}

	// The task is the more recent deadline, so it leads.
	recent := deriveMoment(readerCtx(), now, pageWith(now.Add(-2*24*time.Hour)))
	if recent.Rule != crmcontracts.PersonMomentRuleOverduePromise {
		t.Fatalf("rule = %q, want overdue_promise", recent.Rule)
	}
	if recent.Headline != "You owe them: Send the signed contract" {
		t.Errorf("headline = %q, want the task, whose deadline passed most recently", recent.Headline)
	}

	// The claim is, so it leads — and it brings the quote a task cannot.
	older := deriveMoment(readerCtx(), now, pageWith(now.Add(-30*24*time.Hour)))
	if older.Headline != "You owe them: Send the revised quote" {
		t.Errorf("headline = %q, want the claim, whose deadline passed most recently", older.Headline)
	}
	if older.Evidence[0].Snippet == nil {
		t.Error("the claim's card carries no quote; the conversation it was read from is the " +
			"one thing a claim has that a task does not")
	}
}

// The rung shows the promise that slipped MOST RECENTLY, and it has to look at
// all of them to know which that is. Picking the earliest overdue claim and
// comparing only that one against the tasks named the oldest, least
// recoverable promise while one that slipped yesterday went unmentioned.
func TestTheRungLooksPastTheOldestOverduePromise(t *testing.T) {
	now := time.Now()
	claimDue := func(days int) *time.Time {
		at := now.Add(-time.Duration(days) * 24 * time.Hour)
		return &at
	}
	claims := []crmcontracts.ConversationClaim{
		{
			Kind: crmcontracts.CommitmentOurs, Status: crmcontracts.ConversationClaimStatusOpen,
			Body: "Send the ancient quote", SourceQuote: "Bis Montag.",
			SourceActivityId: openapi_types.UUID(ids.NewV7()), DueAt: claimDue(10),
		},
		{
			Kind: crmcontracts.CommitmentOurs, Status: crmcontracts.ConversationClaimStatusOpen,
			Body: "Send yesterday's quote", SourceQuote: "Bis gestern.",
			SourceActivityId: openapi_types.UUID(ids.NewV7()), DueAt: claimDue(1),
		},
	}
	taskDue := now.Add(-5 * 24 * time.Hour)
	page := &crmcontracts.Person360{
		Claims: &claims,
		NextSteps: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{{
			Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
			Subject: ptr("Send the signed contract"), OccurredAt: now.Add(-72 * time.Hour),
			DueAt: &taskDue,
		}}},
	}

	// Yesterday's claim is the most recent slip, ahead of the 5-day task and
	// the 10-day claim.
	if got := deriveMoment(readerCtx(), now, page).Headline; got != "You owe them: Send yesterday's quote" {
		t.Errorf("headline = %q, want the promise that slipped most recently", got)
	}

	// Among several overdue TASKS the same rule holds.
	recent := now.Add(-2 * time.Hour)
	page.NextSteps.Data = append(page.NextSteps.Data, crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
		Subject: ptr("Send this morning's file"), OccurredAt: now.Add(-24 * time.Hour),
		DueAt: &recent,
	})
	if got := deriveMoment(readerCtx(), now, page).Headline; got != "You owe them: Send this morning's file" {
		t.Errorf("headline = %q, want the task that slipped most recently", got)
	}
}

// A dismissal is stored against the claim key, so the key a task's late card
// carries must not change when the rule around it is renamed — every task a
// reader had put away would come back on deploy.
func TestTheLateTaskCardKeepsItsDismissalKey(t *testing.T) {
	now := time.Now()
	due := now.Add(-24 * time.Hour)
	page := &crmcontracts.Person360{
		NextSteps: &struct {
			Data []crmcontracts.Activity `json:"data"`
			Page crmcontracts.PageInfo   `json:"page"`
		}{Data: []crmcontracts.Activity{{
			Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
			Subject: ptr("Send the signed contract"), OccurredAt: now.Add(-72 * time.Hour), DueAt: &due,
		}}},
	}

	moment := deriveMoment(readerCtx(), now, page)
	if moment.ClaimKey != "moment:overdue_task" {
		t.Errorf("claim key = %q, want moment:overdue_task — dismissals are looked up by exact "+
			"key, so renaming it un-dismisses every task a reader already put away", moment.ClaimKey)
	}
	if moment.Rule != crmcontracts.PersonMomentRuleOverduePromise {
		t.Errorf("rule = %q, want overdue_promise — one rung covers both sources", moment.Rule)
	}
}

// A promise read out of a conversation is owed whether or not anybody typed it
// as a task. Before the two sources shared one reader this rung looked only at
// the task list, so a person owing nothing but an extracted commitment was
// told "nothing needs you today" while the commitments card beneath the fold
// listed the promise.
func TestAnUpcomingCommitmentIsTheMomentWithNoTaskFiled(t *testing.T) {
	now := time.Now()
	due := now.Add(3 * 24 * time.Hour)
	said := now.Add(-48 * time.Hour)
	page := &crmcontracts.Person360{
		Claims: &[]crmcontracts.ConversationClaim{{
			Kind:             crmcontracts.CommitmentOurs,
			Status:           crmcontracts.ConversationClaimStatusOpen,
			Body:             "Send the security questionnaire",
			SourceQuote:      "Ich schicke Ihnen den Fragebogen diese Woche.",
			SourceActivityId: openapi_types.UUID(ids.NewV7()),
			DueAt:            &due,
			OccurredAt:       &said,
		}},
	}

	moment := deriveMoment(readerCtx(), now, page)

	if moment.Rule != crmcontracts.PersonMomentRuleOpenPromise {
		t.Fatalf("rule = %q, want open_promise; a commitment nobody typed as a task is still owed", moment.Rule)
	}
	if moment.Headline != "You owe them: Send the security questionnaire" {
		t.Errorf("headline = %q, want the promise itself", moment.Headline)
	}
	if moment.Evidence[0].Snippet == nil {
		t.Error("the card carries no quote; the sentence the promise was made in is what a claim has")
	}
	if moment.WhyNow != "Due in 3 days." {
		t.Errorf("why-now = %q, want the deadline still ahead", moment.WhyNow)
	}
}

// The not-yet-due rung ranks its two sources by date alone, exactly as the
// overdue rung above it does. A nearer task beats a further commitment and the
// reverse, so which table a promise sits in never decides what a reader is
// shown next.
func TestTheNearestUpcomingPromiseWinsWhicheverSourceHoldsIt(t *testing.T) {
	now := time.Now()
	said := now.Add(-48 * time.Hour)
	pageWith := func(claimDays, taskDays int) *crmcontracts.Person360 {
		claimDue := now.Add(time.Duration(claimDays) * 24 * time.Hour)
		taskDue := now.Add(time.Duration(taskDays) * 24 * time.Hour)
		return &crmcontracts.Person360{
			Claims: &[]crmcontracts.ConversationClaim{{
				Kind:             crmcontracts.CommitmentOurs,
				Status:           crmcontracts.ConversationClaimStatusOpen,
				Body:             "Send the questionnaire",
				SourceQuote:      "Diese Woche.",
				SourceActivityId: openapi_types.UUID(ids.NewV7()),
				DueAt:            &claimDue,
				OccurredAt:       &said,
			}},
			NextSteps: &struct {
				Data []crmcontracts.Activity `json:"data"`
				Page crmcontracts.PageInfo   `json:"page"`
			}{Data: []crmcontracts.Activity{{
				Id: openapi_types.UUID(ids.NewV7()), Kind: "task",
				Subject: ptr("Book the workshop"), OccurredAt: said, DueAt: &taskDue,
			}}},
		}
	}

	if got := deriveMoment(readerCtx(), now, pageWith(9, 2)).Headline; got != "You owe them: Book the workshop" {
		t.Errorf("headline = %q, want the task, whose deadline is nearer", got)
	}
	if got := deriveMoment(readerCtx(), now, pageWith(2, 9)).Headline; got != "You owe them: Send the questionnaire" {
		t.Errorf("headline = %q, want the claim, whose deadline is nearer", got)
	}
}

// Two promises read out of ONE message must be dismissable apart. A dismissal
// is one row per (reader, person, claim key), so a key naming only the rung
// would let putting the first away silence the second — a promise the reader
// never dismissed and is never told about.
//
// The fingerprint cannot separate them: it hashes the source row and its
// moment and ignores the words, which both claims share.
func TestTwoPromisesFromOneMessageDismissApart(t *testing.T) {
	now := time.Now()
	said := now.Add(-24 * time.Hour)
	source := openapi_types.UUID(ids.NewV7())
	due := now.Add(48 * time.Hour)
	claim := func(body string) crmcontracts.ConversationClaim {
		return crmcontracts.ConversationClaim{
			Id:               openapi_types.UUID(ids.NewV7()),
			Kind:             crmcontracts.CommitmentOurs,
			Status:           crmcontracts.ConversationClaimStatusOpen,
			Body:             body,
			SourceQuote:      "Ich schicke Ihnen die Unterlagen.",
			SourceActivityId: source,
			DueAt:            &due,
			OccurredAt:       &said,
		}
	}

	nda := openClaimCard(now, claim("Send the NDA"))
	quote := openClaimCard(now, claim("Send the quote"))

	if nda.ClaimKey == quote.ClaimKey {
		t.Errorf("both promises carry claim key %q; dismissing one would silence the other", nda.ClaimKey)
	}
	if nda.EvidenceFingerprint != quote.EvidenceFingerprint {
		t.Error("the fingerprints differ, so this test no longer covers the case it was written for: " +
			"two claims sharing one source message and moment")
	}
}

// A dismissal silences ONE card, not the record. A reader who puts one promise
// away must still be told about the next: stopping at the first rung and
// answering "nothing needs you today" says something false about every promise
// below the dismissed one.
//
// The rung has to resolve this, not the ladder around it. A rung speaks for a
// SET — three open promises, one card — so passing over the rung would hide the
// other two along with the one that was dismissed.
func TestDismissingOnePromiseShowsTheNext(t *testing.T) {
	now := time.Now()
	said := now.Add(-24 * time.Hour)
	source := openapi_types.UUID(ids.NewV7())
	claim := func(body string, dueInDays int) crmcontracts.ConversationClaim {
		due := now.Add(time.Duration(dueInDays) * 24 * time.Hour)
		return crmcontracts.ConversationClaim{
			Id:               openapi_types.UUID(ids.NewV7()),
			Kind:             crmcontracts.CommitmentOurs,
			Status:           crmcontracts.ConversationClaimStatusOpen,
			Body:             body,
			SourceQuote:      "Ich schicke Ihnen beides.",
			SourceActivityId: source,
			DueAt:            &due,
			OccurredAt:       &said,
		}
	}
	page := &crmcontracts.Person360{
		Claims: &[]crmcontracts.ConversationClaim{claim("Send the NDA", 2), claim("Send the quote", 5)},
	}

	first := deriveMoment(readerCtx(), now, page)
	if first.Headline != "You owe them: Send the NDA" {
		t.Fatalf("headline = %q, want the nearer promise first", first.Headline)
	}

	// The reader puts that one away. The second promise is untouched.
	next := deriveMomentPast(readerCtx(), now, page, func(m crmcontracts.PersonMoment) bool {
		return m.ClaimKey == first.ClaimKey
	})
	if next.Rule == crmcontracts.PersonMomentRuleNothingNeeded {
		t.Fatal("dismissing one promise reported the contact as needing nothing, " +
			"while a second promise is still open")
	}
	if next.Headline != "You owe them: Send the quote" {
		t.Errorf("headline = %q, want the promise the reader has not dismissed", next.Headline)
	}
}

// With every promise dismissed there IS nothing left, and the quiet success
// state is the honest answer rather than a card the reader already put away.
func TestDismissingEveryPromiseReachesTheQuietState(t *testing.T) {
	now := time.Now()
	said := now.Add(-24 * time.Hour)
	due := now.Add(48 * time.Hour)
	page := &crmcontracts.Person360{
		Claims: &[]crmcontracts.ConversationClaim{{
			Id:               openapi_types.UUID(ids.NewV7()),
			Kind:             crmcontracts.CommitmentOurs,
			Status:           crmcontracts.ConversationClaimStatusOpen,
			Body:             "Send the NDA",
			SourceQuote:      "Ich schicke die NDA.",
			SourceActivityId: openapi_types.UUID(ids.NewV7()),
			DueAt:            &due,
			OccurredAt:       &said,
		}},
	}
	got := deriveMomentPast(readerCtx(), now, page, func(crmcontracts.PersonMoment) bool { return true })
	if got.Rule != crmcontracts.PersonMomentRuleNothingNeeded {
		t.Errorf("rule = %q, want nothing_needed once every card is dismissed", got.Rule)
	}
}
