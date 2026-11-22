// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The notes Gemini mails to a meeting's guests belong to the meeting, not to the
// colleague who received them. Captured through the real Gmail path, an
// authenticated notes mail lands as the transcript of the one meeting it names.
// That transcript carries the meeting's contacts and readers and queues the one
// transcript reading. The mail itself is filed under no contact.

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	meetingTitle = "Kick-off review"
	notesBody    = "Summary\nAda will send the offer by Friday.\n"
)

// geminiNotesMail is the mail Google sends after a recorded meeting. authentic
// adds Gmail's DMARC pass for google.com.
func geminiNotesMail(msgID string, sent time.Time, authentic bool) []byte {
	return notesMailAbout(msgID, meetingTitle, captureOwner, "", sent, authentic)
}

// notesMailAbout is geminiNotesMail for another title, another To line and a
// subject prefix such as "Fwd: ".
func notesMailAbout(msgID, title, to, prefix string, sent time.Time, authentic bool) []byte {
	raw := []byte(strings.Join([]string{
		"From: Gemini Notes <gemini-notes@google.com>",
		"To: " + to,
		fmt.Sprintf("Subject: %sNotes: \"%s\" %s", prefix, title, sent.Format("Jan 2, 2006")),
		"Date: " + sent.Format(time.RFC1123Z),
		"Message-ID: <" + msgID + ">",
		"Content-Type: text/plain", "", notesBody,
	}, "\r\n"))
	if authentic {
		return authenticated(raw, "google.com")
	}
	return raw
}

// notesEnv is a capture harness whose transcript readings are counted, with
// the calendar meeting the notes are about already synced.
type notesEnv struct {
	env     captureEnv
	queued  *queuedReadings
	contact ids.ContactID
}

// syncedMeetings syncs one calendar event per title and start, each with the
// seat, a colleague and a known outside contact on it.
func syncedMeetings(t *testing.T, starts map[string]time.Time, title string) notesEnv {
	t.Helper()
	queued := &queuedReadings{}
	env := newCaptureEnvConfigured(t, captureNow, compose.CaptureConfig{TranscriptEnqueue: queued.enqueue})
	seatFor(t, env.e, guestColleagueA)
	contact := ensureGuestContact(t, env.e, guestKnown)
	ensureGuestContact(t, env.e, captureOwner)
	feed := &calendarFeed{}
	var items []map[string]any
	for id, start := range starts {
		event := lateGuestEvent(id, "confirmed", start, guestColleagueA, guestList()...)
		event["summary"] = title
		items = append(items, event)
	}
	feed.set(items...)
	connectFeed(t, env, feed)()
	return notesEnv{env: env, queued: queued, contact: contact}
}

func notesTranscriptCount(t *testing.T, e *integration.SearchEnv) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity WHERE source_system = 'transcript' AND source = 'meeting_notes_mail'`)
}

// contactLinks counts the contact links of the activity captured under sourceID.
func contactLinks(t *testing.T, e *integration.SearchEnv, system, sourceID string) int {
	t.Helper()
	return countRows(t, e, `SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE a.source_system = '`+system+`' AND a.source_id = '`+sourceID+`' AND l.contact_id IS NOT NULL`)
}

// One matching meeting: the notes become its transcript, filed on the meeting's
// contacts, with one reading queued. The mail is filed under nobody, so the
// receiver's own contact gathers no notes, and only its participants read it.
func TestAGeminiNotesMailLandsAsTheTranscriptOfItsMeeting(t *testing.T) {
	start := captureNow.Add(-4 * time.Hour)
	w := syncedMeetings(t, map[string]time.Time{"review": start}, meetingTitle)
	e := w.env.e
	meeting := gcalMeeting(t, e)

	w.env.sync(t, geminiNotesMail("notes1@google.com", start.Add(2*time.Hour), true))

	if got := notesTranscriptCount(t, e); got != 1 {
		t.Fatalf("%d transcripts from the notes mail, want 1", got)
	}
	var body, source string
	var recorded ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `
		SELECT body, source_id, source_activity_id FROM activity
		 WHERE source_system = 'transcript' AND source = 'meeting_notes_mail'`).Scan(&body, &source, &recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != meeting || source != "mail:notes1@google.com" || body != strings.TrimSuffix(notesBody, "\n") {
		t.Errorf("transcript of %s from %q with body %q, want the notes text recording meeting %s", recorded, source, body, meeting)
	}
	if got := countRows(t, e, `SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE a.source_system = 'transcript' AND l.contact_id = '`+w.contact.String()+`'`); got != 1 {
		t.Errorf("the meeting's contact has %d transcript links, want 1", got)
	}
	if len(w.queued.activity) != 1 {
		t.Errorf("%d transcript readings queued, want 1", len(w.queued.activity))
	}
	if got := contactLinks(t, e, "email", "notes1@google.com"); got != 0 {
		t.Errorf("the notes mail is filed under %d contacts, want none", got)
	}
	var audience string
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT audience FROM activity WHERE source_system = 'email' AND source_id = 'notes1@google.com'`).Scan(&audience); err != nil {
		t.Fatal(err)
	}
	if audience != "participants" {
		t.Errorf("the notes mail audience is %q, want participants", audience)
	}
}

// Two meetings match the subject, so the mail cannot say which it is about. It
// lands as mail held to its participants and no transcript is written.
func TestANotesMailNamingTwoMeetingsLandsNoTranscript(t *testing.T) {
	start := captureNow.Add(-5 * time.Hour)
	w := syncedMeetings(t, map[string]time.Time{"review": start, "review2": start.Add(time.Hour)}, meetingTitle)

	w.env.sync(t, geminiNotesMail("notes2@google.com", start.Add(3*time.Hour), true))

	if got := notesTranscriptCount(t, w.env.e); got != 0 {
		t.Errorf("%d transcripts from a mail that names two meetings, want none", got)
	}
	if got := contactLinks(t, w.env.e, "email", "notes2@google.com"); got != 0 {
		t.Errorf("the ambiguous notes mail is filed under %d contacts, want none", got)
	}
	if len(w.queued.activity) != 0 {
		t.Errorf("%d readings queued, want none", len(w.queued.activity))
	}
}

// The same From without Gmail's authentication is a sender's own text, so it is
// ordinary mail and reaches no meeting.
func TestAnUnauthenticatedNotesMailLandsNoTranscript(t *testing.T) {
	start := captureNow.Add(-4 * time.Hour)
	w := syncedMeetings(t, map[string]time.Time{"review": start}, meetingTitle)

	w.env.sync(t, geminiNotesMail("notes3@google.com", start.Add(2*time.Hour), false))

	if got := notesTranscriptCount(t, w.env.e); got != 0 {
		t.Errorf("%d transcripts from an unauthenticated mail, want none", got)
	}
}

// A colleague's copy of the same notes names the same meeting. One transcript
// reads a meeting, so the second copy proposes nothing twice.
func TestASecondNotesMailForTheSameMeetingLandsNoSecondTranscript(t *testing.T) {
	start := captureNow.Add(-4 * time.Hour)
	w := syncedMeetings(t, map[string]time.Time{"review": start}, meetingTitle)

	w.env.sync(t, geminiNotesMail("notes4@google.com", start.Add(2*time.Hour), true))
	w.env.sync(t, geminiNotesMail("notes5@google.com", start.Add(2*time.Hour), true))

	if got := notesTranscriptCount(t, w.env.e); got != 1 {
		t.Errorf("%d transcripts from two copies of the notes, want 1", got)
	}
	if len(w.queued.activity) != 1 {
		t.Errorf("%d readings queued, want 1", len(w.queued.activity))
	}
}

// Notes already landed from the mail are the meeting's transcript. The same
// notes in a Doc are not read again, or every next step is proposed twice.
func TestADocNotesReadingFollowsNoNotesMail(t *testing.T) {
	env := newCaptureEnv(t)
	meetingWithNotes(t, env)
	start := env.now.AddDate(0, 0, -3)
	env.sync(t, geminiNotesMail("notes6@google.com", start.Add(2*time.Hour), true))
	docs, queued := &docsFeed{}, &queuedReadings{}
	_, syncNotes := connectNotes(t, env, docs, captureOwner, queued)

	syncNotes()

	if got := docs.readCount(); got != 0 {
		t.Errorf("the notes Doc was opened %d times after the mail landed its notes, want none", got)
	}
	if got := notesTranscripts(t, env.e); got != 0 {
		t.Errorf("%d transcripts from the Doc, want none", got)
	}
	if got := notesTranscriptCount(t, env.e); got != 1 {
		t.Errorf("%d transcripts from the notes mail, want 1", got)
	}
}

// The subject names "Quarterly Review", and the meeting is called "Review". A
// title is matched whole, so the notes of one meeting never land on another.
func TestANotesMailLandsOnlyOnAMeetingWithTheWholeTitle(t *testing.T) {
	start := captureNow.Add(-4 * time.Hour)
	w := syncedMeetings(t, map[string]time.Time{"review": start}, "Review")

	w.env.sync(t, notesMailAbout("notes7@google.com", "Quarterly Review", captureOwner, "", start.Add(2*time.Hour), true))
	if got := notesTranscriptCount(t, w.env.e); got != 0 {
		t.Fatalf("%d transcripts on a meeting whose title is only part of the subject's, want none", got)
	}

	w.env.sync(t, notesMailAbout("notes8@google.com", "  review ", captureOwner, "", start.Add(2*time.Hour), true))
	if got := notesTranscriptCount(t, w.env.e); got != 1 {
		t.Errorf("%d transcripts from the exact title in other case and spacing, want 1", got)
	}
}

// A copy somebody forwarded, or one the owner only received as a Cc, is not
// Google's mail to the owner about the owner's own meeting.
func TestAForwardedNotesMailLandsNoTranscript(t *testing.T) {
	start := captureNow.Add(-4 * time.Hour)
	w := syncedMeetings(t, map[string]time.Time{"review": start}, meetingTitle)

	w.env.sync(t,
		notesMailAbout("notes9@google.com", meetingTitle, "other@client-co.example", "Fwd: ", start.Add(2*time.Hour), true),
		notesMailAbout("notes10@google.com", meetingTitle, "other@client-co.example\r\nCc: "+captureOwner, "", start.Add(2*time.Hour), true))

	if got := notesTranscriptCount(t, w.env.e); got != 0 {
		t.Errorf("%d transcripts from forwarded or copied notes, want none", got)
	}
}
