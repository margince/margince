// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// "Waiting for their reply" is the sender's to see. A colleague copied on the
// mail, or reading it from a shared mailbox, did not write last. The calendar
// shapes mail programs send go through the production parser, and the badge
// and the follow-up lane agree on each.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// moveFor is the server's whose-move answer for one message, read as reader.
func (o *owedEnv) moveFor(t *testing.T, reader context.Context, message ids.UUID) crmcontracts.EmailSummaryMove {
	t.Helper()
	summaries, err := activities.NewStore(o.e.DB()).EmailSummariesByID(reader, []ids.UUID{message})
	if err != nil {
		t.Fatalf("reading the move: %v", err)
	}
	summary, ok := summaries[message]
	if !ok {
		t.Fatalf("no email row came back for %s", message)
	}
	return summary.Move
}

func TestOnlyTheSeatThatSentAMailSeesItWaiting(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	colleagueSeat := seatAddress(t, o.e, o.e.Rep2)
	colleague := o.e.As(o.e.Rep2, nil, integration.AdminPerms)

	ours := o.weWrite(t, owedMail{
		to: "pat@customer.example", cc: colleagueSeat, subject: "The quote", at: o.now.Add(-2 * time.Hour),
	})
	// The colleague's own mail, received as a copy in the shared mailbox from
	// their login address at a domain the workspace trusts.
	login := o.e.WsScalar(t, `SELECT email FROM app_user WHERE id = $1`, o.e.Rep2)
	o.e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ($1, 'admin', true)
		ON CONFLICT DO NOTHING`, login[strings.LastIndex(login, "@")+1:])
	theirs := o.capture(t, owedMail{
		from: login, to: "pat@customer.example", cc: o.seat, subject: "Our terms", authenticated: true,
		messageID: "terms-" + ids.NewV7().String() + "@ws.example", at: o.now.Add(-2 * time.Hour),
	})
	if n := o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND audience = 'workspace'`, ours); n != 1 {
		t.Fatal("the shared mailbox did not share our mail, so the colleague cannot read it at all")
	}
	if n := o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND direction = 'outbound'
		AND sent_by_colleague = $2`, theirs, o.e.Rep2); n != 1 {
		t.Fatal("capture did not read the copy as the colleague's sent mail, so the case proves nothing")
	}

	for name, c := range map[string]struct {
		reader  context.Context
		message ids.UUID
		want    crmcontracts.EmailSummaryMove
	}{
		"the sender reads their own mail":                     {o.reader(), ours, crmcontracts.EmailSummaryMoveWaitingForThem},
		"a copied colleague reads it from the shared mailbox": {colleague, ours, crmcontracts.EmailSummaryMoveNone},
		"the mailbox owner reads a colleague's mail":          {o.reader(), theirs, crmcontracts.EmailSummaryMoveNone},
		"the colleague reads their own mail":                  {colleague, theirs, crmcontracts.EmailSummaryMoveWaitingForThem},
	} {
		if got := o.moveFor(t, c.reader, c.message); got != c.want {
			t.Errorf("%s: move %q, want %q", name, got, c.want)
		}
	}
}

// customerContact writes a contact at a customer company, the record the
// follow-up lane reminds about, and returns the contact.
func (o *owedEnv) customerContact(t *testing.T, name, address string) ids.UUID {
	t.Helper()
	contact := o.contact(t, name, address)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		company := ids.NewV7()
		if _, err := tx.Exec(t.Context(), `INSERT INTO company (id, display_name, lifecycle, source, captured_by)
			VALUES ($1, 'Customer Co', 'customer', 'seed', 'system')`, company); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
			VALUES ('employment', $1, $2, 'seed', 'system')`, contact, company)
		return err
	}); err != nil {
		t.Fatalf("making %s a customer: %v", name, err)
	}
	return contact
}

// followedUp reports whether the follow-up lane reminds the reader of message.
func (o *owedEnv) followedUp(t *testing.T, message ids.UUID) bool {
	t.Helper()
	rows, _, err := activities.NewStore(o.e.DB()).AwaitingReplies(o.reader(), o.now)
	if err != nil {
		t.Fatalf("reading the follow-up lane: %v", err)
	}
	for _, row := range rows {
		if row.ActivityID == message {
			return true
		}
	}
	return false
}

// calendarShapedMail is one sent mail with the given MIME parts after its
// text, three days old so the follow-up window has passed.
func calendarShapedMail(from, to, subject, inReplyTo string, at time.Time, parts ...string) []byte {
	lines := []string{
		"From: " + from, "To: " + to, "Subject: " + subject, "Date: " + at.Format(time.RFC1123Z),
		"Message-ID: <shape-" + ids.NewV7().String() + "@ws.example>", "MIME-Version: 1.0",
	}
	if inReplyTo != "" {
		lines = append(lines, "In-Reply-To: <"+inReplyTo+">", "References: <"+inReplyTo+">")
	}
	lines = append(lines, `Content-Type: multipart/mixed; boundary="b1"`, "",
		"--b1", "Content-Type: text/plain; charset=UTF-8", "", "Can you confirm the time?")
	for _, part := range parts {
		lines = append(lines, "--b1", part)
	}
	return []byte(strings.Join(append(lines, "--b1--", ""), "\r\n"))
}

const requestCalendar = "BEGIN:VCALENDAR\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nEND:VEVENT\r\nEND:VCALENDAR"

func TestTheBadgeAndTheFollowUpLaneAgreeOnCalendarShapedMail(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.customerContact(t, "Pat Buyer", "pat@customer.example")
	sent := o.now.Add(-3 * 24 * time.Hour)
	invitation := o.captureAs(t, invitationMail(o.seat, "pat@customer.example", "REQUEST", sent.Add(-time.Hour)), true)

	for _, c := range []struct {
		name    string
		raw     []byte
		waiting bool
	}{
		{"an Outlook invitation with a named invite.ics part", calendarShapedMail(o.seat, "pat@customer.example",
			"Kickoff", "", sent,
			"Content-Type: text/calendar; method=REQUEST; name=\"invite.ics\"\r\n"+
				"Content-Disposition: attachment; filename=\"invite.ics\"\r\n\r\n"+requestCalendar), false},
		{"an invitation forwarded inside a message", calendarShapedMail(o.seat, "pat@customer.example",
			"Fwd: Invitation: Kickoff", "", sent,
			"Content-Type: message/rfc822\r\n\r\nFrom: lead@ws.example\r\nSubject: Invitation: Kickoff\r\n"+
				"Content-Type: text/calendar; method=REQUEST\r\n\r\n"+requestCalendar), true},
		{"our plain reply inside the invitation's thread", calendarShapedMail(o.seat, "pat@customer.example",
			"Re: Invitation: Demo", o.messageIDOf(t, invitation), sent), true},
	} {
		message := o.captureAs(t, c.raw, true)
		want := crmcontracts.EmailSummaryMoveNone
		if c.waiting {
			want = crmcontracts.EmailSummaryMoveWaitingForThem
		}
		if got := o.move(t, message); got != want {
			t.Errorf("%s: move %q (calendar method %q), want %q", c.name, got, o.calendarMethod(t, message), want)
		}
		if got := o.followedUp(t, message); got != c.waiting {
			t.Errorf("%s: on the follow-up lane %v, want %v", c.name, got, c.waiting)
		}
	}
	if o.followedUp(t, invitation) || o.move(t, invitation) != crmcontracts.EmailSummaryMoveNone {
		t.Error("our invitation itself is waiting for a reply")
	}
}
