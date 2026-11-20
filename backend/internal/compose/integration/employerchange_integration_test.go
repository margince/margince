// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contact's mail from another company's domain becomes one question in the
// owner's inbox. Accepting it moves the employer and keeps the history;
// declining it is remembered for that company.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/employerchange"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var employerOwnerPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":      {Create: true, Read: true, Update: true},
		"company":      {Create: true, Read: true, Update: true},
		"relationship": {Create: true, Read: true, Update: true},
		"activity":     {Read: true},
	},
	RowScope: principal.RowScopeAll,
}

type employerChangeFixture struct {
	e            *Env
	ctx          context.Context
	store        *contacts.Store
	contact      ids.ContactID
	oldco, newco ids.CompanyID
	employedAt   time.Time
}

func setupEmployerChange(t *testing.T) employerChangeFixture {
	t.Helper()
	e := Setup(t)
	f := employerChangeFixture{e: e, ctx: e.As(e.Rep1, []ids.UUID{e.Team1}, employerOwnerPerms)}
	f.store = e.Contacts.WithEmployerChangeProposer(employerchange.Proposer{DB: e.DB()})
	f.oldco, f.newco = f.company(t, "Oldco", "oldco.test"), f.company(t, "Newco", "newco.test")
	created, err := f.store.CreateContact(f.ctx, contacts.CreateContactInput{
		FullName: "Ann Lee", Source: "manual",
		Emails: []contacts.ContactEmailInput{
			{Email: "ann@oldco.test", EmailType: "work", IsPrimary: true},
			{Email: "ann@newco.test", EmailType: "work"},
		},
	})
	if err != nil {
		t.Fatalf("creating Ann: %v", err)
	}
	f.contact = ids.From[ids.ContactKind](ids.UUID(created.Id))
	if err := f.store.ResolveContactEmployer(f.ctx, f.contact, contacts.EmployerChoice{CompanyID: &f.oldco, Source: "manual"}); err != nil {
		t.Fatalf("employing Ann at Oldco: %v", err)
	}
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT created_at FROM relationship WHERE contact_id = $1`, f.contact).Scan(&f.employedAt); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f employerChangeFixture) company(t *testing.T, name, domain string) ids.CompanyID {
	t.Helper()
	created, err := f.e.Contacts.CreateCompany(f.ctx, contacts.CreateCompanyInput{
		DisplayName: name, Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(created.Id))
}

// mailFromNewco captures one message Ann sent from her Newco address, seconds
// after her Oldco employment was recorded.
func (f employerChangeFixture) mailFromNewco(t *testing.T, seconds int) {
	t.Helper()
	activity := ids.New[ids.ActivityKind]()
	f.e.WsExec(t, `INSERT INTO activity (id, kind, subject, direction, occurred_at, source_system, source_id, source, captured_by)
		VALUES ($1, 'email', 'hello', 'inbound', $2, 'gmail', $3, 'gmail:seed', 'connector:gmail')`,
		activity, f.employedAt.Add(time.Duration(seconds)*time.Second), activity.String())
	f.e.WsExec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'from', 'ann@newco.test')`, activity)
	if _, err := f.store.EnsureCounterparty(f.ctx, contacts.EnsureCounterpartyInput{
		Email: "ann@newco.test", Domain: "newco.test", OwnerID: f.e.Rep1, ActivityID: activity,
		Source: "gmail:" + activity.String(), CapturedBy: "connector:gmail", Replied: true,
	}); err != nil {
		t.Fatalf("capturing Ann's mail: %v", err)
	}
}

func (f employerChangeFixture) questions(t *testing.T, status string) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = $1 AND status = $2`, employerchange.Kind, status)
}

func (f employerChangeFixture) question(t *testing.T) ids.ApprovalID {
	t.Helper()
	var id ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM approval WHERE kind = $1`, employerchange.Kind).Scan(&id); err != nil {
		t.Fatalf("reading the question: %v", err)
	}
	return ids.From[ids.ApprovalKind](id)
}

func TestASecondMailJoinsTheOpenQuestion(t *testing.T) {
	f := setupEmployerChange(t)
	f.mailFromNewco(t, 30)
	f.mailFromNewco(t, 60)
	if n := f.questions(t, "pending"); n != 1 {
		t.Fatalf("%d open questions after two mails, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = $1`, employerchange.Kind); n != 1 {
		t.Fatalf("%d questions in all after two mails, want the one joined", n)
	}
}

func TestAcceptingMovesTheEmployerAndKeepsTheHistory(t *testing.T) {
	f := setupEmployerChange(t)
	f.mailFromNewco(t, 30)
	if _, err := compose.NewApprovalsServiceWithEffects(f.e.Pool).Decide(f.ctx, f.question(t), true, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	var oldStatus, endedOn string
	var oldPrimary bool
	if err := OwnerConn(t).QueryRow(context.Background(), `
		SELECT employment_status, coalesce(ended_at::text, ''), is_current_primary FROM relationship
		 WHERE contact_id = $1 AND company_id = $2 AND archived_at IS NULL`, f.contact, f.oldco).Scan(&oldStatus, &endedOn, &oldPrimary); err != nil {
		t.Fatalf("reading the Oldco employment: %v", err)
	}
	if want := f.employedAt.Add(30 * time.Second).UTC().Format(time.DateOnly); oldStatus != "former" || endedOn != want || oldPrimary {
		t.Errorf("Oldco employment is %s, ended %q, primary %v; want former, ended %s, not primary", oldStatus, endedOn, oldPrimary, want)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2
		AND is_current_primary AND archived_at IS NULL AND ended_at IS NULL`, f.contact, f.newco); n != 1 {
		t.Errorf("%d current primary Newco employments, want 1", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'relationship'
		AND actor_id = 'human:' || $1`, f.e.Rep1.String()); n < 2 {
		t.Errorf("%d audited employment writes by the owner, want the start and the end", n)
	}
}

func TestADeclinedQuestionIsNotAskedAgain(t *testing.T) {
	f := setupEmployerChange(t)
	f.mailFromNewco(t, 30)
	if _, err := compose.NewApprovalsServiceWithEffects(f.e.Pool).Decide(f.ctx, f.question(t), false, nil); err != nil {
		t.Fatalf("declining: %v", err)
	}
	f.mailFromNewco(t, 60)
	if n := f.questions(t, "pending"); n != 0 {
		t.Fatalf("%d open questions after a declined one and a new mail, want none", n)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2`, f.contact, f.newco); n != 0 {
		t.Fatalf("%d Newco employments after declining, want none", n)
	}
}

// The question carries the employment it would end. When a colleague moved
// the contact since, accepting writes nothing.
func TestAcceptingAfterTheEmployerChangedWritesNothing(t *testing.T) {
	f := setupEmployerChange(t)
	f.mailFromNewco(t, 30)
	var oldEdge ids.UUID
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT id FROM relationship WHERE contact_id = $1`, f.contact).Scan(&oldEdge); err != nil {
		t.Fatal(err)
	}
	former := "former"
	if _, err := f.store.UpdateRelationship(f.ctx, oldEdge, contacts.UpdateRelationshipInput{EmploymentStatus: &former}); err != nil {
		t.Fatalf("ending Ann's Oldco employment by hand: %v", err)
	}
	thirdco := f.company(t, "Thirdco", "thirdco.test")
	if err := f.store.ResolveContactEmployer(f.ctx, f.contact, contacts.EmployerChoice{CompanyID: &thirdco, Source: "manual"}); err != nil {
		t.Fatalf("employing Ann at Thirdco by hand: %v", err)
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2 AND is_current_primary`, f.contact, thirdco); n != 1 {
		t.Fatalf("Ann has %d current primary Thirdco employments, want the one a colleague set", n)
	}
	_, err := compose.NewApprovalsServiceWithEffects(f.e.Pool).Decide(f.ctx, f.question(t), true, nil)
	if err == nil {
		t.Fatal("accepting a stale question succeeded, want it refused as no longer needed")
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2`, f.contact, f.newco); n != 0 {
		t.Fatalf("%d Newco employments after a stale accept, want none", n)
	}
}

// A departed owner cannot be asked, and nobody else is asked in their place.
func TestADepartedOwnerIsAskedNothingAndNobodyElseIs(t *testing.T) {
	f := setupEmployerChange(t)
	f.e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, f.e.Rep1)
	f.mailFromNewco(t, 30)
	if n := f.e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = $1`, employerchange.Kind); n != 0 {
		t.Fatalf("%d questions with a departed owner, want none", n)
	}
}

// The question is answered as it stands: its day becomes the end date.
func TestAnEditedAnswerIsRefused(t *testing.T) {
	f := setupEmployerChange(t)
	f.mailFromNewco(t, 30)
	var raw []byte
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT proposed_change FROM approval WHERE kind = $1`, employerchange.Kind).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var change approvalsubject.EmployerChange
	if err := json.Unmarshal(raw, &change); err != nil {
		t.Fatal(err)
	}
	change.ObservedOn = "2001-01-01"
	edited, err := json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compose.NewApprovalsServiceWithEffects(f.e.Pool).DecideEdited(f.ctx, f.question(t), edited); err == nil {
		t.Fatal("an answer with a retyped end date was accepted")
	}
	if n := f.e.WsCount(t, `SELECT count(*) FROM relationship WHERE contact_id = $1 AND company_id = $2`, f.contact, f.newco); n != 0 {
		t.Fatalf("%d Newco employments after a refused answer, want none", n)
	}
}
