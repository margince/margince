// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// qualifyReminder is the reminder task one lead carries, as stored.
type qualifyReminder struct {
	count    int
	assignee *ids.UUID
	open     bool
	origin   string
}

func readQualifyReminder(t *testing.T, e *integration.Env, lead ids.UUID) qualifyReminder {
	t.Helper()
	var r qualifyReminder
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(*), min(a.assignee_id::text)::uuid, COALESCE(bool_or(NOT a.is_done), false),
			       COALESCE(min(a.origin), '')
			FROM activity a JOIN activity_link l ON l.activity_id = a.id
			WHERE a.kind = 'task' AND a.source_system = $1 AND l.lead_id = $2`,
			contacts.QualifyReminderSource, lead).Scan(&r.count, &r.assignee, &r.open, &r.origin)
	}); err != nil {
		t.Fatalf("reading the reminder for lead %s: %v", lead, err)
	}
	return r
}

// A lead worked from a contact that nobody qualified or closed within the
// reminder age gets one task for its owner, once; a younger lead, or one not
// worked from a contact, gets none.
func TestALeadWorkedFromAContactIsRemindedOnceWhenLeftUnqualified(t *testing.T) {
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Dana Example", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	owner := ids.From[ids.UserKind](e.AdminUser)
	worked, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		Source: "manual", FromContactID: &contactID, OwnerID: &owner,
	})
	if err != nil {
		t.Fatalf("working the contact as a lead: %v", err)
	}
	name, email := "Erik Example", "erik@contoso.example"
	typed, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FullName: &name, Email: &email})
	if err != nil {
		t.Fatalf("creating a typed lead: %v", err)
	}
	log := slog.New(slog.DiscardHandler)
	scanAt := func(after time.Duration) {
		t.Helper()
		if err := remindUnqualifiedLeads(e.Admin(), db, func() time.Time { return time.Now().Add(after) }, log); err != nil {
			t.Fatalf("the reminder pass %s ahead: %v", after, err)
		}
	}

	scanAt(24 * time.Hour)
	if got := readQualifyReminder(t, e, ids.UUID(worked.Id)); got.count != 0 {
		t.Fatalf("a lead one day old was reminded %d times, want none", got.count)
	}

	scanAt(leadQualifyReminderAge + 24*time.Hour)
	scanAt(leadQualifyReminderAge + 48*time.Hour)
	got := readQualifyReminder(t, e, ids.UUID(worked.Id))
	if got.count != 1 || got.assignee == nil || *got.assignee != e.AdminUser || !got.open {
		t.Fatalf("the overdue lead carries %+v, want one open reminder for its owner", got)
	}
	// Work the product files about the lead, not a touch of it: the reminder
	// must not read as the lead's latest activity.
	if got.origin != activities.OriginSystemRemediation {
		t.Errorf("the reminder's origin is %q, want %q", got.origin, activities.OriginSystemRemediation)
	}
	if typed := readQualifyReminder(t, e, ids.UUID(typed.Id)); typed.count != 0 {
		t.Errorf("a lead not worked from a contact was reminded %d times, want none", typed.count)
	}
	// The scan itself stops offering a reminded lead; the task's idempotent
	// write alone would hide a scan that offered it on every pass.
	stillDue, err := contacts.NewStore(db).LeadsDueQualifyReminder(e.Admin(), time.Now().Add(leadQualifyReminderAge))
	if err != nil {
		t.Fatalf("listing the leads still due: %v", err)
	}
	for _, d := range stillDue {
		if d.UUID == ids.UUID(worked.Id) {
			t.Error("the scan still offers a lead it already reminded")
		}
	}

	// The follow-up that answers the reminder closes it: the auto-resolve
	// workflows complete the lead's open system tasks, and this one is one.
	if _, err := activities.NewStore(db).CompleteOpenSystemTasksForLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(worked.Id))); err != nil {
		t.Fatalf("completing the lead's system tasks: %v", err)
	}
	if got := readQualifyReminder(t, e, ids.UUID(worked.Id)); got.open {
		t.Error("the reminder stayed open after the lead's system tasks were completed; it was not written as a system task")
	}
	scanAt(leadQualifyReminderAge + 72*time.Hour)
	if got := readQualifyReminder(t, e, ids.UUID(worked.Id)); got.count != 1 {
		t.Errorf("a lead whose reminder was done was reminded again: %d tasks", got.count)
	}
}

// The clock-trigger pass is what runs the reminder in production: a lead
// created more than the reminder age ago is reminded by one ordinary pass.
func TestTheTimeScanPassRemindsAnOverdueLead(t *testing.T) {
	e := integration.Setup(t)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Dana Example", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FromContactID: &contactID})
	if err != nil {
		t.Fatalf("working the contact as a lead: %v", err)
	}
	// The pass reads the wall clock, so the lead is made old rather than the
	// clock moved.
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE lead SET created_at = $2 WHERE id = $1`,
			lead.Id, time.Now().Add(-leadQualifyReminderAge-time.Hour))
		return err
	}); err != nil {
		t.Fatalf("ageing the lead: %v", err)
	}

	worker := &timeScanWorker{pool: e.Pool, log: slog.New(slog.DiscardHandler)}
	if err := worker.scanWorkspace(context.Background(), e.WS); err != nil {
		t.Fatalf("the time scan pass: %v", err)
	}
	if got := readQualifyReminder(t, e, ids.UUID(lead.Id)); got.count != 1 {
		t.Fatalf("the time scan pass left %d reminders on an overdue lead, want one", got.count)
	}
}

// A lead merge carries the loser's reminder onto the survivor, and the
// survivor then counts as reminded: it is not handed a second task.
func TestAMergedLeadIsNotRemindedTwice(t *testing.T) {
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Dana Example", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
	worked, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "manual", FromContactID: &contactID})
	if err != nil {
		t.Fatalf("working the contact as a lead: %v", err)
	}
	name, email := "Dana Example", "dana@contoso.example"
	survivor, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{Source: "import", FullName: &name, Email: &email})
	if err != nil {
		t.Fatalf("creating the surviving lead: %v", err)
	}
	log := slog.New(slog.DiscardHandler)
	overdue := func() time.Time { return time.Now().Add(leadQualifyReminderAge + 24*time.Hour) }
	if err := remindUnqualifiedLeads(e.Admin(), db, overdue, log); err != nil {
		t.Fatalf("the first reminder pass: %v", err)
	}

	if _, err := e.Contacts.MergeLead(e.Admin(),
		ids.From[ids.LeadKind](ids.UUID(worked.Id)), ids.From[ids.LeadKind](ids.UUID(survivor.Id))); err != nil {
		t.Fatalf("merging the leads: %v", err)
	}
	if err := remindUnqualifiedLeads(e.Admin(), db, overdue, log); err != nil {
		t.Fatalf("the pass after the merge: %v", err)
	}

	if got := readQualifyReminder(t, e, ids.UUID(survivor.Id)); got.count != 1 {
		t.Errorf("the surviving lead carries %d reminders, want the one the merge carried over", got.count)
	}
}
