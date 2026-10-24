// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A colleague creates a person whose contact capture is keeping to its mailbox
// owner. Each test captures the mail through the real sink, so the contact is
// owner-scoped the way production leaves it, and drives a create door the
// server mounts.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const heldPerson = "person@company-a.example"

// heldByRep1 captures one exchange with the person into Rep1's mailbox and
// returns the contact it left private to Rep1.
func heldByRep1(t *testing.T) (*integration.Env, ids.ContactID) {
	t.Helper()
	e := setupWithOwnMailbox(t)
	seedAttestedOutbound(t, e, "held-out", heldPerson, "thr-held-person")
	captureInboundThroughRealSink(t, e, e.Rep1, "held-in", heldPerson, "thr-held-person")
	requireNarrowed(t, e, heldPerson, contacts.NarrowedAwaitingVerdict)
	return e, contactIDFor(t, e, heldPerson)
}

func rep2(e *integration.Env) context.Context {
	return e.As(e.Rep2, []ids.UUID{e.Team1}, integration.RepPerms)
}

// createAsRep2 posts the issue's create through the handlers the server mounts
// and returns the status and the problem body.
func createAsRep2(t *testing.T, e *integration.Env) (int, map[string]any) {
	t.Helper()
	body := `{"full_name":"Pat Example","source":"manual","emails":[{"email":"` + heldPerson + `","email_type":"work","is_primary":true}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/contacts", strings.NewReader(body)).WithContext(rep2(e))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	newContactsHandlers(e.Pool).CreateContact(rec, req, crmcontracts.CreateContactParams{})
	var problem map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("reading the answer %q: %v", rec.Body.String(), err)
	}
	return rec.Code, problem
}

// requireHeldAnswer checks the 409 names no record and says whether Rep1 was
// asked.
func requireHeldAnswer(t *testing.T, e *integration.Env, held ids.ContactID, status int, problem map[string]any, asked bool) {
	t.Helper()
	if status != http.StatusConflict || problem["code"] != "contact_held_by_colleague" {
		t.Fatalf("the create answered %d %v, want 409 contact_held_by_colleague", status, problem["code"])
	}
	details, _ := problem["details"].(map[string]any)
	if details["share_requested"] != asked {
		t.Errorf("share_requested = %v, want %v", details["share_requested"], asked)
	}
	raw, err := json.Marshal(problem)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{held.String(), e.Rep1.String(), "existing_id"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("the refusal carries %q: %s", leaked, raw)
		}
	}
}

// shareRequests counts the requests filed for the held contact, by status.
func shareRequests(t *testing.T, e *integration.Env, held ids.ContactID, status string) int {
	t.Helper()
	return countIn(t, e, `SELECT count(*) FROM approval
		WHERE kind = 'contact_share' AND target_entity_id = $1 AND status = '`+status+`'
		  AND proposed_change->>'owner_id' = '`+e.Rep1.String()+`'
		  AND proposed_change->>'requester_id' = '`+e.Rep2.String()+`'`, held)
}

func openShareRequest(t *testing.T, e *integration.Env, held ids.ContactID) ids.ApprovalID {
	t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT id FROM approval
			WHERE kind = 'contact_share' AND target_entity_id = $1 AND status = 'pending'`, held).Scan(&id)
	}); err != nil {
		t.Fatalf("reading the open share request: %v", err)
	}
	return ids.From[ids.ApprovalKind](id)
}

func shareRequestsInInbox(t *testing.T, e *integration.Env, reader context.Context) int {
	t.Helper()
	kind := contactShareKind
	rows, _, err := approvalsServiceWithEffects(e.Pool).ListWire(reader, approvals.ListInput{Kind: &kind})
	if err != nil {
		t.Fatalf("reading the Review queue: %v", err)
	}
	return len(rows)
}

func decideAsRep1(t *testing.T, e *integration.Env, request ids.ApprovalID, approve bool) {
	t.Helper()
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	if _, err := approvalsServiceWithEffects(e.Pool).Decide(rep1, request, approve, nil); err != nil {
		t.Fatalf("Rep1 deciding the share request (approve=%v): %v", approve, err)
	}
}

func TestACreateOfAColleaguesCapturedContactAsksThemOnce(t *testing.T) {
	e, held := heldByRep1(t)

	status, problem := createAsRep2(t, e)
	requireHeldAnswer(t, e, held, status, problem, true)
	if n := shareRequests(t, e, held, "pending"); n != 1 {
		t.Fatalf("%d open share requests for Rep1, want 1", n)
	}
	if n := shareRequestsInInbox(t, e, e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)); n != 1 {
		t.Errorf("Rep1's Review queue shows %d share requests, want 1", n)
	}
	if n := shareRequestsInInbox(t, e, rep2(e)); n != 0 {
		t.Errorf("Rep2's Review queue shows %d share requests about a contact they cannot see", n)
	}

	status, problem = createAsRep2(t, e)
	requireHeldAnswer(t, e, held, status, problem, true)
	if n := countIn(t, e, `SELECT count(*) FROM approval WHERE kind = 'contact_share'`); n != 1 {
		t.Errorf("a second attempt left %d share requests, want 1", n)
	}
	if n := countIn(t, e, `SELECT count(*) FROM contact_email WHERE email = $1`, heldPerson); n != 1 {
		t.Errorf("%d contacts hold the address, want the one Rep1 holds", n)
	}
}

func TestADeclinedShareRequestIsNotFiledAgain(t *testing.T) {
	e, held := heldByRep1(t)
	createAsRep2(t, e)
	decideAsRep1(t, e, openShareRequest(t, e, held), false)

	status, problem := createAsRep2(t, e)

	requireHeldAnswer(t, e, held, status, problem, false)
	if n := countIn(t, e, `SELECT count(*) FROM approval WHERE kind = 'contact_share'`); n != 1 {
		t.Errorf("after the decline %d share requests exist, want the declined one", n)
	}
}

func TestAnAcceptedShareRequestLetsTheColleagueEditTheContact(t *testing.T) {
	e, held := heldByRep1(t)
	createAsRep2(t, e)

	decideAsRep1(t, e, openShareRequest(t, e, held), true)

	title := "Head of Purchasing"
	if _, err := e.Contacts.UpdateContact(rep2(e), held, contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Errorf("Rep2 editing the shared contact: %v", err)
	}
	rep3 := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.RepPerms)
	if _, err := e.Contacts.GetContact(rep3, held, storekit.LiveOnly); err == nil {
		t.Error("a third seat can read a contact shared with Rep2 alone")
	}
	if visibility, _ := contactVisibility(t, e, heldPerson); visibility != "owner" {
		t.Errorf("accepting a share changed the contact's visibility to %q", visibility)
	}
}

// The request was filed while Rep2 held a full seat; by the time Rep1 accepts,
// the seat may only read.
func TestAShareAcceptedForAReadSeatGrantsRead(t *testing.T) {
	e, held := heldByRep1(t)
	createAsRep2(t, e)
	e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, e.Rep2)

	decideAsRep1(t, e, openShareRequest(t, e, held), true)

	if n := countIn(t, e, `SELECT count(*) FROM record_grant
		WHERE record_type = 'contact' AND record_id = $1 AND subject_id = $2 AND access = 'read'`, held, e.Rep2); n != 1 {
		t.Errorf("%d read grants for the read seat, want 1", n)
	}
}

// Every other door that creates a contact gives the same refusal and asks the
// same way.
func TestEveryCreateDoorAsksTheColleagueWhoHoldsTheAddress(t *testing.T) {
	doors := map[string]func(t *testing.T, e *integration.Env) error{
		"quick capture": func(t *testing.T, e *integration.Env) error {
			req := httptest.NewRequest(http.MethodPost, "/v1/contacts/quick-capture",
				strings.NewReader(`{"full_name":"Pat Example","email":"`+heldPerson+`"}`)).WithContext(rep2(e))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			newContactsHandlers(e.Pool).QuickCaptureContact(rec, req, crmcontracts.QuickCaptureContactParams{})
			if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "contact_held_by_colleague") {
				t.Errorf("quick capture answered %d %s, want 409 contact_held_by_colleague", rec.Code, rec.Body.String())
			}
			return nil
		},
		"agent": func(t *testing.T, e *integration.Env) error {
			agent := e.AgentFor(t, e.Rep2, []ids.UUID{e.Team1}, integration.RepPerms)
			_, err := NewProviderFor(e.DB()).Create(agent, datasource.CreateInput{
				EntityType: datasource.EntityContact, Source: "agent",
				Fields: map[string]any{"full_name": "Pat Example", "emails": []map[string]any{
					{"email": heldPerson, "email_type": "work", "is_primary": true}}},
			})
			return err
		},
		"csv import": func(t *testing.T, e *integration.Env) error {
			w := newCSVWriters(e.DB(), ids.NewV7(), migration.PartMapping{Object: migration.ObjectContact},
				runOptions{}, &linkResolver{sourceSystem: "csv:held"}, importColumns{})
			result, err := w.createContact(rep2(e), migration.Row{ExternalID: "row-1",
				Fields: map[string]any{"full_name": "Pat Example", fieldEmail: heldPerson}}, nil)
			if err == nil && (!result.Skipped || result.SkipWhy.Code != migration.ReasonContactEmailHeld) {
				t.Errorf("the import landed %+v, want the row skipped as held", result)
			}
			return err
		},
	}
	for name, door := range doors {
		t.Run(name, func(t *testing.T) {
			e, held := heldByRep1(t)
			err := door(t, e)
			if name != "agent" && err != nil {
				t.Fatalf("the %s door failed: %v", name, err)
			}
			if name == "agent" && !strings.Contains(errorText(err), "a colleague already has this person") {
				t.Errorf("the %s door answered %v, want the held refusal", name, err)
			}
			if n := shareRequests(t, e, held, "pending"); n != 1 {
				t.Errorf("the %s door filed %d share requests, want 1", name, n)
			}
		})
	}
}

// A connector creating for its mailbox is automatic work, and asks nobody.
func TestAutomaticWorkFilesNoShareRequest(t *testing.T) {
	e, held := heldByRep1(t)
	store := contacts.NewStore(e.DB()).WithShareRequests(contactShareRequests(e.Pool))

	_, err := store.CreateContact(mailboxOwnerCtx(e, e.Rep2), contacts.CreateContactInput{
		FullName: "Pat Example",
		Emails:   []contacts.ContactEmailInput{{Email: heldPerson, EmailType: "work", IsPrimary: true}},
	})
	if !strings.Contains(errorText(err), "a colleague already has this person") {
		t.Fatalf("a connector create answered %v, want the held refusal", err)
	}
	if n := countIn(t, e, `SELECT count(*) FROM approval WHERE kind = 'contact_share' AND target_entity_id = $1`, held); n != 0 {
		t.Errorf("a connector create filed %d share requests, want none", n)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
