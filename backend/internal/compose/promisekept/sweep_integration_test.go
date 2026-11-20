// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package promisekept_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promise is one "I will invite <guest>" promise on a customer, with a later
// meeting the guest accepted.
type promise struct {
	claim, source, meeting, holder, guest, task ids.UUID
}

// seedPromise files the promise through the real writers. The meeting is
// readable by everyone, or by one seat only when onlyFor names it.
func seedPromise(t *testing.T, e *integration.Env, tag string, onlyFor *ids.UUID) promise {
	t.Helper()
	for _, seat := range []ids.UUID{e.Rep1, e.Rep2} {
		e.GrantRole(t, seat, "rep")
	}
	p := promise{
		holder: e.SeedContact(t, "Casey "+tag, nil), guest: e.SeedContact(t, "Riley "+tag+"son", nil),
		meeting: ids.NewV7(),
	}
	company := e.SeedCompany(t, "Acme "+tag, nil)
	for _, contact := range []ids.UUID{p.holder, p.guest} {
		e.WsExec(t, `INSERT INTO relationship (contact_id, company_id, kind, is_current_primary, source, captured_by)
			VALUES ($1, $2, 'employment', true, 'manual', 'human:test')`, contact, company)
	}
	p.source = e.LogThreadMail(e.Admin(), t, ids.From[ids.ContactKind](p.holder), "thread-"+tag, tag+"@acme.example", "outbound", clocktest.Now(t).Add(-72*time.Hour))
	name := "Riley " + tag + "son"
	claim := e.RecordClaim(e.Admin(), t, contacts.ClaimInput{
		ContactID: ids.From[ids.ContactKind](p.holder), Kind: "commitment_ours", Body: "I will invite " + name + " to the demo",
		ActivityID: p.source, Quote: "I will invite " + name, Source: "manual",
	})
	p.claim = ids.UUID(claim.Id)
	audience := "workspace"
	if onlyFor != nil {
		audience = "participants"
	}
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at, meeting_status, audience)
		VALUES ($1, 'meeting', 'Demo', 'manual', 'human:test', now() + interval '2 days', 'booked', $2)`, p.meeting, audience)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role, response) VALUES ($1, $2, 'attendee', 'accepted')`, p.meeting, p.guest)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, 'attendee')`, p.meeting, p.holder)
	if onlyFor != nil {
		e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, 'attendee')`, p.meeting, *onlyFor)
	}
	return p
}

// assignTask makes seat the promise's task assignee, as accepting the offer does.
func (p *promise) assignTask(t *testing.T, e *integration.Env, seat ids.UUID) {
	t.Helper()
	task := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, source, captured_by, occurred_at, assignee_id)
		VALUES ($1, 'task', 'Invite', 'manual', 'human:test', now(), $2)`, task, seat)
	e.WsExec(t, `UPDATE conversation_claim SET task_activity_id = $1 WHERE id = $2`, task, p.claim)
	p.task = task
}

func (p promise) reassign(t *testing.T, e *integration.Env, seat ids.UUID) {
	t.Helper()
	e.WsExec(t, `UPDATE activity SET assignee_id = $1 WHERE id = (SELECT task_activity_id FROM conversation_claim WHERE id = $2)`, seat, p.claim)
}

func (p promise) status(t *testing.T, e *integration.Env) string {
	t.Helper()
	return e.WsScalar(t, `SELECT status FROM conversation_claim WHERE id = $1`, p.claim)
}

func sweep(t *testing.T, e *integration.Env) {
	t.Helper()
	if err := compose.NewPromiseKeptSweep(e.Pool).Workspace(t.Context(), e.WS); err != nil {
		t.Fatalf("sweep: %v", err)
	}
}

func TestTheSweepClosesThePromiseOfItsTaskAssignee(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromise(t, e, "a", nil)
	p.assignTask(t, e, e.Rep1)
	sweep(t, e)
	if got := p.status(t, e); got != "done" {
		t.Fatalf("status = %q, want done", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE after->>'claim_id' = $1 AND actor_id = $2`, p.claim.String(), contacts.PromiseKeptActor); got != 1 {
		t.Errorf("system audit rows = %d, want 1", got)
	}
}

func TestAManagerWhoRecordedAColleaguesPromiseIsNotItsOwner(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromise(t, e, "b", nil)
	sweep(t, e)
	if got := p.status(t, e); got != "open" {
		t.Errorf("the recorder of an unowned promise became its owner: %q", got)
	}
}

func TestAReassignedTaskIsJudgedByItsCurrentOwner(t *testing.T) {
	e := integration.Setup(t)
	p := seedPromise(t, e, "c", &e.Rep1)
	p.assignTask(t, e, e.Rep1)
	p.reassign(t, e, e.Rep2)
	sweep(t, e)
	if got := p.status(t, e); got != "open" {
		t.Errorf("the former owner's evidence closed the promise of %s: %q", "Rep2", got)
	}
	p.reassign(t, e, e.Rep1)
	sweep(t, e)
	if got := p.status(t, e); got != "done" {
		t.Errorf("the owner who may open the meeting left it %q", got)
	}
}

func TestEachOwnerIsJudgedByWhatTheyThemselvesMayRead(t *testing.T) {
	e := integration.Setup(t)
	mine := seedPromise(t, e, "d", &e.Rep1)
	mine.assignTask(t, e, e.Rep1)
	theirs := seedPromise(t, e, "e", &e.Rep1)
	theirs.assignTask(t, e, e.Rep2)
	sweep(t, e)
	if got := mine.status(t, e); got != "done" {
		t.Errorf("the owner who reads the meeting: %q, want done", got)
	}
	if got := theirs.status(t, e); got != "open" {
		t.Errorf("the owner who cannot read the meeting: %q, want open", got)
	}
}

func TestASharedMailboxPromiseGoesToTheCustomersOwner(t *testing.T) {
	e := integration.Setup(t)
	box := integration.ConnectSharedMailbox(t, integration.SharedMailboxRegistry(t, e.Pool, "imap"), e.WS, integration.SharedMailbox{
		Provider: "imap", Address: "ops@fable.test", ReaderTeam: e.Team1, ConnectedBy: e.AdminUser, Posture: "held",
	})
	p := seedPromise(t, e, "f", nil)
	sweep(t, e)
	if got := p.status(t, e); got != "open" {
		t.Fatalf("before the mailbox imported the mail nobody owns the promise, yet it is %q", got)
	}
	e.WsExec(t, `UPDATE contact SET owner_id = $1 WHERE id = $2`, e.Rep1, p.holder)
	e.WsExec(t, `INSERT INTO capture_import(activity_id, connection_id, posture_at_import) VALUES ($1, $2, 'held')`, p.source, box)
	sweep(t, e)
	if got := p.status(t, e); got != "done" {
		t.Errorf("a promise sent from a shared mailbox, owned by its customer's owner: %q, want done", got)
	}
}
