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

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
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
	return e.followUpReaderAs(e.rep, without...)
}

// followUpReaderAs is followUpReader for any seat.
func (e *loadEnv) followUpReaderAs(seat ids.UUID, without ...string) context.Context {
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
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
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

// An account in set-up or live is one we deliver for. Unanswered mail to it is
// owed a follow-up, and mail to an account that is over is not.
func TestUnansweredMailToAnAccountWeDeliverForIsAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	for stage, want := range map[string]bool{"onboarding": true, "live": true, "former_customer": false} {
		sent, _ := e.sentTo(t, e.customerAt(t, stage), "", 3*24*time.Hour)
		if got := awaited(t, s, e, sent); got != want {
			t.Errorf("unanswered mail to a %s account: follow-up = %v, want %v", stage, got, want)
		}
	}
}

// Without the company arm the statement carries no lifecycle list. A reader
// who may not read companies still gets an answer rather than an error.
func TestAReaderWhoMayNotReadCompaniesLearnsNoStageFromAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	sent, _ := e.sentTo(t, e.customerAt(t, "live"), "", 3*24*time.Hour)
	if awaitedAs(e.followUpReader("company"), t, s, sent) {
		t.Error("a reader who may not read companies learned through a reminder that an account is live")
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

// An import brings history that may sit inside the lookback. Nobody left a
// message unanswered in this CRM, so no reminder is about it.
func TestAnImportedMessageIsNotAFollowUp(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	customer := e.customerAt(t, "customer")
	live, _ := e.sentTo(t, customer, "", 3*24*time.Hour)
	imported, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	e.exec(t, `UPDATE activity SET source_system = 'mirror:csv:old-crm' WHERE id = $1`, imported)

	if !awaited(t, s, e, live) {
		t.Error("the reader's own live message is not a follow-up, so the test cannot tell an exclusion from an empty read")
	}
	if awaited(t, s, e, imported) {
		t.Error("a message an import brought in is a follow-up")
	}
}

// A day email logged by hand can be awaiting a reply, and its time is a date.
func TestAFollowUpCarriesTheRealPrecisionOfItsMessage(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	dayEmail, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)
	e.exec(t, `UPDATE activity SET occurred_precision = 'day' WHERE id = $1`, dayEmail)
	instantEmail, _ := e.sentTo(t, e.customerAt(t, "customer"), "", 3*24*time.Hour)

	rows, _, err := s.AwaitingReplies(e.followUpReader(), clocktest.Now(t))
	if err != nil {
		t.Fatalf("reading follow-ups: %v", err)
	}
	got := map[ids.UUID]string{}
	for _, row := range rows {
		got[row.ActivityID] = row.Precision
	}
	if p, ok := got[dayEmail]; !ok || p != PrecisionDay {
		t.Errorf("day email: listed=%v precision=%q, want listed with %q", ok, p, PrecisionDay)
	}
	if p, ok := got[instantEmail]; !ok || p != PrecisionInstant {
		t.Errorf("instant email: listed=%v precision=%q, want listed with %q", ok, p, PrecisionInstant)
	}
}

// An imported inbound message is history, so neither the badge nor the lane
// asks for a reply to it.
func TestAnImportedInboundIsNeverOwedAReply(t *testing.T) {
	e := setupLoad(t)
	contact := ids.NewV7()
	e.exec(t, `INSERT INTO contact (id, full_name, source, captured_by)
		VALUES ($1, 'Buyer Contact', 'seed', 'system')`, contact)
	activity := e.seedWait(t, "Can you send the quote?", "contact_id", contact)
	move := func() crmcontracts.EmailSummaryMove {
		tx, err := e.owner.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		states, err := EmailStatesFor(e.followUpReader(), tx, []ids.UUID{activity})
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(context.Background()); err != nil {
			t.Fatal(err)
		}
		return states[activity].Move
	}
	if got := move(); got != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("a live inbound's move = %q, want needs_reply", got)
	}
	e.exec(t, `UPDATE activity SET source_system = 'mirror:csv:old-crm' WHERE id = $1`, activity)
	if got := move(); got != crmcontracts.EmailSummaryMoveNone {
		t.Fatalf("an imported inbound's move = %q, want none", got)
	}
}

// The follow-up lane and the badge ask one question about a sent message. An
// invitation we sent is on neither; a plain unanswered mail is on both until
// the customer answers on its thread.
func TestTheFollowUpLaneAndTheBadgeAgreeOnASentMessage(t *testing.T) {
	e := setupLoad(t)
	s := storeAddressing(e, readerAddress)
	customer := e.customerAt(t, "customer")
	plain, thread := e.sentTo(t, customer, "", 3*24*time.Hour)
	invitation, _ := e.sentTo(t, customer, "", 3*24*time.Hour)
	setCalendarMethod(t, e, invitation, "REQUEST")

	moves := func() map[ids.UUID]crmcontracts.EmailSummaryMove {
		tx, err := e.owner.Begin(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := tx.Rollback(context.Background()); err != nil {
				t.Fatal(err)
			}
		}()
		states, err := EmailStatesFor(e.followUpReader(), tx, []ids.UUID{plain, invitation})
		if err != nil {
			t.Fatal(err)
		}
		return map[ids.UUID]crmcontracts.EmailSummaryMove{plain: states[plain].Move, invitation: states[invitation].Move}
	}
	got := moves()
	if !awaited(t, s, e, plain) || got[plain] != crmcontracts.EmailSummaryMoveWaitingForThem {
		t.Errorf("an unanswered plain mail: lane %v, move %q; want on the lane and waiting_for_them",
			awaited(t, s, e, plain), got[plain])
	}
	if awaited(t, s, e, invitation) || got[invitation] != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("our invitation: lane %v, move %q; want off the lane and none",
			awaited(t, s, e, invitation), got[invitation])
	}

	e.afterwards(t, customer, "email", "inbound", thread)
	if awaited(t, s, e, plain) || moves()[plain] != crmcontracts.EmailSummaryMoveNone {
		t.Error("the customer answered on the thread, yet the lane or the badge still waits on them")
	}
}

// seatAddresses gives each seat its own mail addresses.
type seatAddresses struct {
	fixedDomains
	bySeat map[ids.UUID][]string
}

func (s seatAddresses) ReaderAddresses(_ context.Context, _ pgx.Tx, reader ids.UUID) ([]string, error) {
	return s.bySeat[reader], nil
}

// copiedMail seeds a mail the colleague sent to the customer, as the reader's
// mailbox holds it. Each extra participant row is a role and a seat or address.
func (e *loadEnv) copiedMail(t *testing.T, customer ids.UUID, colleagueAddress string, participants ...[3]any) ids.UUID {
	t.Helper()
	mail, _ := e.sentTo(t, customer, colleagueAddress, 3*24*time.Hour)
	for _, p := range participants {
		e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, user_id, address)
			VALUES ($1, $2, $3, $4, $5)`, ids.NewV7(), mail, p[0], p[1], p[2])
	}
	return mail
}

func (e *loadEnv) addressedSeats(colleagueAddress string, shared ...string) *Store {
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).WithOwnDomains(seatAddresses{bySeat: map[ids.UUID][]string{
		e.rep: append([]string{readerAddress}, shared...), e.other: append([]string{colleagueAddress}, shared...),
	}})
}

// A colleague's mail that names the reader on To or CC is the colleague's to
// follow up, never the reader's.
func TestAColleaguesMailToTheReaderIsTheColleaguesFollowUp(t *testing.T) {
	e := setupLoad(t)
	colleagueAddress := "colleague-" + e.other.String() + "@ourco.test"
	s := e.addressedSeats(colleagueAddress)
	// One customer each: a later mail to the same customer would settle the first.
	onTo := e.copiedMail(t, e.customerAt(t, "customer"), colleagueAddress, [3]any{"to", e.rep, nil})
	onCC := e.copiedMail(t, e.customerAt(t, "customer"), colleagueAddress, [3]any{"cc", nil, readerAddress})

	for name, mail := range map[string]ids.UUID{"on To": onTo, "on CC": onCC} {
		if !awaitedAs(e.followUpReaderAs(e.other), t, s, mail) {
			t.Errorf("the colleague's own mail with the reader %s is not the colleague's follow-up", name)
		}
		if awaitedAs(e.followUpReader(), t, s, mail) {
			t.Errorf("a colleague's mail with the reader %s is on the reader's follow-ups", name)
		}
	}
}

// A mail captured from a colleague's send names that colleague in
// sent_by_colleague and From. It is their follow-up, not the copied mailbox's.
func TestAMailSentByAColleagueIsTheirFollowUp(t *testing.T) {
	e := setupLoad(t)
	colleagueAddress := "colleague-" + e.other.String() + "@ourco.test"
	s := e.addressedSeats(colleagueAddress)
	mail := e.copiedMail(t, e.customerAt(t, "customer"), colleagueAddress, [3]any{"cc", e.rep, nil})
	e.exec(t, `UPDATE activity SET sent_by_colleague = $2 WHERE id = $1`, mail, e.other)

	if !awaitedAs(e.followUpReaderAs(e.other), t, s, mail) {
		t.Error("the colleague named in sent_by_colleague is not given their own follow-up")
	}
	if awaitedAs(e.followUpReader(), t, s, mail) {
		t.Error("the mailbox that holds a colleague's send is given the colleague's follow-up")
	}
}

// Two seats that both list one mailbox address each read a mail logged from it
// as their own: each seat's own address list decides.
func TestASharedMailboxAddressIsEachListingSeatsFollowUp(t *testing.T) {
	e := setupLoad(t)
	shared := "shared-" + e.rep.String() + "@ourco.test"
	s := e.addressedSeats("colleague-"+e.other.String()+"@ourco.test", shared)
	logged, _ := e.sentTo(t, e.customerAt(t, "customer"), shared, 3*24*time.Hour)

	for seat, name := range map[ids.UUID]string{e.rep: "reader", e.other: "colleague"} {
		if !awaitedAs(e.followUpReaderAs(seat), t, s, logged) {
			t.Errorf("the %s, who lists the shared address, is not given the mail logged from it", name)
		}
	}
}
