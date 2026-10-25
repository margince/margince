// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A due reminder on the Worklist, through the real activity writer and the
// attention wiring the route serves.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var reminderRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Read: true},
		"activity":              {Create: true, Read: true, Update: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

type reminderEnv struct {
	e          *integration.Env
	now        time.Time
	rep1, rep2 context.Context
}

func setupReminders(t *testing.T) reminderEnv {
	e := integration.Setup(t)
	return reminderEnv{
		e: e, now: clocktest.Now(t).Truncate(time.Second),
		rep1: e.As(e.Rep1, []ids.UUID{e.Team1}, reminderRepPerms),
		rep2: e.As(e.Rep2, []ids.UUID{e.Team1}, reminderRepPerms),
	}
}

// task writes one task for holder through the real writer; nil holder leaves
// it to nobody.
func (r reminderEnv) task(t *testing.T, subject string, due time.Time, remind *time.Time, holder *ids.UUID) ids.ActivityID {
	t.Helper()
	in := activities.LogActivityInput{Kind: "task", Subject: &subject, DueAt: &due, RemindAt: remind, Source: "manual"}
	if holder != nil {
		in.AssigneeID = new(ids.From[ids.UserKind](*holder))
	}
	row, _, err := r.e.Activities.LogActivity(r.e.Admin(), in)
	if err != nil {
		t.Fatalf("logging %q: %v", subject, err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if holder == nil {
		if _, err := r.e.Activities.UpdateActivity(r.e.Admin(), id, activities.UpdateActivityInput{Clear: []string{"assignee_id"}}); err != nil {
			t.Fatalf("leaving %q to nobody: %v", subject, err)
		}
	}
	return id
}

func (r reminderEnv) worklist(ctx context.Context, t *testing.T, scope string, owner ids.UUID) crmcontracts.Worklist {
	t.Helper()
	svc := newAttentionService(r.e.Pool, approvals.NewService(r.e.DB()), func() time.Time { return r.now })
	page, err := svc.Worklist(ctx, scope, "all", owner, 100, "")
	if err != nil {
		t.Fatalf("reading the %s worklist: %v", scope, err)
	}
	return page
}

func rowFor(page crmcontracts.Worklist, id ids.ActivityID) (int, *crmcontracts.WorklistItem) {
	for i := range page.Queue {
		if page.Queue[i].Id == id.String() {
			return i, &page.Queue[i]
		}
	}
	return -1, nil
}

func saysReminderDue(item *crmcontracts.WorklistItem) bool {
	for _, because := range item.Because {
		if because.Kind == crmcontracts.WorklistReasonKindReminderDue {
			return true
		}
	}
	return false
}

func TestADueReminderLeadsOnlyItsAssigneesWorklist(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	nextWeek := r.now.AddDate(0, 0, 7)
	reminded := r.task(t, "Call back about the renewal", nextWeek, new(r.now.Add(-time.Minute)), &e.Rep1)
	for range 15 {
		r.task(t, "Due today", r.now.Add(-time.Hour), nil, &e.Rep1)
	}

	mine := r.worklist(r.rep1, t, "mine", ids.Nil)
	at, row := rowFor(mine, reminded)
	if at != 0 || !saysReminderDue(row) {
		t.Fatalf("the reminded task sits at %d on its assignee's Worklist, want first and saying why", at)
	}
	if row.AboveNext == nil || row.AboveNext.Comparator != crmcontracts.WorklistComparisonComparatorReminder {
		t.Errorf("the reminded task explains its place as %+v, want the reminder", row.AboveNext)
	}
	if mine.Focus == nil || len(mine.Focus.Items) == 0 || mine.Focus.Items[0].Id != reminded.String() {
		t.Error("the reminded task does not lead Home's focus")
	}
	for _, again := range mine.Queue[1:] {
		if again.Id == reminded.String() {
			t.Error("the reminded task is listed a second time further down")
		}
	}

	if _, other := rowFor(r.worklist(r.rep2, t, "mine", ids.Nil), reminded); other != nil {
		t.Error("a colleague's own Worklist carries the assignee's reminder")
	}
	manager := e.As(e.AdminUser, nil, integration.AdminPerms)
	if _, viewed := rowFor(r.worklist(manager, t, "team", e.Rep1), reminded); viewed != nil && saysReminderDue(viewed) {
		t.Error("a manager opening the assignee's queue is reminded in their place")
	}
}

func TestATaskGivesNoReminderWhenNobodyIsDue(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	past := new(r.now.Add(-time.Minute))
	nextWeek := r.now.AddDate(0, 0, 7)
	done := r.task(t, "Done", nextWeek, past, &e.Rep1)
	if _, err := e.Activities.UpdateActivity(r.rep1, done, activities.UpdateActivityInput{IsDone: new(true)}); err != nil {
		t.Fatal(err)
	}
	archived := r.task(t, "Archived", nextWeek, past, &e.Rep1)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), archived, nil); err != nil {
		t.Fatal(err)
	}
	future := r.task(t, "Future", nextWeek, new(r.now.Add(time.Hour)), &e.Rep1)
	stale := r.task(t, "Stale", nextWeek, new(r.now.AddDate(0, 0, -31)), &e.Rep1)
	unassigned := r.task(t, "Nobody's", nextWeek, past, nil)

	mine := r.worklist(r.rep1, t, "mine", ids.Nil)
	for _, id := range []ids.ActivityID{done, archived, future, stale} {
		if _, row := rowFor(mine, id); row != nil && saysReminderDue(row) {
			t.Errorf("task %s says its reminder is due", id)
		}
	}
	if _, row := rowFor(r.worklist(r.rep1, t, "unassigned", ids.Nil), unassigned); row != nil && saysReminderDue(row) {
		t.Error("a task nobody holds reminds the reader of the unassigned queue")
	}
	if _, row := rowFor(mine, unassigned); row != nil {
		t.Error("a task nobody holds reached a rep's own Worklist through its reminder")
	}
}

func TestADueReminderKeepsAnOldTaskOnTheWorklist(t *testing.T) {
	r := setupReminders(t)
	old := r.task(t, "Missed last month", r.now.AddDate(0, 0, -40), new(r.now.Add(-time.Minute)), &r.e.Rep1)
	if at, row := rowFor(r.worklist(r.rep1, t, "mine", ids.Nil), old); at != 0 || !saysReminderDue(row) {
		t.Errorf("a task past the age-out with a due reminder sits at %d, want first", at)
	}
}

// The lane's count is the rows it lists: the day's tasks plus the reminders
// ahead of them, each task once.
func TestTheTaskCountCountsEachDueReminderOnce(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	past := new(r.now.Add(-time.Minute))
	r.task(t, "Due today", r.now.Add(-time.Hour), nil, &e.Rep1)
	r.task(t, "Due today, reminded", r.now.Add(-time.Hour), past, &e.Rep1)
	r.task(t, "Next week, reminded", r.now.AddDate(0, 0, 7), past, &e.Rep1)

	tasks := attentionTasks{store: e.Activities}
	until := r.now.Add(time.Hour)
	total, err := tasks.CountOpenForViewer(r.rep1, r.now, until, attention.TasksMine, ids.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Errorf("the lane counts %d tasks, want 3", total)
	}
	reminders, err := tasks.RemindersForViewer(r.rep1, r.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 2 {
		t.Errorf("the reminder read lists %d tasks, want 2", len(reminders))
	}
}

// More reminders than the day's cap all reach the Worklist, and the lane says
// it holds them all.
func TestEveryDueReminderReachesTheWorklist(t *testing.T) {
	r := setupReminders(t)
	want := map[string]bool{}
	for range 13 {
		want[r.task(t, "Reminded", r.now.AddDate(0, 0, 7), new(r.now.Add(-time.Minute)), &r.e.Rep1).String()] = true
	}

	mine := r.worklist(r.rep1, t, "mine", ids.Nil)
	listed := 0
	for i := range mine.Queue {
		if want[mine.Queue[i].Id] && saysReminderDue(&mine.Queue[i]) {
			listed++
		}
	}
	if listed != len(want) {
		t.Errorf("the Worklist lists %d of the %d due reminders", listed, len(want))
	}
	for _, reach := range mine.Reach {
		if reach.Source == crmcontracts.WorklistReachSourceTask && reach.MoreAvailable {
			t.Error("the task lane says more are available when it lists every reminder")
		}
	}
	total, err := attentionTasks{store: r.e.Activities}.CountOpenForViewer(r.rep1, r.now, r.now.Add(time.Hour), attention.TasksMine, ids.Nil)
	if err != nil || total != len(want) {
		t.Errorf("the lane counts %d (err %v), want the %d it lists", total, err, len(want))
	}
}

// The reminder read follows the store's cursor past its largest page.
func TestTheReminderReadFollowsItsCursor(t *testing.T) {
	r := setupReminders(t)
	for range reminderPage + 1 {
		r.task(t, "Reminded", r.now.AddDate(0, 0, 7), new(r.now.Add(-time.Minute)), &r.e.Rep1)
	}
	due, err := attentionTasks{store: r.e.Activities}.RemindersForViewer(r.rep1, r.now)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[ids.UUID]bool{}
	for _, task := range due {
		seen[task.ID] = true
	}
	if len(seen) != reminderPage+1 {
		t.Errorf("the reminder read reached %d distinct tasks, want %d", len(seen), reminderPage+1)
	}
}

func TestMovingTheDueDateEndsAPassedReminderInTheSameWrite(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	nextWeek := r.now.AddDate(0, 0, 7)
	later := nextWeek.AddDate(0, 0, 1)
	passed := r.task(t, "Passed", nextWeek, new(r.now.Add(-time.Minute)), &e.Rep1)
	named := r.task(t, "Named", nextWeek, new(r.now.Add(-time.Minute)), &e.Rep1)
	coming := r.task(t, "Coming", nextWeek, new(r.now.Add(time.Hour)), &e.Rep1)
	store := e.Activities.WithClock(func() time.Time { return r.now })

	moved, err := store.UpdateActivity(r.rep1, passed, activities.UpdateActivityInput{DueAt: &later})
	if err != nil {
		t.Fatal(err)
	}
	if moved.RemindAt != nil {
		t.Errorf("moving the due date left the passed reminder at %v", *moved.RemindAt)
	}
	var trailed bool
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT after ? 'remind_at' AND after->'remind_at' = 'null'::jsonb FROM audit_log
		WHERE entity_id = $1 AND action = 'update' ORDER BY id DESC LIMIT 1`, passed.UUID).Scan(&trailed); err != nil {
		t.Fatalf("reading the trail: %v", err)
	}
	if !trailed {
		t.Error("the update's audit row does not say the reminder was removed")
	}

	if kept, err := store.UpdateActivity(r.rep1, named, activities.UpdateActivityInput{DueAt: &later, RemindAt: &later}); err != nil || kept.RemindAt == nil {
		t.Errorf("a patch naming the reminder lost it (err %v)", err)
	}
	if kept, err := store.UpdateActivity(r.rep1, coming, activities.UpdateActivityInput{DueAt: &later}); err != nil || kept.RemindAt == nil {
		t.Errorf("moving the due date cleared a reminder still to come (err %v)", err)
	}
}

// The filter follows the row at the next read. The cases are a reminder moved
// into the past, a task written with one, and a task handed to a colleague.
func TestTheReminderFilterFollowsEditsAndReassignment(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	store := e.Activities.WithClock(func() time.Time { return r.now })
	nextWeek := r.now.AddDate(0, 0, 7)
	edited := r.task(t, "Edited", nextWeek, new(r.now.Add(time.Hour)), &e.Rep1)
	written := r.task(t, "Written due", nextWeek, new(r.now.Add(-time.Minute)), &e.Rep1)
	handed := r.task(t, "Handed over", nextWeek, new(r.now.Add(-time.Minute)), &e.Rep1)

	if _, err := store.UpdateActivity(r.rep1, edited, activities.UpdateActivityInput{RemindAt: new(r.now.Add(-time.Minute))}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateActivity(r.rep1, handed, activities.UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](e.Rep2))}); err != nil {
		t.Fatal(err)
	}

	listed := func(ctx context.Context) map[string]bool {
		rows, _, err := store.ListActivities(ctx, activities.ListActivitiesInput{RemindersDue: true})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]bool{}
		for _, row := range rows {
			out[row.Id.String()] = true
		}
		return out
	}
	rep1, rep2 := listed(r.rep1), listed(r.rep2)
	if !rep1[edited.String()] || !rep1[written.String()] || rep1[handed.String()] || len(rep1) != 2 {
		t.Errorf("the assignee's reminders are %v, want the edited and the written task", rep1)
	}
	if !rep2[handed.String()] || len(rep2) != 1 {
		t.Errorf("the colleague's reminders are %v, want the task handed to them", rep2)
	}
}

func TestATaskTheReaderMayNotReadIsNotOnTheirWorklist(t *testing.T) {
	r := setupReminders(t)
	e := r.e
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	subject := "Renewal terms for the Buyer account"
	due := r.now.Add(-time.Hour)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, RemindAt: new(r.now.Add(-time.Minute)), Source: "manual",
		AssigneeID: new(ids.From[ids.UserKind](e.Rep2)),
		Links:      []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	hidden := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := e.Activities.SetAudience(e.Admin(), hidden, activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep1}},
	}); err != nil {
		t.Fatal(err)
	}
	readable := r.task(t, "Send the agenda", due, nil, &e.Rep2)

	page := r.worklist(r.rep2, t, "mine", ids.Nil)

	if at, _ := rowFor(page, hidden); at >= 0 {
		t.Errorf("a task the reader may not read sits at %d on their Worklist", at)
	}
	if at, _ := rowFor(page, readable); at < 0 {
		t.Error("the readable task is missing from the same Worklist")
	}
	for _, count := range page.Counts {
		if count.Category == crmcontracts.WorklistCountCategoryTasks && count.Considered != 1 {
			t.Errorf("tasks considered = %d, want only the readable one", count.Considered)
		}
	}
	for _, item := range page.Queue {
		if item.Title != nil && *item.Title == subject {
			t.Error("the hidden task's subject reached the Worklist")
		}
	}
}
