// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A lead never arrives for an existing contact. Every lead door refuses. A
// refused arrival becomes one Worklist task for another owner.

import (
	"errors"
	"log/slog"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const heldLinkedIn = "https://www.linkedin.com/in/mara-held"

// leadForContactEnv is a contact owned by Rep2, and a contacts store that tells
// its owner through the production writer.
type leadForContactEnv struct {
	e       *integration.Env
	store   *contacts.Store
	contact ids.ContactID
	address string
}

func setupLeadForContact(t *testing.T) *leadForContactEnv {
	t.Helper()
	e := integration.Setup(t)
	l := &leadForContactEnv{
		e:       e,
		store:   e.Contacts.WithLeadArrivals(newLeadWorkTasks(InstallationDB(e.Pool), slog.Default())),
		address: "mara@customer.example",
	}
	owner := ids.From[ids.UserKind](e.Rep2)
	created, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Mara Held", Source: "manual", OwnerID: &owner,
		Emails: []contacts.ContactEmailInput{{Email: l.address, EmailType: "work", IsPrimary: true}},
		Social: map[string]any{"linkedin": heldLinkedIn},
	})
	if err != nil {
		t.Fatalf("creating the contact: %v", err)
	}
	l.contact = ids.From[ids.ContactKind](ids.UUID(created.Id))
	return l
}

// ownerTasks counts the open lead_for_contact tasks on the contact for Rep2.
func (l *leadForContactEnv) ownerTasks(t *testing.T) int {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link al ON al.activity_id = a.id
		WHERE al.contact_id = $1 AND a.kind = 'task' AND a.source_system = $2
		  AND NOT a.is_done AND a.assignee_id = $3`, l.contact, provenance.LeadForContactSource, l.e.Rep2)
}

func (l *leadForContactEnv) leads(t *testing.T) int {
	t.Helper()
	return l.e.WsCount(t, `SELECT count(*) FROM lead`)
}

func requireContactHeld(t *testing.T, err error, door string) *contacts.ContactHoldsIdentityError {
	t.Helper()
	held, ok := errors.AsType[*contacts.ContactHoldsIdentityError](err)
	if !ok {
		t.Fatalf("%s over a contact's key = %v, want the contact-held refusal", door, err)
	}
	return held
}

func TestALeadCreateForAContactIsRefusedAndTheOwnerTold(t *testing.T) {
	l := setupLeadForContact(t)
	rep1 := l.e.As(l.e.Rep1, nil, integration.AdminPerms)
	in := contacts.CreateLeadInput{FullName: new("Mara H."), Email: new("Mara@Customer.example"), Source: "web_form"}

	_, _, err := l.store.CreateLead(rep1, in)
	held := requireContactHeld(t, err, "a lead create")
	if held.ContactID != l.contact || !held.OwnerNotified || held.Field != "email" {
		t.Errorf("refusal = %+v, want it to name contact %v on email with its owner told", held, l.contact)
	}
	if _, _, err := l.store.CreateLead(rep1, in); err == nil {
		t.Fatal("the same submission again created a lead")
	}
	if n := l.ownerTasks(t); n != 1 {
		t.Errorf("one arrival sent twice left %d tasks for the contact's owner, want 1", n)
	}

	owner := l.e.As(l.e.Rep2, nil, integration.AdminPerms)
	other := contacts.CreateLeadInput{FullName: new("Mara Again"), Email: new(l.address), Source: "manual"}
	_, _, err = l.store.CreateLead(owner, other)
	if held := requireContactHeld(t, err, "the owner's own lead create"); held.OwnerNotified {
		t.Error("the contact's owner was told about a lead they typed themselves")
	}
	if n := l.ownerTasks(t); n != 1 {
		t.Errorf("the owner's own refused lead filed a task: %d open, want 1", n)
	}

	url := heldLinkedIn
	_, _, err = l.store.CreateLead(rep1, contacts.CreateLeadInput{FullName: new("Mara L."), LinkedInURL: &url, Source: "api"})
	if held := requireContactHeld(t, err, "a lead create by LinkedIn"); held.Field != "linkedin_url" {
		t.Errorf("a LinkedIn refusal names field %q, want linkedin_url", held.Field)
	}
	if n := l.leads(t); n != 0 {
		t.Errorf("%d leads exist for a contact, want none", n)
	}
}

func TestALeadEditOrReopenOntoAContactsAddressIsRefused(t *testing.T) {
	l := setupLeadForContact(t)
	admin := l.e.Admin()
	lead, _, err := l.store.CreateLead(admin, contacts.CreateLeadInput{FullName: new("Other"), Email: new("other@prospect.example"), Source: "manual"})
	if err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	_, err = l.store.UpdateLead(admin, id, contacts.UpdateLeadInput{Email: new(l.address)})
	if held := requireContactHeld(t, err, "a lead email edit"); held.Field != "email" {
		t.Errorf("the email edit refusal names field %q, want email", held.Field)
	}
	got, err := l.store.GetLead(admin, id, 0)
	if err != nil || got.Email == nil || string(*got.Email) != "other@prospect.example" {
		t.Errorf("the lead after a refused edit = %+v (err %v), want its own address", got.Email, err)
	}

	const later = "later@customer.example"
	dropped, _, err := l.store.CreateLead(admin, contacts.CreateLeadInput{FullName: new("Dropped"), Email: new(later), Source: "manual"})
	if err != nil {
		t.Fatalf("creating the second lead: %v", err)
	}
	droppedID := ids.From[ids.LeadKind](ids.UUID(dropped.Id))
	if _, err := l.store.DisqualifyLead(admin, droppedID, contacts.DisqualifyLeadInput{Note: new("No budget")}); err != nil {
		t.Fatalf("disqualifying: %v", err)
	}
	if _, err := l.e.Contacts.CreateContact(admin, contacts.CreateContactInput{
		FullName: "Later Contact", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: later, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("creating the contact for the closed lead's address: %v", err)
	}
	_, err = l.store.ReopenLead(admin, droppedID)
	if held := requireContactHeld(t, err, "a lead reopen"); held.Field != "email" {
		t.Errorf("the reopen refusal names field %q, want email", held.Field)
	}
}

func TestACSVLeadRowForAContactIsSkippedInThePreviewAndTheCommit(t *testing.T) {
	l := setupLeadForContact(t)
	ctx := importingAdmin(l.e)
	run, err := migration.NewRunStore(l.e.DB()).CreateDraftRun(ctx, migration.CreateDraftRunInput{
		SourceRef: "ws/import/" + ids.NewV7().String(), Source: "import_api", FileName: "leads.csv",
		Profile: []byte(`{"files":[{"name":"leads.csv","row_count":1}]}`),
		Mapping: migration.RunMapping{Object: migration.ObjectLead, Fields: map[string]string{"Email": "email"}, SourceKey: "Email"},
	})
	if err != nil {
		t.Fatalf("opening the import run: %v", err)
	}
	w := newCSVWriters(l.e.DB(), run.ID, migration.PartMapping{Object: migration.ObjectLead},
		runOptions{}, &linkResolver{sourceSystem: "csv:held"}, importColumns{})
	row := migration.Row{ExternalID: "lead-row-a", Fields: map[string]any{"full_name": "Mara Imported", fieldEmail: l.address}}

	_, why, err := w.predictCreatePath(ctx, row, nil)
	if err != nil || why.Code != migration.ReasonContactHoldsLead {
		t.Fatalf("preview = %v (err %v), want %s", why.Code, err, migration.ReasonContactHoldsLead)
	}
	for range 2 {
		result, err := w.createLead(ctx, row, nil)
		if err != nil || !result.Skipped || result.SkipWhy.Code != migration.ReasonContactHoldsLead {
			t.Fatalf("commit = %+v (err %v), want the row skipped as %s", result, err, migration.ReasonContactHoldsLead)
		}
	}
	if n := l.leads(t); n != 0 {
		t.Errorf("%d leads after importing a contact's address, want none", n)
	}
	if n := l.ownerTasks(t); n != 1 {
		t.Errorf("one import row run twice left %d tasks for the contact's owner, want 1", n)
	}
}

func TestACapturedLeadForAContactLandsOnTheContact(t *testing.T) {
	l := setupLeadForContact(t)
	ctx := threadConnectorCtx(l.e, l.e.Rep1)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	ctx = principal.WithActor(ctx, actor)
	rec := connector.NormalizedRecord{
		EntityType: datasource.EntityLead,
		NaturalKey: connector.NaturalKey{SourceSystem: "formsapp", SourceID: "submission-1"},
		Fields:     capture.LeadFields{FullName: "Mara Form", Email: l.address, Title: "Buyer"},
		Source:     "formsapp", CapturedBy: "connector:gmail", Raw: []byte(`{}`),
	}
	ref, err := newCaptureSink(l.e.Pool, CaptureConfig{}).Upsert(ctx, rec)
	if err != nil {
		t.Fatalf("capturing the lead: %v", err)
	}
	if ref.Type != datasource.EntityContact || ref.ID != l.contact.UUID {
		t.Errorf("the capture answered %+v, want the contact %v", ref, l.contact)
	}
	if n := l.leads(t); n != 0 {
		t.Errorf("%d leads after capturing a contact's address, want none", n)
	}
	if n := l.ownerTasks(t); n != 1 {
		t.Errorf("the captured arrival left %d tasks for the contact's owner, want 1", n)
	}
}
