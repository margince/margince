// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// followUpDay is Mira's account: our email promising the team and revised
// pricing, unanswered for four days, and the promise task read out of it.
type followUpDay struct {
	contact, sent, promise ids.UUID
	reminder               AwaitedReply
	task                   Task
}

func newFollowUpDay() followUpDay {
	contact, sent := ids.NewV7(), ids.NewV7()
	due := rankInstant.Add(2 * time.Hour)
	return followUpDay{
		contact: contact, sent: sent, promise: ids.NewV7(),
		reminder: AwaitedReply{
			ActivityID: sent, Subject: "Delivery team and pricing", SentAt: rankInstant.AddDate(0, 0, -4),
			ContactID: contact, EmailSummary: &crmcontracts.EmailSummary{ActivityId: openapi_types.UUID(sent)},
		},
		task: Task{
			AssigneeID: &pageSeat, Subject: "Follow up with Mira and Ben about the team and revised pricing", DueAt: &due,
			SourceActivityID: &sent, LinkType: string(subjectContact), LinkID: contact, Commitment: true,
		},
	}
}

// otherTask is one more open task for the same contact, derived from source
// when it is set.
func (d followUpDay) otherTask(subject string, source *ids.UUID) Task {
	due := rankInstant.Add(3 * time.Hour)
	return Task{
		ID: ids.NewV7(), AssigneeID: &pageSeat, Subject: subject, DueAt: &due, SourceActivityID: source,
		LinkType: string(subjectContact), LinkID: d.contact,
	}
}

func (d followUpDay) worklist(t *testing.T, tasks []Task, followUps []ranked) crmcontracts.Worklist {
	t.Helper()
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems(tasks, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
	s := &Service{followUps: followUpRead{rows: followUps, read: true}}
	return s.worklistFrom(pageReader(), day, scopeMine, "", 50, waitingRead{}, worklistCursor{}, nil)
}

func rowOf(page crmcontracts.Worklist, source crmcontracts.WorklistItemSource, id ids.UUID) (crmcontracts.WorklistItem, bool) {
	for _, row := range page.Queue {
		if row.Source == source && row.Id == id.String() {
			return row, true
		}
	}
	return crmcontracts.WorklistItem{}, false
}

func reasonKinds(row crmcontracts.WorklistItem) []crmcontracts.WorklistReasonKind {
	kinds := make([]crmcontracts.WorklistReasonKind, 0, len(row.Because))
	for _, because := range row.Because {
		kinds = append(kinds, because.Kind)
	}
	return kinds
}

func TestAnUnansweredEmailAndItsPromiseAreOneRowLedByThePromise(t *testing.T) {
	d := newFollowUpDay()
	d.task.ID = d.promise
	page := d.worklist(t, []Task{d.task}, []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)})

	if _, shown := rowOf(page, sourceAwaitingReply, d.sent); shown {
		t.Fatal("the reminder is still its own row beside the promise")
	}
	row, ok := rowOf(page, sourceTask, d.promise)
	if !ok {
		t.Fatalf("the promise is not on the page: %+v", page.Queue)
	}
	if row.Title == nil || *row.Title != d.task.Subject {
		t.Errorf("title = %v, want the promise", row.Title)
	}
	if row.EmailSummary == nil || ids.UUID(row.EmailSummary.ActivityId) != d.sent {
		t.Errorf("email summary = %+v, want the sent email as context", row.EmailSummary)
	}
	if kinds := reasonKinds(row); !slices.Contains(kinds, "you_wrote_last") || !slices.Contains(kinds, "no_reply_days") {
		t.Errorf("because = %v, want the email's silence among the reasons", kinds)
	}
	for _, action := range []crmcontracts.WorklistItemActions{crmcontracts.WorklistItemActionsComplete, crmcontracts.WorklistItemActionsReply} {
		if !slices.Contains(row.Actions, action) {
			t.Errorf("actions = %v, want %s", row.Actions, action)
		}
	}
	if row.Move == nil || row.Move.Action != crmcontracts.WorklistMoveActionDraftReply ||
		row.Move.ActivityId == nil || ids.UUID(*row.Move.ActivityId) != d.sent {
		t.Errorf("move = %+v, want one follow-up drafted from the sent email", row.Move)
	}
	if page.Summary.Total != 1 {
		t.Errorf("summary total = %d, want the follow-up counted once", page.Summary.Total)
	}
	for _, count := range page.Counts {
		if count.Category == crmcontracts.WorklistCountCategoryTasks && (count.Total == nil || *count.Total != 1) {
			t.Errorf("tasks total = %v, want 1", count.Total)
		}
	}
}

// Only the task derived from the email takes the reminder. Another promise to
// the same contact, a second unanswered email and a meeting with its own
// follow-up task each stay a row of their own.
func TestUnrelatedWorkKeepsItsOwnRows(t *testing.T) {
	d := newFollowUpDay()
	other := ids.NewV7()
	otherMail := d.reminder
	otherMail.ActivityID, otherMail.EmailSummary = other, &crmcontracts.EmailSummary{ActivityId: openapi_types.UUID(other)}
	met := ids.NewV7()
	meeting := AwaitedReply{ActivityID: met, Subject: "Workshop", SentAt: rankInstant.AddDate(0, 0, -3), ContactID: d.contact, Recipients: []ids.UUID{d.contact}}
	meetingRow, ok := classifyMeetingFollowUp(meeting, rankInstant, time.UTC)
	if !ok {
		t.Fatal("the meeting fixture asks for no follow-up")
	}
	sameContact := d.otherTask("Send the case study", nil)
	fromMeeting := d.otherTask("Share the workshop notes", &met)

	page := d.worklist(t, []Task{sameContact, fromMeeting},
		[]ranked{classifyAwaiting(otherMail, rankInstant, time.UTC), meetingRow})

	for name, at := range map[string]struct {
		source crmcontracts.WorklistItemSource
		id     ids.UUID
	}{
		"the other email": {sourceAwaitingReply, other}, "the meeting": {sourceMeetingFollowUp, met},
		"the promise to the same contact": {sourceTask, sameContact.ID}, "the meeting's task": {sourceTask, fromMeeting.ID},
	} {
		row, shown := rowOf(page, at.source, at.id)
		if !shown {
			t.Errorf("%s lost its row", name)
			continue
		}
		if at.source == sourceTask && row.EmailSummary != nil {
			t.Errorf("%s took an email it was not read out of", name)
		}
	}
	if len(page.Queue) != 4 || page.Summary.Total != 4 {
		t.Errorf("queue holds %d rows and counts %d, want 4 separate items", len(page.Queue), page.Summary.Total)
	}
}

// A reminder the reader put down never reaches the fold: the task stays open
// and plain, and nothing marks it complete.
func TestAReminderMarkedDoneLeavesThePromiseAsItWas(t *testing.T) {
	d := newFollowUpDay()
	d.task.ID = d.promise
	reminder := classifyAwaiting(d.reminder, rankInstant, time.UTC)
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems([]Task{d.task}, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
	s := &Service{
		followUps: followUpRead{rows: []ranked{reminder}, read: true},
		marked:    doneState{at: map[RowRef]time.Time{refOf(reminder): rankInstant.Add(-time.Hour)}},
	}
	page := s.worklistFrom(pageReader(), day, scopeMine, "", 50, waitingRead{}, worklistCursor{}, nil)

	row, ok := rowOf(page, sourceTask, d.promise)
	if !ok || len(page.Queue) != 1 {
		t.Fatalf("queue = %+v, want the promise alone", page.Queue)
	}
	if row.EmailSummary != nil || slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Errorf("the promise carries the reminder the reader put down: %+v", row)
	}
	if !slices.Contains(row.Actions, crmcontracts.WorklistItemActionsComplete) {
		t.Errorf("actions = %v, want the task still open to complete", row.Actions)
	}
}

// fromActivities is a task backlog holding the tasks derived from activities,
// and the sources each read narrowed to.
type fromActivities struct {
	windowRecorder
	tasks []Task
	asked [][]ids.UUID
}

func (b *fromActivities) TasksAfter(_ context.Context, window TaskWindow, _ string, _ int) ([]Positioned[Task], bool, error) {
	b.asked = append(b.asked, window.DerivedFrom)
	read := make([]Positioned[Task], 0, len(b.tasks))
	for _, task := range b.tasks {
		read = append(read, Positioned[Task]{Row: task, After: task.ID.String()})
	}
	return read, false, nil
}

// A promise past the planned lane's cap is read for the reminder. The two are
// one row on the first page, and the tail never meets the task again.
func TestAPromisePastTheLanesCapStillTakesItsReminder(t *testing.T) {
	d := newFollowUpDay()
	d.task.ID = d.promise
	full := make([]Task, 0, plannedCap)
	for range plannedCap {
		full = append(full, d.otherTask("Agreed work", nil))
	}
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems(full, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
	backlog := &fromActivities{tasks: []Task{d.task, full[0]}}
	s := (&Service{followUps: followUpRead{rows: []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)}, read: true}}).
		WithTaskBacklog(backlog)
	s.backlog.until = rankInstant.Add(8 * time.Hour)

	read := s.readingFollowUpPromises(t.Context(), day)

	if len(backlog.asked) != 1 || !slices.Equal(backlog.asked[0], []ids.UUID{d.sent}) {
		t.Fatalf("asked for %v, want the reminder's email once", backlog.asked)
	}
	if len(read.followUps.promises) != 1 || read.followUps.promises[0].item.Id != d.promise.String() {
		t.Fatalf("promises = %+v, want the task past the cap and not the one already on the day", read.followUps.promises)
	}
	page := read.worklistFrom(pageReader(), day, scopeMine, "", 50, waitingRead{}, worklistCursor{}, nil)
	if _, shown := rowOf(page, sourceAwaitingReply, d.sent); shown {
		t.Error("the reminder stayed its own row beside the promise read past the cap")
	}
	if row, ok := rowOf(page, sourceTask, d.promise); !ok || row.EmailSummary == nil {
		t.Errorf("the promise past the cap did not take its reminder: %+v", row)
	}
	if page.Summary.Total != plannedCap+1 {
		t.Errorf("summary total = %d, want %d: the cap's tasks and one follow-up", page.Summary.Total, plannedCap+1)
	}
}

// A lane read whole already holds every task an email could have, so no
// second read is made.
func TestAWholeLaneReadsNoPromiseForAFollowUp(t *testing.T) {
	d := newFollowUpDay()
	day := crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems([]Task{d.otherTask("Agreed work", nil)}, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
	backlog := &fromActivities{}
	s := (&Service{followUps: followUpRead{rows: []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)}, read: true}}).
		WithTaskBacklog(backlog)
	s.backlog.until = rankInstant.Add(8 * time.Hour)

	s.readingFollowUpPromises(t.Context(), day)

	if len(backlog.asked) != 0 {
		t.Fatalf("a whole lane asked the store for %v", backlog.asked)
	}
}
