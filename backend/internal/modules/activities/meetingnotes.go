// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The text of a meeting's notes Doc, landed as a transcript of that meeting so
// the transcript reading proposes the next steps in it. Capture decides which
// connection may read; this side holds the meeting, its document links and the
// transcript the text becomes.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingNotesSource is the source of a transcript landed from a notes Doc.
const meetingNotesSource = "meeting_notes"

// meetingNotesRawKey holds, in the transcript's raw, the meeting and the Doc
// revision it was read from. source_activity_id is not used: on a meeting it
// names the task an outcome answers.
const meetingNotesRawKey = "google_doc"

// MeetingNotesQuery asks for the notes links on one owner's meetings.
type MeetingNotesQuery struct {
	Owner        ids.UserID
	Domain       string
	Since, Until time.Time
	Limit        int
}

// MeetingNotesLink is one notes link on a meeting, by its address.
type MeetingNotesLink struct {
	Meeting ids.ActivityID
	URL     string
}

// MeetingNotesInput is one notes Doc's text for the meeting that carries it.
type MeetingNotesInput struct {
	Meeting  ids.ActivityID
	URL      string
	Owner    ids.UserID
	Domain   string
	Title    string
	Revision string
	Text     string
}

// meetingNotesReadable is the evidence a notes link must still have when it is
// listed and again when its text lands. The link is live on a live meeting.
// The owner is on that meeting, and its organizer is on the owner's own domain,
// because Google writes the notes into the organizer's Drive.
const meetingNotesReadable = `
	    l.entity_type = 'activity' AND l.entity_id = a.id AND l.category = '` + categoryLink + `'
	AND l.archived_at IS NULL AND l.external_source_id = '` + meetingDocumentSource + `' || a.id::text
	AND a.kind = 'meeting' AND a.archived_at IS NULL AND a.restricted_at IS NULL
	AND EXISTS (SELECT 1 FROM activity_participant o WHERE o.activity_id = a.id AND o.user_id = @owner)
	AND EXISTS (SELECT 1 FROM activity_participant g
	             WHERE g.activity_id = a.id AND g.role = 'organizer' AND g.address_domain_folded = @domain)`

// DueMeetingNotesTx answers the notes links on the owner's meetings in the
// window that no transcript holds yet, newest meeting first.
func (s *Store) DueMeetingNotesTx(ctx context.Context, tx pgx.Tx, q MeetingNotesQuery) ([]MeetingNotesLink, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT a.id, l.url FROM attachment l JOIN activity a ON a.id = l.entity_id
		 WHERE `+meetingNotesReadable+`
		   AND a.occurred_at >= @since AND a.occurred_at <= @until
		   AND NOT EXISTS (SELECT 1 FROM activity t
		                    WHERE t.source_system = @transcript AND t.source_id = l.url)
		 ORDER BY a.occurred_at DESC, l.id
		 LIMIT @limit`, pgx.NamedArgs{
		"owner": q.Owner, "domain": strings.ToLower(q.Domain), "since": q.Since, "until": q.Until,
		"transcript": transcriptSourceSystem, "limit": q.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("activities: listing the meeting notes due a reading: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (MeetingNotesLink, error) {
		var link MeetingNotesLink
		return link, row.Scan(&link.Meeting, &link.URL)
	})
}

// LandMeetingNotesTx lands one notes Doc's text as a transcript of its
// meeting, and answers whether it landed. The link's evidence is asked again
// under a lock on the meeting, which a later calendar sync takes to archive a
// removed Doc. The transcript is keyed by the Doc's address, so a Doc lands
// once whichever colleague's connection read it.
//
// A meeting not open to the whole workspace gives its transcript to the reader
// alone, so the text is never wider than the meeting.
func (s *Store) LandMeetingNotesTx(ctx context.Context, tx pgx.Tx, in MeetingNotesInput) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return false, err
	}
	var (
		occurredAt time.Time
		audience   string
		linkName   string
	)
	err := tx.QueryRow(ctx, `
		SELECT a.occurred_at, a.audience, l.filename FROM attachment l JOIN activity a ON a.id = l.entity_id
		 WHERE a.id = @meeting AND l.url = @url AND `+meetingNotesReadable+`
		   FOR SHARE OF a`, pgx.NamedArgs{
		"meeting": in.Meeting, "url": in.URL, "owner": in.Owner, "domain": strings.ToLower(in.Domain),
	}).Scan(&occurredAt, &audience, &linkName)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: checking the meeting still carries its notes: %w", err)
	}
	body, err := normalizeTranscript(in.Text)
	if errors.Is(err, ErrBlankTranscript) || errors.As(err, new(oversizedTranscriptError)) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	links, err := meetingLinksTx(ctx, tx, in.Meeting)
	if err != nil {
		return false, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = linkName
	}
	transcript := LogActivityInput{
		Kind: KindMeeting, Subject: &title, Body: &body, OccurredAt: &occurredAt,
		SourceSystem: new(transcriptSourceSystem), SourceID: &in.URL,
		Source: meetingNotesSource, Links: links,
		Raw: &map[string]any{meetingNotesRawKey: map[string]any{
			"meeting_id": in.Meeting.String(), "revision_id": in.Revision,
		}},
	}
	if audience != string(crmcontracts.ActivityAudienceWorkspace) {
		transcript.VisibleOnlyTo(in.Owner.UUID)
	}
	_, created, err := s.LogActivityTx(ctx, tx, transcript)
	return created, err
}

// meetingLinksTx is the records a meeting is filed under, so the tasks its
// transcript proposes are filed there too.
func meetingLinksTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID) ([]ActivityLinkInput, error) {
	rows, err := tx.Query(ctx, `
		SELECT entity_type, coalesce(contact_id, company_id, deal_id, lead_id, project_id)
		  FROM activity_link WHERE activity_id = $1 ORDER BY id`, meeting)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the records a meeting is filed under: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ActivityLinkInput, error) {
		var link ActivityLinkInput
		return link, row.Scan(&link.EntityType, &link.EntityID)
	})
}
