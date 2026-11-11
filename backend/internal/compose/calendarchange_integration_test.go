// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A later calendar mail goes through the production parser and sink. It reads
// as its meeting and what changed since the send before it. A colleague whose
// mailbox never received it reads a plain mail.

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// meetingSend is one send of an event as Outlook writes it: a Windows zone
// stated in a VTIMEZONE, and the full event at its revision.
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

func (o *owedEnv) calendarChange(t *testing.T, message ids.UUID, reader ids.UUID) *crmcontracts.EmailCalendarChange {
	t.Helper()
	summaries, err := activities.NewStore(o.e.DB()).EmailSummariesByID(o.e.As(reader, nil, integration.AdminPerms), []ids.UUID{message})
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
	o := setupOwed(t)
	now := clocktest.Now(t)
	uid := "040000008200E00074C5B7101A82E008" + ids.NewV7().String()
	guests := []string{o.seat}
	invite := meetingSend{method: "REQUEST", start: "20261202T100000", location: "Teams", guests: guests, sentAt: now.Add(-3 * time.Hour)}
	update := meetingSend{method: "REQUEST", start: "20261203T150000", location: "Office Berlin", sequence: 1,
		guests: append([]string{"ben@customer.example"}, guests...), sentAt: now.Add(-2 * time.Hour)}
	cancel := update
	cancel.method, cancel.sequence, cancel.sentAt = "CANCEL", 2, now.Add(-time.Hour)

	invited := o.captureAs(t, invite.mail("pat@customer.example", o.seat, uid), false)
	updated := o.captureAs(t, update.mail("pat@customer.example", o.seat, uid), false)
	canceled := o.captureAs(t, cancel.mail("pat@customer.example", o.seat, uid), false)

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
