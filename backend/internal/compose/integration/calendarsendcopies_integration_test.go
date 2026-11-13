// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// Google Calendar mails each guest their own copy of the organizer's
// invitation and files every copy in the organizer's Sent. Captured through
// the production sink, the copies of one send are one activity, so the
// promise reader reads the invitation once.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// inviteUID is the event every copy names, whichever seat organizes it.
const inviteUID = "5c32ehv2a2j2aso23n324ua1ke@google.com"

// guestCopy is the copy of one send that Google mails to guest. sequence is
// the event's revision, so a changed sequence is a later send.
func guestCopy(organizer, guest, sequence string) []byte {
	return []byte(strings.Join([]string{
		"Sender: Google Calendar <calendar-notification@google.com>",
		"Message-ID: <calendar-" + ids.NewV7().String() + "@google.com>",
		"From: " + organizer, "To: " + guest,
		"Subject: Invitation: Document Management @ Fri Oct 30, 2026 2pm - 3pm (" + guest + ")",
		"Date: Fri, 23 Oct 2026 10:52:51 +0000",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="b1"`,
		"",
		"--b1",
		"Content-Type: text/plain; charset=UTF-8",
		"",
		"Document Management, Friday Oct 30, 2026. Reply for " + guest,
		"--b1",
		"Content-Type: text/calendar; charset=UTF-8; method=REQUEST",
		"",
		"BEGIN:VCALENDAR",
		"METHOD:REQUEST",
		"BEGIN:VEVENT",
		"UID:" + inviteUID,
		"SEQUENCE:" + sequence,
		"SUMMARY:Document Management",
		"DTSTART:20261030T070000Z",
		"DTEND:20261030T080000Z",
		"ORGANIZER:mailto:" + organizer,
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ana@customer.example",
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:ben@customer.example",
		"END:VEVENT",
		"END:VCALENDAR",
		"--b1--",
		"",
	}, "\r\n"))
}

// captureCopy files one copy as the seat's Gmail sync does, with the mailbox's
// own id for it.
func captureCopy(t *testing.T, e *integration.Env, owner ids.UUID, seat, providerID string, raw []byte) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, seat)
	if err != nil {
		t.Fatalf("parsing the copy: %v", err)
	}
	rec := parsed.AttestSentByOwner(true).ToRecord("gmail", raw)
	rec.ProviderMessageID = providerID
	ref, err := compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}).Upsert(connectorCtx(e, "gmail", owner), rec)
	if err != nil {
		t.Fatalf("capturing the copy: %v", err)
	}
	return ref.ID
}

// guestCopyMailbox connects a Gmail mailbox for the seat and answers its
// address, so the seat's sync is recorded as holding what it captures.
func guestCopyMailbox(t *testing.T, e *integration.Env, seat ids.UUID) string {
	t.Helper()
	address := "organizer-" + seat.String() + "@ws.example"
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label)
			VALUES ($1, 'gmail', 'connected', 'vault:test', $2)`, seat, address)
		return err
	}); err != nil {
		t.Fatalf("connecting the seat's mailbox: %v", err)
	}
	return address
}

// storedInvitations counts the seat's live invitation mail of the event, and
// the mailbox ids its import rows hold.
func storedInvitations(t *testing.T, e *integration.Env, owner ids.UUID, uid string) (rows, mailboxIDs int) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT count(DISTINCT a.id), coalesce(sum(cardinality(ci.provider_message_ids)), 0)
			  FROM activity a JOIN capture_import ci ON ci.activity_id = a.id AND ci.user_id = $1
			 WHERE a.kind = 'email' AND a.calendar_uid = $2 AND a.archived_at IS NULL`,
			owner, uid).Scan(&rows, &mailboxIDs)
	}); err != nil {
		t.Fatalf("reading the stored invitations: %v", err)
	}
	return rows, mailboxIDs
}

func TestEveryGuestsCopyOfOneInvitationIsOneActivity(t *testing.T) {
	e := integration.Setup(t)
	seat := guestCopyMailbox(t, e, e.Rep1)

	first := captureCopy(t, e, e.Rep1, seat, "mailbox-1", guestCopy(seat, "ana@customer.example", "0"))
	second := captureCopy(t, e, e.Rep1, seat, "mailbox-2", guestCopy(seat, "ben@customer.example", "0"))
	if second != first {
		t.Errorf("the second guest's copy landed as %s, want the invitation %s", second, first)
	}
	// A sync that reads the second copy again changes nothing.
	captureCopy(t, e, e.Rep1, seat, "mailbox-2", guestCopy(seat, "ben@customer.example", "0"))
	if rows, held := storedInvitations(t, e, e.Rep1, inviteUID); rows != 1 || held != 2 {
		t.Errorf("stored %d invitations holding %d mailbox ids, want 1 holding both copies", rows, held)
	}

	// An update is a send of its own, and is stored beside the invitation.
	update := captureCopy(t, e, e.Rep1, seat, "mailbox-3", guestCopy(seat, "ana@customer.example", "1"))
	if update == first {
		t.Error("the update joined the invitation, want an activity of its own")
	}
}

// Two seats' copies of one event never share a row through a calendar send,
// because a seat's mail joins only that seat's own rows.
func TestAnotherSeatsCopiesStayTheirOwn(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep2, "rep")
	one, two := guestCopyMailbox(t, e, e.Rep1), guestCopyMailbox(t, e, e.Rep2)
	mine := captureCopy(t, e, e.Rep1, one, "mailbox-1", guestCopy(one, "ana@customer.example", "0"))
	theirs := captureCopy(t, e, e.Rep2, two, "mailbox-1", guestCopy(two, "ana@customer.example", "0"))
	if mine == theirs {
		t.Fatal("two seats' invitations were captured as one activity")
	}
}
