// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The notes Gemini mails to a meeting's guests, landed as the transcript of
// that meeting. They reach the transcript reading as a notes Doc does
// (meetingnotes.go), so their next steps become proposals through one path.
// Capture decides the mail is Google's own. This side finds the meeting it
// names and writes the transcript.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// meetingNotesMailSource is the source of a transcript landed from a notes mail.
const meetingNotesMailSource = "meeting_notes_mail"

// notesMailKeyPrefix starts the source_id of a transcript landed from a notes mail.
const notesMailKeyPrefix = "mail:"

// notesMailTranscriptSQL tests that the transcript at alias was landed from a
// notes mail, also before the correction task reached it. A Doc's notes are
// captured as well, and their source_id is the Doc's link.
func notesMailTranscriptSQL(alias string) string {
	return fmt.Sprintf("(%[1]s.source IN ('%[2]s', '%[3]s') AND starts_with(%[1]s.source_id, '%[4]s') AND split_part(%[1]s.captured_by, ':', 1) = 'connector')",
		alias, meetingNotesMailSource, provenance.RecordSourceCapture, notesMailKeyPrefix)
}

// meetingNotesMailRawKey preserves which mail the transcript was read from.
const meetingNotesMailRawKey = "notes_mail"

// notesMailWindow is how long before a notes mail its meeting may have started.
// Gemini sends the notes after the call, on the day it was held.
const notesMailWindow = 24 * time.Hour

// MeetingNotesMailInput is one Gemini notes mail, for the meeting it names.
type MeetingNotesMailInput struct {
	// MessageID is the mail's own identity and keys the transcript it becomes.
	MessageID string
	Owner     ids.UserID
	Subject   string
	SentAt    time.Time
	Text      string
}

// notesSubject is Gemini's subject: Notes: "<title>" <date>. The title sits
// between the first opening quote and the last closing one, straight or curly.
var notesSubject = regexp.MustCompile(`^\s*Notes:\s*["“”](.+)["“”][^"“”]*$`)

// notesMailTitle is the meeting title a Gemini subject names, with whitespace
// folded and case dropped. A subject in any other shape names no meeting.
func notesMailTitle(subject string) (string, bool) {
	found := notesSubject.FindStringSubmatch(subject)
	if found == nil {
		return "", false
	}
	title := strings.ToLower(strings.Join(strings.Fields(found[1]), " "))
	return title, title != ""
}

// notesMailMeetingSQL lists the live meetings the mailbox owner is on, and may
// read. Each has the title the subject names, compared whole. Each started
// within notesMailWindow before the mail. A transcript is not a meeting.
func notesMailMeetingSQL(ctx context.Context, in MeetingNotesMailInput, title string) (string, []any, error) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return "", nil, err
	}
	titleArg, ownerArg := "$"+strconv.Itoa(arg(title)), "$"+strconv.Itoa(arg(in.Owner))
	from := "$" + strconv.Itoa(arg(in.SentAt.Add(-notesMailWindow))) + "::timestamptz"
	until := "$" + strconv.Itoa(arg(in.SentAt.Add(time.Second))) + "::timestamptz"
	return `
	SELECT a.id FROM activity a
	 WHERE a.kind = 'meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
	   AND a.source_system IS DISTINCT FROM '` + transcriptSourceSystem + `'
	   AND lower(regexp_replace(btrim(coalesce(a.subject, '')), '\s+', ' ', 'g')) = ` + titleArg + `
	   AND ` + occurredday.WithinSQL("a", from, until) + `
	   AND EXISTS (SELECT 1 FROM activity_participant o WHERE o.activity_id = a.id AND o.user_id = ` + ownerArg + `
	        AND ` + auth.ParticipantStandsSQL("o", "a") + `)
	   AND ` + scope + `
	 ORDER BY a.occurred_at DESC, a.id
	 LIMIT 2`, args, nil
}

// notesMailMeetingTx answers the one meeting a notes mail names. Two candidates
// name none: the mail cannot say which meeting it is about.
func notesMailMeetingTx(ctx context.Context, tx pgx.Tx, in MeetingNotesMailInput) (ids.ActivityID, bool, error) {
	title, ok := notesMailTitle(in.Subject)
	if !ok {
		return ids.ActivityID{}, false, nil
	}
	query, args, err := notesMailMeetingSQL(ctx, in, title)
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("activities: finding the meeting a notes mail names: %w", err)
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("activities: reading the meetings a notes mail may name: %w", err)
	}
	if len(candidates) != 1 {
		return ids.ActivityID{}, false, nil
	}
	return candidates[0], true, nil
}

// meetingHoldsTranscriptTx reports whether a live transcript already records
// the meeting. One transcript reads a meeting, so a second copy of its notes
// proposes every next step twice.
func meetingHoldsTranscriptTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity t JOIN activity m ON m.id = $1
		 WHERE `+recordingTranscriptSQL+`)`, meeting).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("activities: reading whether a meeting has a transcript: %w", err)
	}
	return held, nil
}

// LandMeetingNotesMailTx lands a Gemini notes mail as the transcript of the one
// meeting it names, and answers whether it landed. Nothing lands when no single
// meeting matches, the meeting already has a transcript, or the text is unusable.
//
// The transcript takes the meeting's audience, parties and contact links, as a
// notes Doc does. The meeting is locked before the transcript check, so two
// copies of one notes mail land one transcript.
func (s *Store) LandMeetingNotesMailTx(ctx context.Context, tx pgx.Tx, in MeetingNotesMailInput) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return false, err
	}
	body, err := normalizeTranscript(in.Text)
	if errors.Is(err, ErrBlankTranscript) || errors.As(err, new(oversizedTranscriptError)) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	id, found, err := notesMailMeetingTx(ctx, tx, in)
	if err != nil || !found {
		return false, err
	}
	key := notesMailKeyPrefix + in.MessageID
	if free, err := lockNotesMailMeetingTx(ctx, tx, in, id, key); err != nil || !free {
		return false, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
		return false, err
	}
	meeting, err := readActivityContent(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return false, err
	}
	subject := strings.TrimSpace(in.Subject)
	transcript := LogActivityInput{
		Kind: KindMeeting, Subject: &subject, Body: &body, OccurredAt: &meeting.OccurredAt,
		SourceSystem: new(transcriptSourceSystem), SourceID: &key,
		Source: meetingNotesMailSource, Links: linksOfKinds(meeting, []string{linkEntityContact}),
		recordsMeeting: &id, keepRecordedOutcome: true, continues: &id.UUID,
		Raw: &map[string]any{meetingNotesMailRawKey: map[string]any{"meeting_id": id.String()}},
		// Both are Google's own words, quoted rather than written by the product.
		SystemText: &systemtext.Message{
			Subject: systemtext.Prose(systemtext.Quote, ""), Body: systemtext.Prose(systemtext.Quote, ""),
		},
		audienceOf: &id,
	}
	_, created, err := s.LogActivityTx(ctx, tx, transcript)
	return created, err
}

// lockNotesMailMeetingTx takes the locks a landing needs and answers whether it
// may go on. The attendee lock comes before the row lock. The match was read
// before the locks, so it is asked again under them. A title, start or guest
// that changed since no longer names this meeting. A meeting with a transcript
// takes no second one.
func lockNotesMailMeetingTx(ctx context.Context, tx pgx.Tx, in MeetingNotesMailInput, id ids.ActivityID, key string) (bool, error) {
	if err := storekit.LockWriteIdentity(ctx, tx, "activity_source", transcriptSourceSystem+":"+key); err != nil {
		return false, err
	}
	if err := storekit.LockMeetingAttendeeChecks(ctx, tx, id.UUID); err != nil {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", id.UUID, storekit.LiveOnly); err != nil {
		return false, err
	}
	if again, still, err := notesMailMeetingTx(ctx, tx, in); err != nil || !still || again != id {
		return false, err
	}
	held, err := meetingHoldsTranscriptTx(ctx, tx, id)
	return !held, err
}
