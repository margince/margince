// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package calendarchangeit_test holds the end-to-end proof for meeting updates.
// A later calendar mail goes through the production parser and sink. It reads
// as its meeting and what changed since the send before it. A colleague whose
// mailbox never received it reads a plain mail.
package calendarchangeit_test

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailbox is one rep's connected, shared Gmail mailbox.
type mailbox struct {
	e     *integration.Env
	owner ids.UUID
	seat  string
}

// setupMailbox connects a shared mailbox for a rep. Capture files a message
// against a seat only when that seat's own connected mailbox is on it.
func setupMailbox(t *testing.T) mailbox {
	t.Helper()
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	m := mailbox{e: e, owner: e.Rep1, seat: "seat-" + e.Rep1.String() + "@ws.example"}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_connection (user_id, provider, status, credential_ref, account_label, mail_posture)
			VALUES ($1, 'gmail', 'connected', 'vault:test', $2, 'shared')
			ON CONFLICT (user_id, provider) DO UPDATE SET account_label = $2, archived_at = NULL, mail_posture = 'shared'`,
			m.owner, m.seat)
		return err
	}); err != nil {
		t.Fatalf("connecting the seat's mailbox: %v", err)
	}
	return m
}

// capture files raw through the production parser and sink, as the Gmail
// connector does for the seat.
func (m mailbox) capture(t *testing.T, raw []byte) ids.UUID {
	t.Helper()
	parsed, err := mailmap.Parse(raw, m.seat)
	if err != nil {
		t.Fatalf("parsing the mail: %v", err)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), m.e.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:gmail", UserID: m.owner, OnBehalfOf: m.owner,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	ref, err := compose.NewCaptureSink(m.e.Pool, compose.CaptureConfig{}).Upsert(ctx, parsed.ToRecord("gmail", raw))
	if err != nil {
		t.Fatalf("capturing the mail: %v", err)
	}
	return ref.ID
}

type meetingSend struct {
	method, start, location string
	sequence                int
	guests                  []string
	sentAt                  time.Time
}

func (s meetingSend) mail(from, to, uid string) []byte {
	lines := []string{
		"From: " + from, "To: " + to,
		"Subject: Pricing review",
		"Date: " + s.sentAt.Format(time.RFC1123Z),
		"Message-ID: <send-" + ids.NewV7().String() + "@partner.example>",
		"Content-Class: urn:content-classes:calendarmessage",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="b1"`,
		"",
		"--b1", "Content-Type: text/plain; charset=UTF-8", "", "Pricing review", "--b1",
		"Content-Type: text/calendar; charset=\"utf-8\"; method=" + s.method,
		"",
		"BEGIN:VCALENDAR", "METHOD:" + s.method,
		"BEGIN:VTIMEZONE", "TZID:W. Europe Standard Time",
		"BEGIN:STANDARD", "DTSTART:16010101T030000", "TZOFFSETTO:+0100",
		"RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=10", "END:STANDARD",
		"BEGIN:DAYLIGHT", "DTSTART:16010101T020000", "TZOFFSETTO:+0200",
		"RRULE:FREQ=YEARLY;BYDAY=-1SU;BYMONTH=3", "END:DAYLIGHT",
		"END:VTIMEZONE",
		"BEGIN:VEVENT", "UID:" + uid, "SUMMARY:Pricing review",
		"SEQUENCE:" + strconv.Itoa(s.sequence),
		"DTSTART;TZID=W. Europe Standard Time:" + s.start,
		"LOCATION:" + s.location,
		"ORGANIZER;CN=\"Buyer, Pat\":mailto:" + from,
	}
	for _, guest := range s.guests {
		lines = append(lines, "ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:"+guest)
	}
	lines = append(lines, "END:VEVENT", "END:VCALENDAR", "--b1--", "")
	return []byte(strings.Join(lines, "\r\n"))
}

func (m mailbox) calendarChange(t *testing.T, message ids.UUID, reader ids.UUID) *crmcontracts.EmailCalendarChange {
	t.Helper()
	summaries, err := activities.NewStore(m.e.DB()).EmailSummariesByID(m.e.As(reader, nil, integration.AdminPerms), []ids.UUID{message})
	if err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	summary, ok := summaries[message]
	if !ok {
		t.Fatalf("no email row came back for %s", message)
	}
	return summary.CalendarChange
}

func TestAMeetingUpdateSaysWhatChangedSinceTheInvitation(t *testing.T) {
	o := setupMailbox(t)
	now := clocktest.Now(t)
	uid := "040000008200E00074C5B7101A82E008" + ids.NewV7().String()
	guests := []string{o.seat}
	invite := meetingSend{method: "REQUEST", start: "20261202T100000", location: "Teams", guests: guests, sentAt: now.Add(-3 * time.Hour)}
	update := meetingSend{
		method: "REQUEST", start: "20261203T150000", location: "Office Berlin", sequence: 1,
		guests: append([]string{"ben@customer.example"}, guests...), sentAt: now.Add(-2 * time.Hour),
	}
	cancel := update
	cancel.method, cancel.sequence, cancel.sentAt = "CANCEL", 2, now.Add(-time.Hour)

	invited := o.capture(t, invite.mail("pat@customer.example", o.seat, uid))
	updated := o.capture(t, update.mail("pat@customer.example", o.seat, uid))
	canceled := o.capture(t, cancel.mail("pat@customer.example", o.seat, uid))

	if got := o.calendarChange(t, invited, o.owner); got != nil {
		t.Errorf("the first invitation reads %+v, want no change: it asks for an answer", got)
	}
	got := o.calendarChange(t, updated, o.owner)
	if got == nil || got.Kind != crmcontracts.EmailCalendarChangeKindUpdated {
		t.Fatalf("the update reads %+v, want updated", got)
	}
	if strings.Join(changedNames(got.Changed), ",") != "time,location" {
		t.Errorf("changed = %v, want time and location", got.Changed)
	}
	// 10:00 in Berlin's winter is 09:00 UTC; 15:00 the next day is 14:00 UTC.
	if got.Before == nil || !got.Before.StartsAt.Equal(time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC)) ||
		!got.StartsAt.Equal(time.Date(2026, 12, 3, 14, 0, 0, 0, time.UTC)) {
		t.Errorf("moved from %v to %v, want 09:00 UTC on the 2nd to 14:00 UTC on the 3rd", got.Before, got.StartsAt)
	}
	if got.GuestsAdded == nil || (*got.GuestsAdded)[0].Email != "ben@customer.example" {
		t.Errorf("guests added = %v, want Ben", got.GuestsAdded)
	}
	if off := o.calendarChange(t, canceled, o.owner); off == nil || off.Kind != crmcontracts.EmailCalendarChangeKindCanceled ||
		*off.Location != "Office Berlin" {
		t.Errorf("the cancellation reads %+v, want canceled at the place it last had", off)
	}
	o.e.GrantRole(t, o.e.Rep2, "rep")
	if seen := o.calendarChange(t, updated, o.e.Rep2); seen != nil {
		t.Errorf("a colleague whose mailbox never received the update reads %+v, want a plain mail", seen)
	}
}

func changedNames(changed []crmcontracts.EmailCalendarChangeChanged) []string {
	out := make([]string, 0, len(changed))
	for _, field := range changed {
		out = append(out, string(field))
	}
	return out
}
