// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A calendar invitation, update or cancellation, inbound or outbound, captured
// through the production parser and sink, waits on nobody's written reply. The
// plain mail beside it is the case that still does.

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// invitationMail is the shape Google sends an attendee: a text part and an
// unnamed text/calendar part whose Content-Type names the method.
func invitationMail(from, to, method string, at time.Time) []byte {
	return []byte(strings.Join([]string{
		"From: " + from, "To: " + to,
		"Subject: Invitation: Demo @ Thu 15:00 - 15:30 (CEST)",
		"Date: " + at.Format(time.RFC1123Z),
		"Message-ID: <invite-" + ids.NewV7().String() + "@calendar.example>",
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"You have been invited to Demo.",
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=" + method,
		"",
		"BEGIN:VCALENDAR",
		"METHOD:" + method,
		"BEGIN:VEVENT",
		"ORGANIZER:mailto:" + from,
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:" + to,
		"END:VEVENT",
		"END:VCALENDAR",
		"--b1--",
		"",
	}, "\r\n"))
}

// captureAs files raw through the production sink. sentBySeat is the
// provider's word that the seat sent it, as it gives for sent mail.
func (o *owedEnv) captureAs(t *testing.T, raw []byte, sentBySeat bool) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatalf("parsing the mail: %v", err)
	}
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(
		threadConnectorCtx(o.e, o.owner), parsed.AttestSentByOwner(sentBySeat).ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing the mail: %v", err)
	}
	return ref.ID
}

// move is the server's whose-move answer for one message, as the History
// timeline reads it.
func (o *owedEnv) move(t *testing.T, message ids.UUID) crmcontracts.EmailSummaryMove {
	t.Helper()
	summaries, err := activities.NewStore(o.e.DB()).EmailSummariesByID(o.reader(), []ids.UUID{message})
	if err != nil {
		t.Fatalf("reading the move: %v", err)
	}
	summary, ok := summaries[message]
	if !ok {
		t.Fatalf("no email row came back for %s", message)
	}
	return summary.Move
}

func TestACapturedInvitationWaitsOnNobodyEitherWay(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	at := o.now.Add(-2 * time.Hour)
	asked := o.customerWrites(t, "pat@customer.example", "Can we talk pricing?", at)
	o.judgeAsRequest(t, asked)

	cases := map[string]struct {
		message ids.UUID
		method  string
		want    crmcontracts.EmailSummaryMove
	}{
		"an inbound invitation": {o.captureAs(t, invitationMail("pat@customer.example", o.seat, "REQUEST", at), false),
			"REQUEST", crmcontracts.EmailSummaryMoveNone},
		"an inbound cancellation": {o.captureAs(t, invitationMail("pat@customer.example", o.seat, "CANCEL", at), false),
			"CANCEL", crmcontracts.EmailSummaryMoveNone},
		"our sent invitation": {o.captureAs(t, invitationMail(o.seat, "pat@customer.example", "REQUEST", at), true),
			"REQUEST", crmcontracts.EmailSummaryMoveNone},
		"our sent update": {o.captureAs(t, invitationMail(o.seat, "pat@customer.example", "PUBLISH", at), true),
			"PUBLISH", crmcontracts.EmailSummaryMoveNone},
		"a plain request from the contact": {asked, "", crmcontracts.EmailSummaryMoveNeedsReply},
		"our plain mail to the contact": {o.weWrite(t, owedMail{to: "pat@customer.example", subject: "The quote", at: at}),
			"", crmcontracts.EmailSummaryMoveWaitingForThem},
	}
	for name, c := range cases {
		if got := o.calendarMethod(t, c.message); got != c.method {
			t.Errorf("%s: capture stored calendar method %q, want %q", name, got, c.method)
		}
		if got := o.move(t, c.message); got != c.want {
			t.Errorf("%s: move %q, want %q", name, got, c.want)
		}
	}
}

// Their reply ends the wait on our mail. A reply the reader may not read does
// not end it, or the badge would tell them a message they cannot open exists.
func TestTheirReplyEndsTheWaitOnlyWhenTheReaderMayReadIt(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	ours := o.weWrite(t, owedMail{
		to: "pat@customer.example", subject: "The quote",
		messageID: "quote-" + ids.NewV7().String() + "@ws.example", at: o.now.Add(-3 * time.Hour),
	})
	if got := o.move(t, ours); got != crmcontracts.EmailSummaryMoveWaitingForThem {
		t.Fatalf("our unanswered mail reads %q, want waiting_for_them", got)
	}
	theirReply := func(mailbox ids.UUID, seat string, at time.Time) ids.UUID {
		return o.captureFor(t, mailbox, seat, owedMail{
			from: "pat@customer.example", to: seat, cc: o.seat, subject: "Re: The quote",
			messageID: "re-" + ids.NewV7().String() + "@customer.example",
			inReplyTo: o.messageIDOf(t, ours), at: at,
		})
	}

	private := theirReply(o.e.Rep2, seatAddress(t, o.e, o.e.Rep2), o.now.Add(-2*time.Hour))
	if o.threadKey(t, private) != o.threadKey(t, ours) {
		t.Fatal("the reply did not join our thread, so the test cannot tell a reply from a new mail")
	}
	if n := o.count(t, `SELECT count(*) FROM activity WHERE id = $1 AND audience = 'workspace'`, private); n != 0 {
		t.Fatal("the colleague's mailbox shares its mail; the case needs a private reply")
	}
	if got := o.move(t, ours); got != crmcontracts.EmailSummaryMoveWaitingForThem {
		t.Errorf("a reply the reader may not read ended the wait: %q", got)
	}

	theirReply(o.owner, o.seat, o.now.Add(-time.Hour))
	if got := o.move(t, ours); got != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("our mail reads %q after a reply the reader can read, want none", got)
	}
}

// Our own invitation captured before calendar_method existed waits on nobody,
// before the pass reads its method back and after. A plain legacy mail beside
// it still waits. The outbound walk keeps its own place: an inbound marker
// newer than our mail must not make the walk skip it.
func TestTheBackfillReadsTheMethodOfOurOwnLegacyInvitation(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	decline := o.captureRaw(t, declineMail(o.seat, o.now.Add(-time.Hour)))
	o.asCapturedBeforeTheColumn(t, decline, time.Hour)
	if n := runCalendarMethodBackfill(t, o, nil); n != 1 {
		t.Fatalf("the inbound pass settled %d messages, want 1", n)
	}

	invitation := o.captureAs(t, invitationMail(o.seat, "pat@customer.example", "REQUEST", o.now.Add(-3*time.Hour)), true)
	o.asCapturedBeforeTheColumn(t, invitation, 3*time.Hour)
	plain := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "The quote", at: o.now.Add(-4 * time.Hour)})
	o.asCapturedBeforeTheColumn(t, plain, 4*time.Hour)
	for message, want := range map[ids.UUID]crmcontracts.EmailSummaryMove{
		invitation: crmcontracts.EmailSummaryMoveNone, plain: crmcontracts.EmailSummaryMoveWaitingForThem,
	} {
		if got := o.move(t, message); got != want {
			t.Fatalf("before the pass, legacy mail %s reads %q, want %q", message, got, want)
		}
	}
	if got := o.offeredByTheWalk(t); got != 2 {
		t.Fatalf("the walk offers %d messages before the pass, want our two legacy mails", got)
	}

	if n := runCalendarMethodBackfill(t, o, nil); n != 2 {
		t.Fatalf("the pass settled %d messages, want our two legacy mails", n)
	}
	if got := o.calendarMethod(t, invitation); got != "REQUEST" {
		t.Fatalf("the pass stored calendar method %q, want REQUEST", got)
	}
	if n := o.count(t, `SELECT count(*) FROM activity_calendar_method_backfill
		WHERE activity_id = $1 AND outcome = 'set' AND direction = 'outbound'`, invitation); n != 1 {
		t.Errorf("the outbound marker says set %d times, want 1", n)
	}
	if n := o.count(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'activity' AND entity_id = $1 AND after ? 'calendar_method'`, invitation); n != 1 {
		t.Errorf("%d audit rows record the method, want 1", n)
	}
	if got := o.move(t, invitation); got != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("our invitation reads %q once its method was read back, want none", got)
	}
	if got := o.move(t, plain); got != crmcontracts.EmailSummaryMoveWaitingForThem {
		t.Errorf("the plain legacy mail reads %q once checked, want waiting_for_them", got)
	}
	if n := runCalendarMethodBackfill(t, o, nil); n != 0 {
		t.Errorf("a second pass settled %d messages, want none", n)
	}
	if got := o.offeredByTheWalk(t); got != 0 {
		t.Errorf("the walk still offers %d messages after the pass", got)
	}
}

// offeredByTheWalk is how many messages the pass's own walk would offer next.
func (o *owedEnv) offeredByTheWalk(t *testing.T) int {
	t.Helper()
	var offered int
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		found, err := activities.CalendarMethodCandidatesTx(t.Context(), tx, calendarMethodBackfillPerTick)
		offered = len(found)
		return err
	}); err != nil {
		t.Fatalf("reading the walk: %v", err)
	}
	return offered
}
