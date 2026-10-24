// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// relationshipReading holds the readers of "how this relationship stands" for
// one contact at a one-contact company, read through the production code.
type relationshipReading struct {
	inbound90d, outbound90d             int
	contactIn, contactOut               *time.Time
	companyIn, companyOut, companyTouch *time.Time
	companyTouchID                      ids.ActivityID
	edgeCount90d                        int
}

type relationshipFixture struct {
	e                *integration.Env
	seat             ids.UUID
	seatAddr         string
	contact, company ids.UUID
	contactAddr      string
	now              time.Time
}

func newRelationshipFixture(t *testing.T) *relationshipFixture {
	t.Helper()
	e := integration.Setup(t)
	f := &relationshipFixture{e: e, seat: e.Rep1, contactAddr: "anna@client.test", now: time.Now().UTC().Truncate(time.Second)}
	f.seatAddr = sharedMailbox(t, e, f.seat)
	made, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Anna Client", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: f.contactAddr, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.contact = ids.UUID(made.Id)
	f.company = e.SeedCompany(t, "Client GmbH", nil)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, role, source, captured_by)
		VALUES ('employment', $1, $2, 'Buyer', 'manual', 'human:x')`, f.contact, f.company)
	return f
}

// mail captures one message through the mail parser and the production sink.
func (f *relationshipFixture) mail(t *testing.T, key, from, to, cc string, daysAgo int, extra ...string) ids.UUID {
	t.Helper()
	headers := []string{
		"From: " + from, "To: " + to,
		"Subject: " + key,
		"Date: " + f.now.AddDate(0, 0, -daysAgo).Format(time.RFC1123Z),
		"Message-ID: <" + key + "@relationship.test>",
	}
	if cc != "" {
		headers = append(headers, "Cc: "+cc)
	}
	body := []string{"Content-Type: text/plain; charset=utf-8", "", "Words a human wrote about " + key + ".", ""}
	if len(extra) > 0 {
		body = extra
	}
	captureMailVia(t, f.e, f.seat, "gmail", []byte(strings.Join(append(headers, body...), "\r\n")))
	return activityBySubject(t, f.e, key)
}

// calendarBody is an invitation the way a calendar sends it: a text part and
// a text/calendar part naming its method.
func calendarBody(method string) []string {
	return []string{
		"MIME-Version: 1.0", `Content-Type: multipart/mixed; boundary="b1"`, "",
		"--b1", "Content-Type: text/plain; charset=UTF-8", "", "You have been invited.",
		"--b1", "Content-Type: text/calendar; charset=UTF-8; method=" + method, "",
		"BEGIN:VCALENDAR", "METHOD:" + method, "BEGIN:VEVENT", "END:VEVENT", "END:VCALENDAR",
		"--b1--", "",
	}
}

// productMail files the product's own confirm-details mail through the writer
// the controller lane uses.
func (f *relationshipFixture) productMail(t *testing.T, daysAgo int) ids.UUID {
	t.Helper()
	subject, body, outbound := "Review your contact details", "Please confirm the details we hold.", "outbound"
	at := f.now.AddDate(0, 0, -daysAgo)
	made, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &outbound, OccurredAt: &at,
		Source: "system:controller", Origin: activities.OriginSystemNotice,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(made.Id)
}

func (f *relationshipFixture) read(t *testing.T) relationshipReading {
	t.Helper()
	var got relationshipReading
	ctx := f.e.Admin()
	err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		// The contact.created handler's repair names the contact on the mail
		// capture filed by address, as a live install does after capture.
		if _, err := f.e.Contacts.PromoteContactCohortTx(ctx, tx, ids.ContactID{UUID: f.contact}); err != nil {
			return err
		}
		if err := search.RebuildEdges(ctx, tx); err != nil {
			return err
		}
		all, err := contacts.StrengthForCompanyContacts(ctx, tx, ids.CompanyID{UUID: f.company}, f.now, nil)
		if err != nil {
			return err
		}
		for _, one := range all {
			if one.ContactID.UUID == f.contact {
				got.inbound90d, got.outbound90d = one.Strength.Inbound90d, one.Strength.Outbound90d
			}
		}
		touch, err := contact360.LastTouchFor(ctx, tx, []ids.ContactID{{UUID: f.contact}}, contact360.AssembleOptions{})
		if err != nil {
			return err
		}
		got.contactIn, got.contactOut = touch[ids.ContactID{UUID: f.contact}].InboundAt, touch[ids.ContactID{UUID: f.contact}].OutboundAt
		company, err := company360.LastTouchFor(ctx, tx, []ids.CompanyID{{UUID: f.company}}, f.now, company360.AssembleOptions{})
		if err != nil {
			return err
		}
		account := company[ids.CompanyID{UUID: f.company}]
		got.companyIn, got.companyOut = account.InboundAt, account.OutboundAt
		if account.Contact != nil {
			at := account.Contact.At
			got.companyTouch, got.companyTouchID = &at, account.Contact.ActivityID
		}
		return tx.QueryRow(context.Background(), `SELECT coalesce(max(count_90d), 0) FROM graph_interaction_edge
			 WHERE user_id = $1 AND contact_id = $2`, f.seat, f.contact).Scan(&got.edgeCount90d)
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func equalMoment(a, b *time.Time) bool {
	return (a == nil) == (b == nil) && (a == nil || a.Equal(*b))
}

func assertReading(t *testing.T, label string, got, want relationshipReading) {
	t.Helper()
	if got.inbound90d != want.inbound90d || got.outbound90d != want.outbound90d {
		t.Errorf("%s: contact strength counts %d in / %d out, want %d / %d", label, got.inbound90d, got.outbound90d, want.inbound90d, want.outbound90d)
	}
	if !equalMoment(got.contactIn, want.contactIn) || !equalMoment(got.contactOut, want.contactOut) {
		t.Errorf("%s: contact page clocks in=%v out=%v, want in=%v out=%v", label, got.contactIn, got.contactOut, want.contactIn, want.contactOut)
	}
	if !equalMoment(got.companyIn, want.companyIn) || !equalMoment(got.companyOut, want.companyOut) {
		t.Errorf("%s: company clocks in=%v out=%v, want in=%v out=%v", label, got.companyIn, got.companyOut, want.companyIn, want.companyOut)
	}
	if !equalMoment(got.companyTouch, want.companyTouch) || got.companyTouchID != want.companyTouchID {
		t.Errorf("%s: company last contact %v (%s), want %v (%s)", label, got.companyTouch, got.companyTouchID, want.companyTouch, want.companyTouchID)
	}
	if got.edgeCount90d != want.edgeCount90d {
		t.Errorf("%s: the Network edge counts %d exchanges, want %d", label, got.edgeCount90d, want.edgeCount90d)
	}
}

func instantOf(t *testing.T, e *integration.Env, activity ids.UUID) *time.Time {
	t.Helper()
	at := workspaceValue[time.Time](t, e, `SELECT occurred_at FROM activity WHERE id = $1`, activity)
	return &at
}

// A calendar invitation, a file-share notice, a delivery report and the
// product's own mail change no reading of the relationship. A human's reply
// moves every one of them. The company has one contact, so its reading and
// the contact's are the same history read twice.
func TestOnlyAHumansMessageMovesTheRelationship(t *testing.T) {
	f := newRelationshipFixture(t)
	asked := f.mail(t, "pricing-question", f.contactAddr, f.seatAddr, "", 20)
	answered := f.mail(t, "pricing-answer", f.seatAddr, f.contactAddr, "", 18)
	before := relationshipReading{
		inbound90d: 1, outbound90d: 1,
		contactIn: instantOf(t, f.e, asked), contactOut: instantOf(t, f.e, answered),
		companyIn: instantOf(t, f.e, asked), companyOut: instantOf(t, f.e, answered),
		companyTouch: instantOf(t, f.e, answered), companyTouchID: ids.ActivityID{UUID: answered},
		edgeCount90d: 2,
	}
	assertReading(t, "two humans writing", f.read(t), before)

	f.mail(t, "kickoff-invite", f.seatAddr, f.contactAddr, "", 6, calendarBody("REQUEST")...)
	f.mail(t, "kickoff-accepted", f.contactAddr, f.seatAddr, "", 5, calendarBody("REPLY")...)
	f.mail(t, "deck-shared", "Rep via Google Slides <drive-shares-dm-noreply@google.com>", f.contactAddr, f.seatAddr, 4)
	f.mail(t, "delivery-failed", "Mail Delivery Subsystem <mailer-daemon@googlemail.com>", f.seatAddr, f.contactAddr, 3)
	f.productMail(t, 2)
	assertReading(t, "after automatic mail", f.read(t), before)

	reply := f.mail(t, "pricing-followup", f.contactAddr, f.seatAddr, "", 1)
	after := before
	after.inbound90d = 2
	after.contactIn, after.companyIn = instantOf(t, f.e, reply), instantOf(t, f.e, reply)
	after.companyTouch, after.companyTouchID = instantOf(t, f.e, reply), ids.ActivityID{UUID: reply}
	after.edgeCount90d = 3
	assertReading(t, "after their reply", f.read(t), after)
}

// A thread the contact is only copied on is not the contact writing: the
// contact page's inbound clock stays on what they sent.
func TestACopiedContactHasNotWrittenTheThread(t *testing.T) {
	f := newRelationshipFixture(t)
	own := f.mail(t, "their-own-mail", f.contactAddr, f.seatAddr, "", 10)
	f.mail(t, "a-colleague-writes", "Bob Colleague <bob@client.test>", f.seatAddr, f.contactAddr, 2)
	got := f.read(t)
	if !equalMoment(got.contactIn, instantOf(t, f.e, own)) {
		t.Fatalf("the contact page says they last wrote %v, want their own mail at %v", got.contactIn, instantOf(t, f.e, own))
	}
}

// The SQL rule every statement renders and the Go rule the read contract's
// automatic_kind comes from give one answer for every row shape.
func TestTheGoAndSQLWrittenByHumanRulesAgree(t *testing.T) {
	e := integration.Setup(t)
	var rows []relstrength.Evidence
	for _, kind := range []string{"email", "call", "meeting", "message", "note"} {
		for _, origin := range []string{"human", "agent", "system_notice", "system_remediation"} {
			for _, method := range []string{"", "REQUEST", "REPLY", "CANCEL"} {
				for _, machine := range []bool{false, true} {
					rows = append(rows, relstrength.Evidence{Kind: kind, Origin: origin, CalendarMethod: method, SenderIsMachine: machine})
				}
			}
		}
	}
	for _, row := range rows {
		var method *string
		if row.CalendarMethod != "" {
			method = &row.CalendarMethod
		}
		inSQL := workspaceValue[bool](t, e, `SELECT `+relstrength.WrittenByHumanSQL("a")+`
			FROM (SELECT $1::text AS kind, $2::text AS origin, $3::text AS calendar_method, $4::boolean AS sender_is_machine) a`,
			row.Kind, row.Origin, method, row.SenderIsMachine)
		if inGo := row.Automatic() == ""; inGo != inSQL {
			t.Errorf("%+v: Go says a human wrote it = %v, SQL = %v", row, inGo, inSQL)
		}
	}
}

// workspaceValue reads one value inside the workspace transaction.
func workspaceValue[T any](t *testing.T, e *integration.Env, query string, args ...any) T {
	t.Helper()
	var got T
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&got)
	}); err != nil {
		t.Fatal(err)
	}
	return got
}

// sharedMailbox connects the seat's mailbox with the shared posture, so its
// mail is the workspace's and every colleague's reading counts it.
func sharedMailbox(t *testing.T, e *integration.Env, seat ids.UUID) string {
	t.Helper()
	address := seatAddress(t, e, seat)
	e.WsExec(t, `UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, seat)
	return address
}
