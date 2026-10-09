// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The follow-up reminder: a message the reader sent to a customer that nobody
// answered within the workspace's window. Each refusal sits beside the case it
// is told apart from, so a query that hides everything fails here too.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const readerAddress = "rep@ourco.test"

// customerAt writes a contact employed at a company in the given lifecycle.
func (e *loadEnv) customerAt(t *testing.T, lifecycle string) ids.UUID {
	t.Helper()
	contact, company := e.buyer(t), ids.NewV7()
	e.exec(t, `INSERT INTO company (id, display_name, owner_id, lifecycle, source, captured_by)
		VALUES ($1, 'Company A', $2, $3, 'seed', 'system')`, company, e.rep, lifecycle)
	e.exec(t, `INSERT INTO relationship (id, kind, contact_id, company_id, source, captured_by)
		VALUES ($1, 'employment', $2, $3, 'seed', 'system')`, ids.NewV7(), contact, company)
	return contact
}

// sentTo seeds one message to the contact, `ago` back. An empty from is the
// shape a send through the product or a captured Sent message has: the seat
// stamped as sender with no address. A from address is mail logged by hand.
func (e *loadEnv) sentTo(t *testing.T, contact ids.UUID, from string, ago time.Duration) (ids.UUID, string) {
	t.Helper()
	activity, thread := ids.NewV7(), "thread-"+ids.NewV7().String()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'the proposal', now() - $2::interval, $3, 'seed', 'system')`,
		activity, ago.String(), thread)
	if from == "" {
		e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, user_id)
			VALUES ($1, $2, 'from', $3)`, ids.NewV7(), activity, e.rep)
	} else {
		e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address)
			VALUES ($1, $2, 'from', $3)`, ids.NewV7(), activity, from)
	}
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activity, contact)
	return activity, thread
}

// followUpReader is the rep with read on every record kind the customer test
// asks about, less the ones named in without.
func (e *loadEnv) followUpReader(without ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"activity", "contact", "deal", "company", "lead", "relationship"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	for _, object := range without {
		delete(grants, object)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

// afterwards seeds a later activity with the contact: a reply on the thread
// when thread is set, or a call when it is not.
func (e *loadEnv) afterwards(t *testing.T, contact ids.UUID, kind, direction, thread string) {
	t.Helper()
	activity := ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, $2, NULLIF($3, ''), 'later', now() - interval '1 hour', NULLIF($4, ''), 'seed', 'system')`,
		activity, kind, direction, thread)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id)
		VALUES ($1, $2, 'contact', $3)`, ids.NewV7(), activity, contact)
}

func awaited(t *testing.T, s *Store, e *loadEnv, activity ids.UUID) bool {
	t.Helper()
	return awaitedAs(e.followUpReader(), t, s, activity)
}

func awaitedAs(ctx context.Context, t *testing.T, s *Store, activity ids.UUID) bool {
	t.Helper()
	rows, days, err := s.AwaitingReplies(ctx, time.Now())
	if err != nil {
		t.Fatalf("reading follow-ups: %v", err)
	}
	if days != 2 {
		t.Fatalf("follow-up window = %d days, want the default 2", days)
	}
	for _, row := range rows {
		if row.ActivityID == activity {
			return true
		}
	}
	return false
}

func TestAnUnansweredMessageToACustomerIsAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	due, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	fresh, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 20*time.Hour)

	if !awaited(t, s, e, due) {
		t.Error("a message the reader sent three days ago with no answer is not a follow-up")
	}
	if awaited(t, s, e, fresh) {
		t.Error("a message sent within the two-day window is already a follow-up")
	}
}

// A newer message to the same contact settles the older one: the newer one
// is the follow-up the reader is now waiting on.
func TestAReplyOrACallSettlesTheFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)

	replied := e.customerAt(t, "prospect")
	answered, thread := e.sentTo(t, replied, "", 3*24*time.Hour)
	e.afterwards(t, replied, "email", "inbound", thread)

	called := e.customerAt(t, "prospect")
	phoned, _ := e.sentTo(t, called, "", 3*24*time.Hour)
	e.afterwards(t, called, "call", "", "")

	still := e.customerAt(t, "prospect")
	open, _ := e.sentTo(t, still, "", 3*24*time.Hour)

	twice := e.customerAt(t, "prospect")
	older, _ := e.sentTo(t, twice, "", 5*24*time.Hour)
	newer, _ := e.sentTo(t, twice, "", 3*24*time.Hour)

	if awaited(t, s, e, answered) {
		t.Error("a message answered on its thread is still a follow-up")
	}
	if awaited(t, s, e, phoned) {
		t.Error("a message followed by a call with the contact is still a follow-up")
	}
	if !awaited(t, s, e, open) {
		t.Error("the unanswered message beside them is not a follow-up")
	}
	if awaited(t, s, e, older) || !awaited(t, s, e, newer) {
		t.Error("two unanswered messages to one contact did not leave only the newer as the follow-up")
	}
}

func TestOnlyTheReadersOwnMailToACustomerIsAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)

	mine, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	colleagues, _ := e.sentTo(t, e.customerAt(t, "customer"), "colleague@ourco.test", 3*24*time.Hour)

	stranger := e.buyer(t)
	notSales, _ := e.sentTo(t, stranger, "", 3*24*time.Hour)

	if !awaited(t, s, e, mine) {
		t.Error("the reader's own message to a customer is not a follow-up")
	}
	if awaited(t, s, e, colleagues) {
		t.Error("a colleague's message is on the reader's follow-ups")
	}
	if awaited(t, s, e, notSales) {
		t.Error("a message to a contact at no customer company and no lead is a follow-up")
	}
}

// Mail logged by hand names the sender's address rather than the seat.
func TestAMessageLoggedFromTheReadersAddressIsAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	logged, _ := e.sentTo(t, e.customerAt(t, "customer"), readerAddress, 3*24*time.Hour)
	if !awaited(t, s, e, logged) {
		t.Error("a message logged from the reader's own address is not a follow-up")
	}
}

// Whether a deal is open is the deal's business: a reader who may not read
// deals is not told one through a reminder.
func TestTheCustomerTestReadsOnlyRecordKindsTheReaderMayRead(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	contact, pipeline, stage, deal := e.buyer(t), ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Deal A', 'open', $2, $3, $4, 'seed', 'system')`, deal, e.rep, pipeline, stage)
	sent, _ := e.sentTo(t, contact, "", 3*24*time.Hour)
	e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id)
		VALUES ($1, $2, 'deal', $3)`, ids.NewV7(), sent, deal)

	if !awaitedAs(e.followUpReader(), t, s, sent) {
		t.Fatal("a message filed under an open deal is not a follow-up for a reader who may read deals")
	}
	if awaitedAs(e.followUpReader("deal"), t, s, sent) {
		t.Error("a reader who may not read deals learned through a reminder that a deal is open")
	}
}

// A thread key is a header the sender chose. A stranger's message carrying
// it must not settle a reminder about somebody else.
func TestAStrangerOnTheThreadDoesNotSettleTheFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	customer := e.customerAt(t, "customer")
	sent, thread := e.sentTo(t, customer, "", 3*24*time.Hour)
	e.afterwards(t, e.customerAt(t, "customer"), "email", "inbound", thread)

	if !awaited(t, s, e, sent) {
		t.Error("a message from another customer on the same thread key settled the follow-up")
	}
}
