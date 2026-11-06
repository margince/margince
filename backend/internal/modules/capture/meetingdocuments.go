// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The notes and transcripts a meeting carries, recorded as document links on
// the meeting. The same link rows a mail's document links become: a pointer to
// the document, no bytes, hanging off the meeting so its audience decides who
// sees it. Reading the text is a later step.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The kinds of meeting document Google writes.
const (
	MeetingDocumentNotes      = "notes"
	MeetingDocumentTranscript = "transcript"
)

// MeetingDocument is one notes or transcript document attached to a meeting.
type MeetingDocument struct {
	FileID string
	Title  string
	Kind   string
}

// meetingDocumentSystem names the meeting in a document row's identity. The
// meeting's own id follows it, so two seats syncing one meeting, or one seat
// syncing it twice, record each document once.
const meetingDocumentSystem = "meeting"

// meetingDocumentURL is the address of a Google Doc, built from its file id.
// Built rather than taken from the event, so one document has one address and
// one row however the provider spelled its link.
func meetingDocumentURL(fileID string) string {
	return "https://docs.google.com/document/d/" + fileID + "/edit"
}

// recordMeetingDocuments writes a link row on the meeting for each notes or
// transcript document the event lists. It runs when the meeting is first
// captured and on every later sync of it, because Google attaches the notes
// after the meeting ends.
//
// On a later sync a new row is a change to a meeting already announced, so it
// is audited on the meeting and announced as activity.updated. On the first
// capture the meeting's own activity.captured covers it. No keeper wired means
// no rows, as for mail.
func (s *Sink) recordMeetingDocuments(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, later bool,
) error {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != meetingKind || len(fields.MeetingDocuments) == 0 || s.files == nil {
		return nil
	}
	links := make([]MailLink, 0, len(fields.MeetingDocuments))
	for _, doc := range fields.MeetingDocuments {
		links = append(links, MailLink{URL: meetingDocumentURL(doc.FileID), Text: doc.Title})
	}
	from := FileSource{
		System: meetingDocumentSystem, MessageID: id.String(), CapturedBy: rec.CapturedBy, Category: fileCategoryFor(fields),
	}
	added, err := s.files.RecordLinks(ctx, tx, id, from, links)
	if err != nil {
		return fmt.Errorf("capture: recording a meeting's notes documents: %w", err)
	}
	if !later || added == 0 {
		return nil
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", id.UUID,
		map[string]any{"meeting_documents_added": added}, nil)
	if err != nil {
		return fmt.Errorf("capture: auditing a meeting's notes documents: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{})
}
