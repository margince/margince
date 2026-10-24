// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A share request is put to whoever owns the contact now, and follows it when
// the contact changes hands.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reassignAsRep1 hands the held contact to another seat through the update
// handler the server mounts, keeping it private. Rep3 sits in another team, so
// Rep1 assigns across teams.
func reassignAsRep1(t *testing.T, e *integration.Env, held ids.ContactID, to ids.UUID) {
	t.Helper()
	acrossTeams := integration.RepPerms
	acrossTeams.RowScope = principal.RowScopeAll
	body := `{"owner_id":"` + to.String() + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/contacts/"+held.String(), strings.NewReader(body)).
		WithContext(e.As(e.Rep1, []ids.UUID{e.Team1}, acrossTeams))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newContactsHandlers(e.Pool).UpdateContact(rec, req, crmcontracts.Id(held.UUID), crmcontracts.UpdateContactParams{})
	if rec.Code != http.StatusOK {
		t.Fatalf("reassigning the contact answered %d: %s", rec.Code, rec.Body.String())
	}
}

func pendingRequestsFor(t *testing.T, e *integration.Env, held ids.ContactID, owner ids.UUID) int {
	t.Helper()
	return countIn(t, e, `SELECT count(*) FROM approval
		WHERE kind = 'contact_share' AND target_entity_id = $1 AND status = 'pending'
		  AND proposed_change->>'owner_id' = $2`, held, owner.String())
}

func TestARequestIsPutToTheOwnerTheContactHasWhenItIsFiled(t *testing.T) {
	e, held := heldByRep1(t)
	reassignAsRep1(t, e, held, e.Rep3)

	status, problem := createAsRep2(t, e)

	requireHeldAnswer(t, e, held, status, problem, true)
	if n := pendingRequestsFor(t, e, held, e.Rep3); n != 1 {
		t.Errorf("%d requests put to the new owner, want 1", n)
	}
	if n := pendingRequestsFor(t, e, held, e.Rep1); n != 0 {
		t.Errorf("%d requests put to the previous owner, want none", n)
	}
}

func TestReassigningAContactWithdrawsTheRequestPutToItsPreviousOwner(t *testing.T) {
	e, held := heldByRep1(t)
	createAsRep2(t, e)

	reassignAsRep1(t, e, held, e.Rep3)

	if n := pendingRequestsFor(t, e, held, e.Rep1); n != 0 {
		t.Errorf("%d requests still wait for the previous owner", n)
	}
	status, problem := createAsRep2(t, e)
	requireHeldAnswer(t, e, held, status, problem, true)
	if n := pendingRequestsFor(t, e, held, e.Rep3); n != 1 {
		t.Errorf("the next attempt put %d requests to the new owner, want 1", n)
	}
}

func TestAnOwnerWhoLeftIsNotAsked(t *testing.T) {
	e, held := heldByRep1(t)
	deactivate(t, e, e.Rep1)

	status, problem := createAsRep2(t, e)

	requireHeldAnswer(t, e, held, status, problem, false)
	if n := countIn(t, e, `SELECT count(*) FROM approval WHERE kind = 'contact_share'`); n != 0 {
		t.Errorf("%d requests put to a member who left", n)
	}
}

func TestAHandoverWithdrawsTheRequestsPutToTheMemberWhoLeft(t *testing.T) {
	e, held := heldByRep1(t)
	createAsRep2(t, e)
	deactivate(t, e, e.Rep1)

	if _, err := handoverSeamFor(e).HandOver(e.Admin(), e.Rep1, e.Rep3); err != nil {
		t.Fatalf("handing over: %v", err)
	}

	if n := pendingRequestsFor(t, e, held, e.Rep1); n != 0 {
		t.Errorf("%d requests still wait for the member who left", n)
	}
	if owner := ownerOf(t, e, "contact", held.UUID); owner != e.Rep3 {
		t.Fatalf("the handover left the contact with %s, want Rep3", owner)
	}
}

// A contact nobody owns is the workspace's to read, so the address is a plain
// duplicate the caller can open, and nobody is asked.
func TestAContactNobodyOwnsAnswersAsAnOrdinaryDuplicate(t *testing.T) {
	e := integration.Setup(t)
	if _, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Unowned", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: heldAddress, EmailType: "work", IsPrimary: true}},
	}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE contact SET owner_id = NULL`)

	status, problem := createAsRep2(t, e)

	if status != http.StatusConflict || problem["code"] != "duplicate_email" {
		t.Errorf("the create answered %d %v, want 409 duplicate_email", status, problem["code"])
	}
	if n := countIn(t, e, `SELECT count(*) FROM approval WHERE kind = 'contact_share'`); n != 0 {
		t.Errorf("%d requests for a contact nobody owns", n)
	}
}
