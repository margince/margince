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
	"github.com/margince/margince/backend/internal/modules/activities"
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

// countOf is a category's stated total, or -1 when the page states none.
func countOf(page crmcontracts.Worklist, category crmcontracts.WorklistCountCategory) int {
	for _, count := range page.Counts {
		if count.Category == category && count.Total != nil {
			return *count.Total
		}
	}
	return -1
}

// A promise due in two days still answers an email that is unanswered today.
// The one row stays in today's work, and the counts hold it there once. A
// snooze moves only the task's date, so a snoozed promise reads the same.
func TestAPromiseDueLaterKeepsItsFollowUpInTodaysWork(t *testing.T) {
	d := newFollowUpDay()
	d.task.ID, d.task.DueAt = d.promise, new(rankInstant.AddDate(0, 0, 2))
	page := d.worklist(t, []Task{d.task}, []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)})

	row, ok := rowOf(page, sourceTask, d.promise)
	if !ok || row.EmailSummary == nil {
		t.Fatalf("the later promise did not take its reminder: %+v", page.Queue)
	}
	if row.Category != crmcontracts.WorklistItemCategoryTasks {
		t.Errorf("category = %s, want tasks: the follow-up is due today", row.Category)
	}
	if page.Summary.Total != 1 {
		t.Errorf("summary total = %d, want the follow-up counted today", page.Summary.Total)
	}
	if tasks, upcoming := countOf(page, crmcontracts.WorklistCountCategoryTasks), countOf(page, crmcontracts.WorklistCountCategoryUpcoming); tasks != 1 || upcoming > 0 {
		t.Errorf("tasks total %d, upcoming total %d; want the row under tasks alone", tasks, upcoming)
	}
}

// A later window the store counted still holds a promise its follow-up pulled
// into today, so the count moves with the row.
func TestAPulledForwardPromiseMovesItsStoreCount(t *testing.T) {
	today := totalKey{sourceTask, crmcontracts.WorklistCountCategoryTasks}
	later := totalKey{sourceTask, crmcontracts.WorklistCountCategoryUpcoming}
	s := &Service{backlog: backlogState{exact: map[totalKey]int{today: 12, later: 9}}}
	pulled := ranked{item: crmcontracts.WorklistItem{Source: sourceTask, Category: crmcontracts.WorklistItemCategoryTasks}, pulledForward: true}

	exact := s.exactSources(crmcontracts.Attention{}, scopeMine, []ranked{pulled})

	if exact[today] != 13 || exact[later] != 8 {
		t.Errorf("tasks %d, upcoming %d; want 13 and 8", exact[today], exact[later])
	}
}

// A colleague's task read out of my email is theirs, not my promise. It stays
// a plain row, and my reminder stays mine, under every scope that shows both.
func TestAColleaguesTaskFromMyEmailDoesNotTakeMyReminder(t *testing.T) {
	colleague := ids.NewV7()
	for _, scope := range []string{scopeTeam, scopeAll} {
		t.Run(scope, func(t *testing.T) {
			d := newFollowUpDay()
			d.task.ID, d.task.AssigneeID = d.promise, &colleague
			day := crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems([]Task{d.task}, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
			s := &Service{
				followUps: followUpRead{rows: []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)}, read: true},
				teammates: roster{{UserID: colleague}, {UserID: pageSeat}},
			}
			page := s.worklistFrom(pageReader(), day, scope, "", 50, waitingRead{}, worklistCursor{}, nil)

			if row, ok := rowOf(page, sourceTask, d.promise); !ok || row.EmailSummary != nil || slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
				t.Errorf("the colleague's task = %+v (shown %v), want a plain row", row, ok)
			}
			if _, ok := rowOf(page, sourceAwaitingReply, d.sent); !ok {
				t.Error("my reminder went into a colleague's task")
			}
		})
	}
}

// Of my tasks from one email, the earliest due carries the reminder. The other
// stays a plain row with nothing to draft.
func TestTheEarliestDuePromiseOfAnEmailTakesTheReminder(t *testing.T) {
	d := newFollowUpDay()
	later := d.otherTask("Send the signed order form", &d.sent)
	d.task.ID = d.promise
	page := d.worklist(t, []Task{later, d.task}, []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)})

	if row, ok := rowOf(page, sourceTask, d.promise); !ok || row.EmailSummary == nil {
		t.Errorf("the earliest promise = %+v (shown %v), want it carrying the reminder", row, ok)
	}
	if row, ok := rowOf(page, sourceTask, later.ID); !ok || row.EmailSummary != nil || slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Errorf("the later promise = %+v (shown %v), want a plain row", row, ok)
	}
	if len(page.Queue) != 2 {
		t.Errorf("queue holds %d rows, want the two tasks", len(page.Queue))
	}
}

// fromEmails is a task backlog holding the reader's promises of some emails,
// and what each read asked for.
type fromEmails struct {
	windowRecorder
	promises    []Task
	askedEmails [][]ids.UUID
	askedTasks  [][]ids.UUID
	windows     []TaskWindow
}

func (b *fromEmails) FollowUpPromises(_ context.Context, emails []ids.UUID) ([]activities.FollowUpPromise, error) {
	b.askedEmails = append(b.askedEmails, emails)
	var named []activities.FollowUpPromise
	for _, task := range b.promises {
		if task.SourceActivityID != nil && slices.Contains(emails, *task.SourceActivityID) {
			named = append(named, activities.FollowUpPromise{ID: task.ID, Email: *task.SourceActivityID, DueAt: task.DueAt})
		}
	}
	return named, nil
}

func (b *fromEmails) TasksIn(_ context.Context, window TaskWindow, asked []ids.UUID) ([]Task, error) {
	b.askedTasks, b.windows = append(b.askedTasks, asked), append(b.windows, window)
	var read []Task
	for _, task := range b.promises {
		if slices.Contains(asked, task.ID) {
			read = append(read, task)
		}
	}
	return read, nil
}

// cutDay is a planned lane at its cap with tasks due in an hour, plus the
// tasks named.
func (d followUpDay) cutDay(onTheDay ...Task) crmcontracts.Attention {
	full := slices.Clone(onTheDay)
	for len(full) < plannedCap {
		task := d.otherTask("Agreed work", nil)
		task.DueAt = new(rankInstant.Add(time.Hour))
		full = append(full, task)
	}
	return crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems(full, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
}

func (d followUpDay) pastTheCap(backlog *fromEmails) *Service {
	s := (&Service{followUps: followUpRead{rows: []ranked{classifyAwaiting(d.reminder, rankInstant, time.UTC)}, read: true}}).
		WithTaskBacklog(backlog)
	s.backlog.until = rankInstant.Add(8 * time.Hour)
	return s
}

// A promise past the planned lane's cap is read for the reminder, through the
// lane's read and as the reader's own. The two are one row on the first page,
// and the tail never meets the task again.
func TestAPromisePastTheLanesCapStillTakesItsReminder(t *testing.T) {
	d := newFollowUpDay()
	d.task.ID = d.promise
	day := d.cutDay()
	backlog := &fromEmails{promises: []Task{d.task}}

	read := d.pastTheCap(backlog).readingFollowUpPromises(pageReader(), day)

	if len(backlog.askedEmails) != 1 || !slices.Equal(backlog.askedEmails[0], []ids.UUID{d.sent}) {
		t.Fatalf("asked for %v, want the reminder's email once", backlog.askedEmails)
	}
	if len(backlog.windows) != 1 || backlog.windows[0].Scope != TasksMine ||
		!backlog.windows[0].Until.Equal(rankInstant.Add(8*time.Hour).AddDate(0, 0, upcomingHorizonDays)) {
		t.Errorf("windows = %+v, want the reader's own tasks through the upcoming horizon", backlog.windows)
	}
	if len(read.followUps.promises) != 1 || read.followUps.promises[0].item.Id != d.promise.String() {
		t.Fatalf("promises = %+v, want the task past the cap", read.followUps.promises)
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

// The earliest-due rule holds across the cap. A promise past it that comes due
// before mine on the day is read and takes the reminder. One due after it is
// not read at all.
func TestTheEarliestDuePromiseTakesTheReminderAcrossTheCap(t *testing.T) {
	d := newFollowUpDay()
	onTheDay := d.otherTask("Send the signed order form", &d.sent)
	onTheDay.DueAt = new(rankInstant.Add(4 * time.Hour))
	earlier, later := d.task, d.otherTask("Book the onsite visit", &d.sent)
	earlier.ID, later.DueAt = d.promise, new(rankInstant.Add(6*time.Hour))
	day := d.cutDay(onTheDay)
	backlog := &fromEmails{promises: []Task{onTheDay, earlier, later}}

	read := d.pastTheCap(backlog).readingFollowUpPromises(pageReader(), day)

	if len(backlog.askedTasks) != 1 || !slices.Equal(backlog.askedTasks[0], []ids.UUID{d.promise}) {
		t.Fatalf("read %v, want only the promise due before the one on the day", backlog.askedTasks)
	}
	page := read.worklistFrom(pageReader(), day, scopeMine, "", 50, waitingRead{}, worklistCursor{}, nil)
	if row, ok := rowOf(page, sourceTask, d.promise); !ok || row.EmailSummary == nil {
		t.Errorf("the earlier promise past the cap = %+v (shown %v), want it carrying the reminder", row, ok)
	}
	if row, ok := rowOf(page, sourceTask, onTheDay.ID); !ok || row.EmailSummary != nil {
		t.Errorf("the promise on the day = %+v (shown %v), want a plain row", row, ok)
	}
}

// A lane read whole already holds every task an email could have. A Done
// check asking no reminder cannot be changed by the read. Neither reads.
func TestAWholeLaneOrAProbeWithoutAReminderReadsNoPromise(t *testing.T) {
	d := newFollowUpDay()
	whole := crmcontracts.Attention{AsOf: rankInstant, Planned: plannedItems([]Task{d.otherTask("Agreed work", nil)}, rankInstant, rankInstant.Add(8*time.Hour), time.UTC)}
	backlog := &fromEmails{promises: []Task{d.task}}
	d.pastTheCap(backlog).readingFollowUpPromises(pageReader(), whole)

	probe := d.pastTheCap(backlog)
	probe.marked.probe = func([]ranked) {}
	probe.marked.asked = map[crmcontracts.WorklistItemSource][]ids.UUID{sourceWaiting: {ids.NewV7()}}
	probe.readingFollowUpPromises(pageReader(), d.cutDay())
	if len(backlog.askedEmails) != 0 {
		t.Fatalf("asked the store for %v", backlog.askedEmails)
	}

	probe.marked.asked[sourceAwaitingReply] = []ids.UUID{d.sent}
	probe.readingFollowUpPromises(pageReader(), d.cutDay())
	if len(backlog.askedEmails) != 1 {
		t.Errorf("a Done check asking the reminder read %d times, want once", len(backlog.askedEmails))
	}
}
