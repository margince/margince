// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contactbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
)

// companyContextOf is the prose of the brief's company_context section.
func companyContextOf(t *testing.T, e *Env, meeting ids.UUID) (text string, withheldNamed bool) {
	t.Helper()
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	brief, err := meetingBriefService(e).Get(rep, meeting)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var out strings.Builder
	for _, section := range brief.Sections {
		if section.Kind != "company_context" {
			continue
		}
		for _, sentence := range section.Sentences {
			out.WriteString(sentence.Text + "\n")
		}
	}
	for _, omission := range valueOf(brief.Omitted) {
		withheldNamed = withheldNamed || omission.Source == "activity_history"
	}
	return out.String(), withheldNamed
}

// A prep for a new contact names what the company's other contacts already
// received. It leaves out what this reader may not open. It also leaves out a
// booking tool's confirmation mail.
func TestMeetingBriefTellsWhatTheCompanyAlreadyReceivedFromOtherContacts(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	company := companyIDOf(e.SeedCompany(t, "Northwind", &e.Rep1))
	earlier := e.SeedContact(t, "Ana Roth", &e.Rep1)
	newContact := e.SeedContact(t, "Newcomer Lee", &e.Rep1)
	shareCompanyEmployee(t, e, company, earlier, false)
	shareCompanyEmployee(t, e, company, newContact, false)

	info := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, direction, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Outdated terms', 'outbound', $2, 'manual', 'human:x')`, roomAgo(14*24*time.Hour))
	LinkActivity(t, owner, info, "contact", earlier)
	booking := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, direction, occurred_at, source, captured_by,
			sender_is_machine, calendar_method)
		VALUES ($1, 'email', 'Booking confirmed', 'inbound', $2, 'manual', 'human:x', true, 'REQUEST')`, roomAgo(10*24*time.Hour))
	LinkActivity(t, owner, booking, "contact", earlier)

	shareContactForWriting(t, e, earlier)
	when := roomAgo(6 * 24 * time.Hour)
	hidden, _, err := e.Activities.LogActivity(
		e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms),
		activities.LogActivityInput{
			Kind: "meeting", Subject: new("Board compensation review"),
			OccurredAt: &when, Source: "manual", MeetingStatus: new(meetingoutcome.Held),
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: earlier}},
		})
	if err != nil {
		t.Fatalf("log the limited meeting: %v", err)
	}
	if _, err := e.Activities.SetAudience(
		e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms),
		ids.From[ids.ActivityKind](ids.UUID(hidden.Id)), activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limit the meeting: %v", err)
	}

	meeting := SeedIDRow(t, owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'meeting', 'Intro call', $2, 'manual', 'human:x')`, roomTomorrow)
	LinkActivity(t, owner, meeting, "contact", newContact)
	seatInRoom(t, owner, e.WS, meeting, newContact)

	text, omitted := companyContextOf(t, e, meeting)
	if !strings.Contains(text, "Outdated terms") || !strings.Contains(text, "Ana Roth") {
		t.Errorf("company context = %q, want the colleague contact's earlier mail named by contact", text)
	}
	if strings.Contains(text, "Board compensation") {
		t.Errorf("company context = %q, but this reader may not open that meeting", text)
	}
	if strings.Contains(text, "Booking confirmed") {
		t.Errorf("company context = %q, but a booking confirmation is not an exchange", text)
	}
	if !omitted {
		t.Error("the conversation the reader may not open was not counted in the history omission")
	}
}

// A quoted line is credited to the writer who wrote it. A note the reader
// logged names the reader as its author, whatever the contact it sits on.
func TestVoicesByIDNameTheReaderAsTheAuthorOfTheirOwnNote(t *testing.T) {
	e := Setup(t)
	contact := e.SeedContact(t, "Ana Roth", &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	note, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Subject: new("Could be customer and partner"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log the note: %v", err)
	}
	var voices map[ids.UUID]*crmcontracts.ActivityVoice
	if err := database.WithTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		voices, err = activities.VoicesByIDBatch(rep, tx, []ids.UUID{ids.UUID(note.Id)}, roomFixedNow)
		return err
	}); err != nil {
		t.Fatalf("VoicesByIDBatch: %v", err)
	}
	speaker := contactbrief.SpeakerFor(voices[ids.UUID(note.Id)])
	if speaker == nil || speaker.Kind != contactbrief.SpeakerYou {
		t.Errorf("speaker = %+v, want you", speaker)
	}
}
