// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// briefRoom is a room meeting with one contact, and the brief read at now.
type briefRoom struct {
	e       *Env
	contact ids.UUID
	meeting ids.UUID
	now     time.Time
}

func newBriefRoom(t *testing.T, zone string, now, opens time.Time) briefRoom {
	t.Helper()
	e := Setup(t)
	e.WsExec(t, `UPDATE setting SET value = to_jsonb($1::text) WHERE key = 'installation.timezone'`, zone)
	owner := OwnerConn(t)
	contact := e.SeedContact(t, "Ana Roth", &e.Rep1)
	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Cutover review', $2, 'manual', 'human:x')`, opens)
	LinkActivity(t, owner, meeting, "contact", contact)
	seatInRoom(t, owner, e.WS, meeting, contact)
	return briefRoom{e: e, contact: contact, meeting: meeting, now: now}
}

// log records an activity the admin took part in with the room's contact.
func (r briefRoom) log(t *testing.T, kind string, at time.Time, precision string) ids.UUID {
	t.Helper()
	subject := kind + " at " + at.Format(time.RFC3339)
	made, _, err := r.e.Activities.LogActivity(r.e.Admin(), activities.LogActivityInput{
		Kind: kind, Subject: &subject, OccurredAt: &at, OccurredPrecision: precision, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: r.contact}},
	})
	if err != nil {
		t.Fatalf("logging %s: %v", subject, err)
	}
	return ids.UUID(made.Id)
}

// read is the brief's "since we last spoke" line and its account arc as JSON.
func (r briefRoom) read(t *testing.T) (lastSpoke, arc string) {
	t.Helper()
	brief, err := meetingBriefServiceAt(r.e, r.now).Get(r.e.Admin(), r.meeting)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, section := range brief.Sections {
		if section.Kind == "what_changed" && len(section.Sentences) > 0 {
			lastSpoke = section.Sentences[0].Text
		}
	}
	if brief.Plan == nil {
		t.Fatal("the brief carries no plan, so its arc cannot be read")
	}
	raw, err := json.Marshal(brief.Plan.CompanyArc)
	if err != nil {
		t.Fatal(err)
	}
	return lastSpoke, string(raw)
}

const (
	neverSpoke = "First contact"
	spokeToday = "earlier today"
)

// A day meeting has taken place once its date is over. In Los Angeles its
// stored noon passes at dawn, and it is still not "when we last spoke".
func TestABriefTakesNoDayMeetingDatedTodayAsWhenWeLastSpoke(t *testing.T) {
	now := time.Date(2026, 8, 4, 13, 0, 0, 0, time.UTC)
	room := newBriefRoom(t, "America/Los_Angeles", now, now.Add(48*time.Hour))
	los, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	day := room.log(t, "meeting", occurredday.AnchorIn(now, los), activities.PrecisionDay)
	if !occurredday.AnchorIn(now, los).Before(now) {
		t.Fatal("today's anchor is not behind the clock, so this test proves nothing")
	}
	lastSpoke, arc := room.read(t)
	if !strings.HasPrefix(lastSpoke, neverSpoke) {
		t.Errorf("last spoke = %q, but the only conversation is a day meeting whose date is not over", lastSpoke)
	}
	if strings.Contains(arc, day.String()) {
		t.Errorf("the arc cites the day meeting dated today: %s", arc)
	}

	call := room.log(t, "call", now.Add(-30*time.Minute), activities.PrecisionInstant)
	lastSpoke, arc = room.read(t)
	if !strings.Contains(lastSpoke, spokeToday) {
		t.Errorf("last spoke = %q, want the call half an hour ago", lastSpoke)
	}
	if !strings.Contains(arc, call.String()) || strings.Contains(arc, day.String()) {
		t.Errorf("the arc = %s, want the call and not the day meeting", arc)
	}
}

// A call logged for today's date has happened from the start of that date. In
// Kiritimati its stored noon is a day ahead, and both reads count it already.
func TestABriefCountsADayCallDatedTodayBeforeItsStoredNoon(t *testing.T) {
	now := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	room := newBriefRoom(t, "Pacific/Kiritimati", now, now.Add(72*time.Hour))
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	if !occurredday.AnchorIn(now, kiritimati).After(now) {
		t.Fatal("today's anchor is not ahead of the clock, so this test proves nothing")
	}
	day := room.log(t, "call", occurredday.AnchorIn(now, kiritimati), activities.PrecisionDay)
	ahead := room.log(t, "call", now.Add(time.Hour), activities.PrecisionInstant)
	lastSpoke, arc := room.read(t)
	if !strings.Contains(lastSpoke, spokeToday) {
		t.Errorf("last spoke = %q, want the day call dated today", lastSpoke)
	}
	if !strings.Contains(arc, day.String()) {
		t.Errorf("the arc = %s, want the day call dated today", arc)
	}
	if strings.Contains(arc, ahead.String()) {
		t.Errorf("the arc cites a call an hour ahead: %s", arc)
	}
}

// attendeeLine is the brief's line for the room's one contact.
func (r briefRoom) attendeeLine(t *testing.T) string {
	t.Helper()
	brief, err := meetingBriefServiceAt(r.e, r.now).Get(r.e.Admin(), r.meeting)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, section := range brief.Sections {
		if section.Kind == crmcontracts.MeetingBriefSectionKindAttendees && len(section.Sentences) == 1 {
			return section.Sentences[0].Text
		}
	}
	t.Fatal("the brief names no attendee")
	return ""
}

// An attendee's last touch has happened by now. A meeting booked for later in
// the week and a day call dated ahead both come before the room. Neither has
// taken place, so the reader still meets the contact for the first time.
func TestABriefTakesNoTouchStillAheadAsAnAttendeesLastTouch(t *testing.T) {
	now := time.Date(2026, 8, 4, 13, 0, 0, 0, time.UTC)
	room := newBriefRoom(t, "UTC", now, now.Add(7*24*time.Hour))
	room.log(t, "meeting", now.Add(48*time.Hour), activities.PrecisionInstant)
	room.log(t, "call", occurredday.Anchor(now.AddDate(0, 0, 3)), activities.PrecisionDay)
	if line := room.attendeeLine(t); !strings.Contains(line, "first time you are meeting them") {
		t.Errorf("attendee = %q, but every touch with them is still ahead", line)
	}

	room.log(t, "call", now.Add(-time.Hour), activities.PrecisionInstant)
	if line := room.attendeeLine(t); !strings.Contains(line, "last spoke today") {
		t.Errorf("attendee = %q, want the call an hour ago", line)
	}
}
