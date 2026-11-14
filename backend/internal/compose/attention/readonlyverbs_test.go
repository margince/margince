// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A row offers a verb that writes only to a reader the write would admit. The
// Worklist's checkbox for Mark done is drawn from the same `complete`, so a
// verb offered here and refused on press is a checkbox that lies too.

import (
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARowTheReaderMayNotChangeOffersNoVerbThatWrites(t *testing.T) {
	asOf := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		item     crmcontracts.AttentionItem
		complete bool
	}{
		"a task they may change":    {taskItem(Task{ID: ids.NewV7()}, asOf, asOf, time.UTC), true},
		"a task they may only read": {taskItem(Task{ID: ids.NewV7(), ReadOnly: true}, asOf, asOf, time.UTC), false},
		"a promise they may settle": {commitmentItem(Commitment{ID: ids.NewV7(), DueAt: asOf}, asOf), true},
		"a promise they may not":    {commitmentItem(Commitment{ID: ids.NewV7(), DueAt: asOf, ReadOnly: true}, asOf), false},
	} {
		if got := slices.Contains(tc.item.Actions, crmcontracts.AttentionItemActionsComplete); got != tc.complete {
			t.Errorf("%s: offers complete = %v, want %v (actions %v)", name, got, tc.complete, tc.item.Actions)
		}
	}
}

func TestOnlyATaskNobodyHoldsThatTheReaderMayChangeOffersTake(t *testing.T) {
	asOf := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	holder := ids.NewV7()
	for name, tc := range map[string]struct {
		task Task
		take bool
	}{
		"nobody holds it and they may change it": {Task{ID: ids.NewV7()}, true},
		"nobody holds it and they may only read": {Task{ID: ids.NewV7(), ReadOnly: true}, false},
		"a colleague holds it":                   {Task{ID: ids.NewV7(), AssigneeID: &holder}, false},
	} {
		item := taskItem(tc.task, asOf, asOf, time.UTC)
		if got := slices.Contains(item.Actions, crmcontracts.AttentionItemActionsTake); got != tc.take {
			t.Errorf("%s: offers take = %v, want %v (actions %v)", name, got, tc.take, item.Actions)
		}
	}
}

// The Worklist gates Reassign on `complete`, because a task carries it only
// when the reader may change it. The client has no other writability signal
// on a row, so this tie must not come apart.
func TestATaskCarriesCompleteExactlyWhenTheReaderMayChangeIt(t *testing.T) {
	holder := ids.NewV7()
	for name, tc := range map[string]struct {
		task     Task
		complete bool
	}{
		"unheld and changeable": {Task{ID: ids.NewV7()}, true},
		"held and changeable":   {Task{ID: ids.NewV7(), AssigneeID: &holder}, true},
		"unheld and read-only":  {Task{ID: ids.NewV7(), ReadOnly: true}, false},
		"held and read-only":    {Task{ID: ids.NewV7(), AssigneeID: &holder, ReadOnly: true}, false},
	} {
		if got := slices.Contains(taskActions(tc.task), crmcontracts.AttentionItemActionsComplete); got != tc.complete {
			t.Errorf("%s: carries complete = %v, want %v", name, got, tc.complete)
		}
	}
}

// Decide and cancel both PATCH the meeting, so a reader who may only read it is
// offered neither. The row says why, and Done still takes it off their list.
func TestAMeetingTheReaderMayNotChangeOffersNoAnswer(t *testing.T) {
	asOf := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		meeting MeetingAwaitingOutcome
		answers bool
	}{
		"a meeting they may change":    {MeetingAwaitingOutcome{ID: ids.NewV7(), Subject: "Discovery call"}, true},
		"a meeting they may only read": {MeetingAwaitingOutcome{ID: ids.NewV7(), Subject: "Discovery call", ReadOnly: true}, false},
	} {
		item := meetingAwaitingOutcomeItem(tc.meeting)
		for _, verb := range []crmcontracts.AttentionItemActions{crmcontracts.AttentionItemActionsDecide, crmcontracts.AttentionItemActionsCancel} {
			if got := slices.Contains(item.Actions, verb); got != tc.answers {
				t.Errorf("%s: offers %s = %v, want %v (actions %v)", name, verb, got, tc.answers, item.Actions)
			}
		}
		ranked := classifyUnansweredMeeting(item, asOf)
		says := slices.ContainsFunc(ranked.item.Because, func(r crmcontracts.WorklistReason) bool { return r.Kind == "read_only" })
		if says == tc.answers {
			t.Errorf("%s: says read_only = %v, want %v", name, says, !tc.answers)
		}
		if !slices.Contains(dispositionsFor(ranked, ids.NewV7()), disposeHandled) {
			t.Errorf("%s: offers no Done, so nothing takes it off the reader's list", name)
		}
	}
}

// Done on an unanswered meeting the reader may only read is written. The row
// then stays off their day, and the organizer still answers it.
func TestDoneTakesAMeetingTheReaderCannotAnswerOffTheirDay(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	ref := RowRef{Source: string(crmcontracts.WorklistItemSourceMeetingOutcome), RowID: meeting.String()}
	unanswered := &stubMeetingsAwaitingOutcome{rows: []MeetingAwaitingOutcome{{
		ID: meeting, Subject: "Second demo", StartedAt: readInstant.Add(-2 * time.Hour), ReadOnly: true,
	}}}
	service := func(done *doneStoreSpy) *Service {
		return NewService(
			stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
			nil, nil, nil, &stubMeetings{}, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
			WithMeetingsAwaitingOutcome(unanswered)).WithDone(done)
	}

	spy := &doneStoreSpy{}
	if err := service(spy).MarkRowsDone(pageReader(), []RowRef{ref}); err != nil {
		t.Fatalf("marking the meeting done: %v", err)
	}
	if len(spy.marked) != 1 || !slices.Contains(spy.marked[0], ref) {
		t.Fatalf("Done wrote %v, want the meeting's row", spy.marked)
	}

	marked := &doneStoreSpy{already: map[RowRef]time.Time{ref: readInstant.Add(-time.Minute)}}
	day, err := service(marked).Worklist(pageReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	for _, row := range day.Queue {
		if row.Id == meeting.String() {
			t.Fatalf("the meeting marked done is still on the day: %+v", row)
		}
	}
}
