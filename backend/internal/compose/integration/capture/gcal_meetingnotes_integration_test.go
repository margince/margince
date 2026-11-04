// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

const (
	notesFileID      = "1AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	transcriptFileID = "1TrAnScRiPt0123456789abcdefghijklmnop"
	agendaFileID     = "1AgEnDa0123456789abcdefghijklmnopqrst"
	googleDocType    = "application/vnd.google-apps.document"
)

// withNotes is a Meet event carrying the attachments Google adds once the call
// ends: its notes, its transcript, and an agenda somebody attached by hand.
func withNotes(event map[string]any, attachments ...map[string]any) map[string]any {
	event["conferenceData"] = map[string]any{
		"conferenceSolution": map[string]any{"key": map[string]any{"type": "hangoutsMeet"}},
	}
	event["attachments"] = attachments
	return event
}

func driveFile(id, title string) map[string]any {
	return map[string]any{
		"fileId": id, "title": title, "mimeType": googleDocType,
		"fileUrl": "https://drive.google.com/open?id=" + id,
	}
}

func notesRows(t *testing.T, e *integration.SearchEnv, meetingID string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM attachment WHERE entity_type = 'activity' AND entity_id = '`+
		meetingID+`' AND category = 'link'`)
}

// meetingUpdates counts the activity.updated events announced for one meeting.
func meetingUpdates(t *testing.T, e *integration.SearchEnv, meetingID string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'activity.updated' AND envelope->'entity'->>'id' = '`+meetingID+`'`)
}

// Google attaches the notes and transcript after the call. A later sync of the
// event records each as a document link on the meeting, once, and announces the
// change. An agenda attached by hand is not one of them. Syncing again adds
// nothing.
func TestTheNotesGoogleAttachesToAMeetingAreRecordedOnIt(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seatFor(t, e, guestColleagueA)
	ensureGuestContact(t, e, guestKnown)
	feed := &calendarFeed{}
	start := captureNow.AddDate(0, 0, -3)
	feed.set(withNotes(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...)))
	sync := connectFeed(t, env, feed)
	sync()
	meeting := gcalMeeting(t, e)
	if got := notesRows(t, e, meeting.String()); got != 0 {
		t.Fatalf("%d document links before Google attached anything, want 0", got)
	}
	updatesBefore := meetingUpdates(t, e, meeting.String())

	feed.set(withNotes(lateGuestEvent("review", "confirmed", start, guestColleagueA, guestList()...),
		driveFile(notesFileID, "Kick-off review - 2026/10/30 10:00 CET - Notes by Gemini"),
		driveFile(transcriptFileID, "Kick-off review - Transcript"),
		driveFile(agendaFileID, "Agenda for the kick-off"),
	))
	sync()
	sync()

	if got := notesRows(t, e, meeting.String()); got != 2 {
		t.Fatalf("%d document links on the meeting, want the notes and the transcript", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM attachment WHERE entity_id = '`+meeting.String()+
		`' AND url = 'https://docs.google.com/document/d/`+notesFileID+`/edit' AND bytes_withheld AND storage_key = ''`); got != 1 {
		t.Errorf("the notes link: %d rows at the document's own address with no bytes, want 1", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM attachment WHERE url LIKE '%`+agendaFileID+`%'`); got != 0 {
		t.Errorf("the hand-attached agenda was recorded %d times, want never", got)
	}
	if got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = '`+meeting.String()+
		`' AND action = 'update' AND (after ->> 'meeting_documents_added')::int = 2`); got != 1 {
		t.Errorf("%d audit rows record the two documents on the meeting, want 1", got)
	}
	if got := meetingUpdates(t, e, meeting.String()) - updatesBefore; got != 1 {
		t.Errorf("%d activity.updated events after the documents arrived, want 1", got)
	}
}

// A meeting not held on Google Meet has no Google notes, so a document titled
// like one is not read.
func TestADocumentOnAMeetingOffGoogleMeetIsNotRecorded(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seatFor(t, e, guestColleagueA)
	ensureGuestContact(t, e, guestKnown)
	feed := &calendarFeed{}
	event := lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...)
	event["attachments"] = []map[string]any{driveFile(notesFileID, "Kick-off review - Notes by Gemini")}
	feed.set(event)
	sync := connectFeed(t, env, feed)
	sync()

	if got := notesRows(t, e, gcalMeeting(t, e).String()); got != 0 {
		t.Errorf("%d document links on a meeting off Google Meet, want 0", got)
	}
}

// An event first synced after Google attached its notes records them with the
// meeting, under the meeting's own capture event.
func TestNotesPresentAtFirstSyncAreRecordedWithTheMeeting(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	seatFor(t, e, guestColleagueA)
	ensureGuestContact(t, e, guestKnown)
	feed := &calendarFeed{}
	feed.set(withNotes(lateGuestEvent("review", "confirmed", captureNow.AddDate(0, 0, -3), guestColleagueA, guestList()...),
		driveFile(notesFileID, "Kick-off review - Notes by Gemini")))
	sync := connectFeed(t, env, feed)
	sync()

	meeting := gcalMeeting(t, e)
	if got := notesRows(t, e, meeting.String()); got != 1 {
		t.Fatalf("%d document links on the meeting, want the notes", got)
	}
	if got := meetingUpdates(t, e, meeting.String()); got != 0 {
		t.Errorf("%d activity.updated events on a new meeting, want none: activity.captured covers it", got)
	}
}
