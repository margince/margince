// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// capturedEmployee is a contact capture made at company's domain, with the
// employment edge the ensure planted, before the address was a seat's.
func capturedEmployee(t *testing.T, e *integration.Env, address string) (ids.ContactID, ids.UUID) {
	t.Helper()
	contact := captureCounterparty(t, e, "Somebody", address)
	edge := e.WsScalar(t, `SELECT id::text FROM relationship WHERE contact_id = $1 AND kind = 'employment' AND archived_at IS NULL`, contact)
	id, err := ids.Parse(edge)
	if err != nil {
		t.Fatalf("the ensure planted no employment for %s: %v", address, err)
	}
	return contact, id
}

func edgeLive(t *testing.T, e *integration.Env, edge ids.UUID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = $1 AND archived_at IS NULL`, edge) == 1
}

func TestTheSeatEmploymentTaskRetiresOnlyMachineInferredEdgesOfSeatLoginsAndReplaysSafely(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Outside GmbH", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "outside.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	other, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Elsewhere AG", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the second company: %v", err)
	}
	seat, inferred := capturedEmployee(t, e, "newhire@outside.test")
	mailboxContact, mailbox := capturedEmployee(t, e, "colleague@outside.test")
	editedContact, edited := capturedEmployee(t, e, "edited@outside.test")
	clientContact, client := capturedEmployee(t, e, "client@outside.test")
	quiet, _ := capturedEmployee(t, e, "quiet@outside.test")
	tagged, _ := capturedEmployee(t, e, "tagged@outside.test")
	tags := collections.NewStore(e.DB())
	tag, err := tags.CreateTag(e.Admin(), "Key account", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tags.ApplyTag(e.Admin(), tag.ID, "contact", tagged.UUID); err != nil {
		t.Fatalf("tagging the contact: %v", err)
	}
	asserted, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &seat, CompanyID: new(ids.From[ids.CompanyKind](ids.UUID(other.Id))), Source: "manual",
	})
	if err != nil {
		t.Fatalf("asserting an employment by hand: %v", err)
	}
	if _, err := e.Contacts.UpdateRelationship(e.Admin(), edited, contacts.UpdateRelationshipInput{Role: new("Advisor")}); err != nil {
		t.Fatalf("editing the inferred edge by hand: %v", err)
	}
	// Two seats sign in at addresses capture met first. A third only has a
	// mailbox there, which proves the address but can change with the grant.
	e.WsExec(t, `UPDATE app_user SET email = 'newhire@outside.test' WHERE id = $1`, e.Rep3)
	e.WsExec(t, `UPDATE app_user SET email = 'edited@outside.test' WHERE id = $1`, e.Rep1)
	e.WsExec(t, `UPDATE app_user SET email = 'quiet@outside.test' WHERE id = $1`, e.Rep2)
	e.WsExec(t, `UPDATE app_user SET email = 'tagged@outside.test' WHERE id = $1`, e.AdminUser)
	connectSeatMailbox(t, e, e.Rep2, "colleague@outside.test", "shared")

	task := seatEmploymentDataTask()
	step := task.Steps[0]
	rollback := errors.New("roll the batch back")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()}); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("the batch before the rollback: %v", err)
	}
	if !edgeLive(t, e, inferred) {
		t.Fatal("a rolled-back batch still retired the edge")
	}

	task.Steps[0].Limit = 1
	runMachineAddressTask(ctx, t, e, task)

	if edgeLive(t, e, inferred) {
		t.Error("the employment capture inferred for a colleague's login address is still live")
	}
	for name, edge := range map[string]ids.UUID{
		"the hand-made edge": asserted.ID, "the hand-edited edge": edited, "a client's edge": client,
		"the edge of an address only a mailbox proves": mailbox,
	} {
		if !edgeLive(t, e, edge) {
			t.Errorf("%s was retired", name)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'relationship' AND action = 'archive' AND evidence ? 'data_task_run'`); got != 3 {
		t.Errorf("archive audit rows naming the run = %d, want the three inferred edges of seat logins nobody worked on", got)
	}
	// The contact capture minted for a seat login, with nothing a colleague
	// did on it, is archived. A seat contact whose edge a human made or edited,
	// a client and an address only a mailbox proves all stay.
	if contactLive(t, e, quiet) {
		t.Error("the contact capture minted for a colleague's login is still live")
	}
	for name, contact := range map[string]ids.ContactID{
		"the seat with a hand-made edge": seat, "the seat with a hand-edited edge": editedContact,
		"a client": clientContact, "an address only a mailbox proves": mailboxContact,
		"the seat a colleague tagged": tagged,
	} {
		if !contactLive(t, e, contact) {
			t.Errorf("%s was archived", name)
		}
	}
	replayChangesNothing(ctx, t, e, task.Steps[1])

	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = step.Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay retired %d edges again", out.Changed)
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
}

func contactLive(t *testing.T, e *integration.Env, contact ids.ContactID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NULL`, contact) == 1
}

// replayChangesNothing runs a step over the whole table again, a page at a time.
func replayChangesNothing(ctx context.Context, t *testing.T, e *integration.Env, step datatask.Step) {
	t.Helper()
	in := datatask.BatchIn{Limit: 2, RunID: ids.NewV7()}
	for {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = step.Batch(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if out.Changed != 0 {
			t.Fatalf("a replay of %s changed %d rows again", step.Name, out.Changed)
		}
		if out.Done {
			return
		}
		in.Cursor = out.Cursor
	}
}

// A seat login's contact stays when anybody but capture worked on it: a note a
// colleague wrote about it, a lead made from it, or a tag an agent gave it.
// The plain seat contact beside them is archived, so the step ran.
func TestTheSeatContactStepKeepsAContactSomebodyWorkedOn(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Outside GmbH", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "outside.test", IsPrimary: true}},
	}); err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	plain, _ := capturedEmployee(t, e, "plain@outside.test")
	noted, _ := capturedEmployee(t, e, "noted@outside.test")
	leadmade, _ := capturedEmployee(t, e, "leadmade@outside.test")
	agenttagged, _ := capturedEmployee(t, e, "agenttag@outside.test")

	note := "Met at the trade fair"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &note, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: noted.UUID}},
	}); err != nil {
		t.Fatalf("writing a note about the contact: %v", err)
	}
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Lead Made"), Source: "manual"})
	if err != nil {
		t.Fatalf("creating the lead: %v", err)
	}
	e.WsExec(t, `UPDATE lead SET from_contact_id = $1 WHERE id = $2`, leadmade, ids.UUID(lead.Id))
	tags := collections.NewStore(e.DB())
	tag, err := tags.CreateTag(e.Admin(), "Agent pick", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tags.ApplyTag(e.Admin(), tag.ID, "contact", agenttagged.UUID); err != nil {
		t.Fatalf("tagging the contact: %v", err)
	}
	// The tag is an agent's, which ColleagueWorkedOnSince does not count.
	e.WsExec(t, `UPDATE taggable SET assigned_by_kind = 'agent' WHERE entity_id = $1`, agenttagged.UUID)

	for user, address := range map[ids.UUID]string{
		e.Rep1: "plain@outside.test", e.Rep2: "noted@outside.test",
		e.Rep3: "leadmade@outside.test", e.AdminUser: "agenttag@outside.test",
	} {
		e.WsExec(t, `UPDATE app_user SET email = $2 WHERE id = $1`, user, address)
	}
	runMachineAddressTask(ctx, t, e, seatEmploymentDataTask())

	if contactLive(t, e, plain) {
		t.Fatal("the plain seat contact is still live, so the step did not run and this proves nothing")
	}
	for name, contact := range map[string]ids.ContactID{
		"the contact a colleague wrote a note about": noted,
		"the contact a lead was made from":           leadmade,
		"the contact an agent tagged":                agenttagged,
	} {
		if !contactLive(t, e, contact) {
			t.Errorf("%s was archived", name)
		}
	}
}
