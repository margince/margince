// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// One follow-up is one Worklist row: our email nobody answered and the promise
// task read out of that email. The rows and the figures are read through the
// route's own wiring, over records the product's writers wrote.

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// unansweredPromise is our promise mail to a customer contact, the task read
// out of it, and a moment four days on, past the follow-up window.
type unansweredPromise struct {
	e       *integration.Env
	p       integration.PromiseThread
	company ids.CompanyID
	mail    ids.UUID
	task    ids.UUID
	asOf    time.Time
	clock   func() time.Time
}

func seedUnansweredPromise(t *testing.T) unansweredPromise {
	t.Helper()
	e := integration.Setup(t)
	p := e.SeedPromiseThread(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Kunde GmbH", &e.AdminUser))
	if _, err := e.Contacts.UpdateCompany(e.Admin(), company, contacts.UpdateCompanyInput{Lifecycle: new("customer")}); err != nil {
		t.Fatalf("making the account a customer: %v", err)
	}
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &p.Contact, CompanyID: &company, Source: provenance.RecordSourceManual,
	}); err != nil {
		t.Fatalf("employing the contact at the account: %v", err)
	}
	u := unansweredPromise{e: e, p: p, company: company, mail: ids.UUID(p.Claim.SourceActivityId), task: e.SeedPromiseTask(t, p)}
	u.asOf = p.SentAt.AddDate(0, 0, 4)
	u.clock = func() time.Time { return u.asOf }
	return u
}

func (u unansweredPromise) worklist(t *testing.T) crmcontracts.Worklist {
	t.Helper()
	page, err := newAttentionService(u.e.Pool, approvals.NewService(u.e.DB()), u.clock).
		Worklist(u.e.Admin(), "mine", "", ids.UUID{}, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	return page
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
	for _, count := range page.Counts {
		if count.Category == crmcontracts.WorklistCountCategoryTasks && count.Total != nil {
			return *count.Total
		}
	}
	return -1
}

func TestAnUnansweredPromiseIsOneWorklistRowUntilItsTaskIsCompleted(t *testing.T) {
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
	if done := readTask(t, u.e, u.task).IsDone; done != nil && *done {
		t.Error("setting the reminder aside completed the promise")
	}
}

// A conversation with a colleague at the same account keeps its own row. So
// does a task for the same contact that was not read out of the promise mail.
func TestAnotherConversationAtTheSameAccountStaysItsOwnRow(t *testing.T) {
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
