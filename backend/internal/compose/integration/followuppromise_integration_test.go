// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One follow-up is one Worklist row: our email nobody answered and the promise
// task read out of that email. The rows and the figures are read through the
// route's own wiring, over records the product's writers wrote.

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// unansweredPromise is our promise mail to a customer contact, the task read
// out of it, and a moment four days on, past the follow-up window.
type unansweredPromise struct {
	e       *Env
	p       PromiseThread
	company ids.CompanyID
	mail    ids.UUID
	task    ids.UUID
	asOf    time.Time
	clock   func() time.Time
}

func seedUnansweredPromise(t *testing.T) unansweredPromise {
	t.Helper()
	u := seedUnansweredMail(t)
	u.task = u.e.SeedPromiseTask(t, u.p)
	return u
}

// seedUnansweredMail is the promise mail and its account, with no task read
// out of it yet.
func seedUnansweredMail(t *testing.T) unansweredPromise {
	t.Helper()
	e := Setup(t)
	p := e.SeedPromiseThread(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Kunde GmbH", &e.AdminUser))
	if _, err := e.Contacts.UpdateCompany(e.Admin(), company, contacts.UpdateCompanyInput{Status: new("customer")}); err != nil {
		t.Fatalf("making the account a customer: %v", err)
	}
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &p.Contact, CompanyID: &company, Source: provenance.RecordSourceManual,
	}); err != nil {
		t.Fatalf("employing the contact at the account: %v", err)
	}
	u := unansweredPromise{e: e, p: p, company: company, mail: ids.UUID(p.Claim.SourceActivityId)}
	u.asOf = p.SentAt.AddDate(0, 0, 4)
	u.clock = func() time.Time { return u.asOf }
	return u
}

func (u unansweredPromise) worklist(t *testing.T) crmcontracts.Worklist {
	t.Helper()
	return u.worklistIn(t, "mine")
}

func (u unansweredPromise) worklistIn(t *testing.T, scope string) crmcontracts.Worklist {
	t.Helper()
	page, err := compose.NewAttentionService(u.e.Pool, approvals.NewService(u.e.DB()), u.clock).
		Worklist(u.e.Admin(), scope, "", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	return page
}

// promiseFor files one more task read out of the promise mail, held by
// assignee and due at due.
func (u unansweredPromise) promiseFor(t *testing.T, assignee ids.UUID, subject string, due time.Time) ids.UUID {
	t.Helper()
	task, _, err := u.e.Activities.LogActivity(u.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: provenance.RecordSourceManual,
		AssigneeID: new(ids.From[ids.UserKind](assignee)), DueAt: &due, SourceActivityID: &u.mail,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: u.p.Contact.UUID}},
	})
	if err != nil {
		t.Fatalf("filing %q: %v", subject, err)
	}
	return ids.UUID(task.Id)
}

func (u unansweredPromise) update(t *testing.T, task ids.UUID, in activities.UpdateActivityInput) {
	t.Helper()
	if _, err := u.e.Activities.UpdateActivity(u.e.Admin(), ids.From[ids.ActivityKind](task), in); err != nil {
		t.Fatalf("updating task %s: %v", task, err)
	}
}

func countTotal(page crmcontracts.Worklist, category crmcontracts.WorklistCountCategory) int {
	for _, count := range page.Counts {
		if count.Category == category && count.Total != nil {
			return *count.Total
		}
	}
	return -1
}

func queued(page crmcontracts.Worklist, source crmcontracts.WorklistItemSource, id ids.UUID) (crmcontracts.WorklistItem, bool) {
	for _, row := range page.Queue {
		if row.Source == source && row.Id == id.String() {
			return row, true
		}
	}
	return crmcontracts.WorklistItem{}, false
}

func tasksTotal(page crmcontracts.Worklist) int {
	return countTotal(page, crmcontracts.WorklistCountCategoryTasks)
}

func TestAnUnansweredPromiseIsOneWorklistRowUntilItsTaskIsCompleted(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)

	page := u.worklist(t)
	if _, shown := queued(page, "awaiting_reply", u.mail); shown {
		t.Fatal("the unanswered mail is its own row beside its promise")
	}
	row, ok := queued(page, "task", u.task)
	if !ok {
		t.Fatalf("the promise is not on the Worklist: %+v", page.Queue)
	}
	if row.EmailSummary == nil || ids.UUID(row.EmailSummary.ActivityId) != u.mail {
		t.Fatalf("the promise does not carry the mail it was read out of: %+v", row.EmailSummary)
	}
	if !slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) || !slices.Contains(row.Actions, crmcontracts.WorklistItemActionsComplete) {
		t.Errorf("actions = %v, want the follow-up draft and the completion", row.Actions)
	}
	if page.Summary.Total != 1 || tasksTotal(page) != 1 {
		t.Errorf("summary total %d, tasks total %d; want the follow-up counted once", page.Summary.Total, tasksTotal(page))
	}

	done := true
	if _, err := u.e.Activities.UpdateActivity(u.e.Admin(), ids.From[ids.ActivityKind](u.task), activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatalf("completing the promise: %v", err)
	}
	after := u.worklist(t)
	if _, shown := queued(after, "awaiting_reply", u.mail); shown {
		t.Error("completing the promise brought its reminder back")
	}
	if _, shown := queued(after, "task", u.task); shown {
		t.Error("the completed promise is still on the Worklist")
	}
}

// Setting the reminder aside is the reminder's own act: the promise stays
// open, on its own row.
func TestAReminderSetAsideLeavesItsPromiseOpen(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)
	if err := u.e.Activities.MarkMessageHandled(u.e.Admin(), ids.From[ids.ActivityKind](u.mail)); err != nil {
		t.Fatalf("setting the reminder aside: %v", err)
	}

	page := u.worklist(t)
	row, ok := queued(page, "task", u.task)
	if !ok || row.EmailSummary != nil {
		t.Fatalf("the promise row = %+v (shown %v), want it open without the set-aside reminder", row, ok)
	}
	if _, shown := queued(page, "awaiting_reply", u.mail); shown {
		t.Error("the reminder set aside is back on the Worklist")
	}
	task, err := u.e.Activities.GetActivity(u.e.Admin(), ids.From[ids.ActivityKind](u.task), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the promise back: %v", err)
	}
	if done := task.IsDone; done != nil && *done {
		t.Error("setting the reminder aside completed the promise")
	}
}

// A conversation with a colleague at the same account keeps its own row. So
// does a task for the same contact that was not read out of the promise mail.
func TestAnotherConversationAtTheSameAccountStaysItsOwnRow(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)
	colleague := u.e.SeedThreadContact(t, "Ben Kunde", "ben@kunde.example")
	if _, err := u.e.Contacts.CreateRelationship(u.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &colleague, CompanyID: &u.company, Source: provenance.RecordSourceManual,
	}); err != nil {
		t.Fatalf("employing the colleague at the account: %v", err)
	}
	other := u.e.LogThreadMail(u.e.Admin(), t, colleague, "thread-contract", "ben@kunde.example", "outbound", u.p.SentAt.Add(time.Hour))
	due := u.p.SentAt.AddDate(0, 0, 3)
	unrelated, _, err := u.e.Activities.LogActivity(u.e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Book the onsite visit"), Source: provenance.RecordSourceManual,
		AssigneeID: new(ids.From[ids.UserKind](u.e.AdminUser)), DueAt: &due,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: u.p.Contact.UUID}},
	})
	if err != nil {
		t.Fatalf("filing the unrelated task: %v", err)
	}

	page := u.worklist(t)
	if row, ok := queued(page, "awaiting_reply", other); !ok || row.EmailSummary == nil {
		t.Errorf("the other conversation lost its reminder: %+v", row)
	}
	if row, ok := queued(page, "task", ids.UUID(unrelated.Id)); !ok || row.EmailSummary != nil {
		t.Errorf("the unrelated task = %+v (shown %v), want its own row without a mail", row, ok)
	}
	if row, ok := queued(page, "task", u.task); !ok || row.EmailSummary == nil || ids.UUID(row.EmailSummary.ActivityId) != u.mail {
		t.Errorf("the promise lost its own mail: %+v", row.EmailSummary)
	}
	if page.Summary.Total != 3 || tasksTotal(page) != 3 {
		t.Errorf("summary total %d, tasks total %d; want three items", page.Summary.Total, tasksTotal(page))
	}
}

// A promise the planned lane cut is still the follow-up's one row, and the
// lane's own count and the scrolled tail each hold it once.
func TestAPromisePastThePlannedCapIsStillOneRow(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)
	earlier := u.p.SentAt.AddDate(0, 0, 1)
	const ahead = 15
	for range ahead {
		if _, _, err := u.e.Activities.LogActivity(u.e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: new("Prepare the renewal"), Source: provenance.RecordSourceManual,
			AssigneeID: new(ids.From[ids.UserKind](u.e.AdminUser)), DueAt: &earlier,
		}); err != nil {
			t.Fatalf("filing a task due before the promise: %v", err)
		}
	}

	page := u.worklist(t)
	if _, shown := queued(page, "awaiting_reply", u.mail); shown {
		t.Fatal("the reminder is its own row while its promise sits past the cap")
	}
	held := 0
	for _, row := range page.Queue {
		if row.Id == u.task.String() {
			held++
			if row.EmailSummary == nil {
				t.Error("the promise read past the cap does not carry its mail")
			}
		}
	}
	if held != 1 {
		t.Errorf("the promise is on the Worklist %d times, want once", held)
	}
	if total := tasksTotal(page); total != ahead+1 {
		t.Errorf("tasks total %d, want %d: the tasks ahead and one follow-up", total, ahead+1)
	}
}

// A snooze moves the promise's date. While the new date is inside the read
// window the follow-up keeps the one row in today's work, because the mail is
// unanswered today. Snoozed past the window, the promise is not read and the
// reminder shows alone.
func TestASnoozedPromiseKeepsItsFollowUpTodayUntilItLeavesTheWindow(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)
	u.update(t, u.task, activities.UpdateActivityInput{DueAt: new(u.asOf.AddDate(0, 0, 2))})

	page := u.worklist(t)
	row, ok := queued(page, "task", u.task)
	if !ok || row.EmailSummary == nil {
		t.Fatalf("the snoozed promise = %+v (shown %v), want it carrying its mail", row, ok)
	}
	if row.Category != crmcontracts.WorklistItemCategoryTasks {
		t.Errorf("category = %s, want tasks: the follow-up is due today", row.Category)
	}
	if _, shown := queued(page, "awaiting_reply", u.mail); shown {
		t.Error("the reminder is its own row beside its snoozed promise")
	}
	if page.Summary.Total != 1 || tasksTotal(page) != 1 || countTotal(page, crmcontracts.WorklistCountCategoryUpcoming) > 0 {
		t.Errorf("summary %d, tasks %d, upcoming %d; want the follow-up once, today", page.Summary.Total, tasksTotal(page),
			countTotal(page, crmcontracts.WorklistCountCategoryUpcoming))
	}

	u.update(t, u.task, activities.UpdateActivityInput{DueAt: new(u.asOf.AddDate(0, 0, 30))})
	page = u.worklist(t)
	if row, shown := queued(page, "awaiting_reply", u.mail); !shown || row.EmailSummary == nil {
		t.Errorf("the reminder = %+v (shown %v), want it alone once its promise left the window", row, shown)
	}
	if _, shown := queued(page, "task", u.task); shown {
		t.Error("a promise past the read window is on the Worklist")
	}
}

// A colleague's task read out of my mail is theirs. It is a plain row under
// the scope that shows it, and my reminder stays its own row. Completing the
// colleague's task does not end my reminder. Only my own promise stands for it.
func TestAColleaguesTaskFromMyMailLeavesMyReminderMine(t *testing.T) {
	t.Parallel()
	u := seedUnansweredMail(t)
	theirs := u.promiseFor(t, u.e.Rep1, "Send Lena the reference list", u.p.SentAt.AddDate(0, 0, 3))

	page := u.worklistIn(t, "all")
	if row, ok := queued(page, "task", theirs); !ok || row.EmailSummary != nil {
		t.Errorf("the colleague's task = %+v (shown %v), want a plain row", row, ok)
	}
	if row, shown := queued(page, "awaiting_reply", u.mail); !shown || row.EmailSummary == nil {
		t.Errorf("my reminder = %+v (shown %v), want it on a row of its own", row, shown)
	}

	u.update(t, theirs, activities.UpdateActivityInput{IsDone: new(true)})
	if _, shown := queued(u.worklist(t), "awaiting_reply", u.mail); !shown {
		t.Error("a colleague's done task ended my reminder")
	}
}

// Of my tasks from one mail, the earliest due carries the reminder and the
// other is a plain row. Completing one of the two keeps the reminder on the
// one still open.
func TestTheEarliestOfMyPromisesCarriesTheReminderUntilAllAreDone(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)
	later := u.promiseFor(t, u.e.AdminUser, "Send the signed order form", u.p.SentAt.AddDate(0, 0, 3))

	page := u.worklist(t)
	if row, ok := queued(page, "task", u.task); !ok || row.EmailSummary == nil {
		t.Errorf("the earliest promise = %+v (shown %v), want it carrying the mail", row, ok)
	}
	if row, ok := queued(page, "task", later); !ok || row.EmailSummary != nil || slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Errorf("the later promise = %+v (shown %v), want a plain row", row, ok)
	}

	u.update(t, u.task, activities.UpdateActivityInput{IsDone: new(true)})
	page = u.worklist(t)
	if row, ok := queued(page, "task", later); !ok || row.EmailSummary == nil {
		t.Errorf("the promise still open = %+v (shown %v), want it carrying the mail", row, ok)
	}
	if _, shown := queued(page, "awaiting_reply", u.mail); shown {
		t.Error("the reminder is its own row while a promise of the mail is open")
	}
}

// A later promise past the upcoming window's cap is still read for its
// follow-up. It joins today's work, and the upcoming count no longer holds it.
func TestALaterPromisePastTheUpcomingCapJoinsTodaysWork(t *testing.T) {
	t.Parallel()
	u := seedUnansweredPromise(t)
	u.update(t, u.task, activities.UpdateActivityInput{DueAt: new(u.asOf.AddDate(0, 0, 5))})
	const ahead = 8
	sooner := u.asOf.AddDate(0, 0, 2)
	for range ahead {
		if _, _, err := u.e.Activities.LogActivity(u.e.Admin(), activities.LogActivityInput{
			Kind: "task", Subject: new("Prepare the renewal"), Source: provenance.RecordSourceManual,
			AssigneeID: new(ids.From[ids.UserKind](u.e.AdminUser)), DueAt: &sooner,
		}); err != nil {
			t.Fatalf("filing a task due before the promise: %v", err)
		}
	}

	page := u.worklist(t)
	row, ok := queued(page, "task", u.task)
	if !ok || row.EmailSummary == nil || row.Category != crmcontracts.WorklistItemCategoryTasks {
		t.Fatalf("the promise past the cap = %+v (shown %v), want it in today's work carrying its mail", row, ok)
	}
	if _, shown := queued(page, "awaiting_reply", u.mail); shown {
		t.Error("the reminder is its own row beside its promise past the cap")
	}
	if tasks, upcoming := tasksTotal(page), countTotal(page, crmcontracts.WorklistCountCategoryUpcoming); tasks != 1 || upcoming != ahead {
		t.Errorf("tasks total %d, upcoming total %d; want 1 and %d", tasks, upcoming, ahead)
	}
}
