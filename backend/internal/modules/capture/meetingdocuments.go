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
	"regexp"
	"strings"

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

// The two ends of a meeting document's address, around its file id.
const (
	meetingDocumentURLPrefix = "https://docs.google.com/document/d/"
	meetingDocumentURLSuffix = "/edit"
)

// driveFileID is the shape of a Drive file id. Anything else is refused, so
// the document address built from it cannot point anywhere but at that file.
var driveFileID = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)

// IsDriveFileID reports whether id has the shape of a Drive file id.
func IsDriveFileID(id string) bool {
	return driveFileID.MatchString(id)
}

// meetingDocumentURL is the address of a Google Doc, built from its file id.
// Built rather than taken from the event, so one document has one address and
// one row however the provider spelled its link.
func meetingDocumentURL(fileID string) string {
	return meetingDocumentURLPrefix + fileID + meetingDocumentURLSuffix
}

// meetingDocumentFileID is the file id in an address meetingDocumentURL built.
// Any other address answers false.
func meetingDocumentFileID(address string) (string, bool) {
	rest, prefixed := strings.CutPrefix(address, meetingDocumentURLPrefix)
	id, suffixed := strings.CutSuffix(rest, meetingDocumentURLSuffix)
	if !prefixed || !suffixed || !IsDriveFileID(id) {
		return "", false
	}
	return id, true
}

// recordMeetingDocuments writes a link row on the meeting for each notes or
// transcript document the event lists. It runs on every sync of the meeting,
// because Google attaches the notes after the meeting ends. A later sync of an
// event that lists all its documents archives the rows of those taken off it.
// The notes reader then opens only a Doc the meeting still carries.
//
// A later change is audited on the meeting and announced as activity.updated.
// On the first capture the meeting's own activity.captured covers it. No
// keeper wired means no rows, as for mail.
func (s *Sink) recordMeetingDocuments(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, later bool,
) error {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != meetingKind || s.files == nil {
		return nil
	}
	links := make([]MailLink, 0, len(fields.MeetingDocuments))
	kept := make([]string, 0, len(fields.MeetingDocuments))
	for _, doc := range fields.MeetingDocuments {
		address := meetingDocumentURL(doc.FileID)
		links = append(links, MailLink{URL: address, Text: doc.Title})
		kept = append(kept, address)
	}
	from := FileSource{
		System: meetingDocumentSystem, MessageID: id.String(), CapturedBy: rec.CapturedBy, Category: fileCategoryFor(fields),
	}
	added, removed := 0, 0
	var err error
	if len(links) > 0 {
		if added, err = s.files.RecordLinks(ctx, tx, id, from, links); err != nil {
			return fmt.Errorf("capture: recording a meeting's notes documents: %w", err)
		}
	}
	if later && fields.MeetingDocumentsListed {
		if removed, err = s.files.RetireLinks(ctx, tx, id, from, kept); err != nil {
			return fmt.Errorf("capture: archiving the notes documents taken off a meeting: %w", err)
		}
	}
	if !later || added+removed == 0 {
		return nil
	}
	after := map[string]any{"meeting_documents_added": added}
	if removed > 0 {
		after["meeting_documents_removed"] = removed
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", id.UUID, after, nil)
	if err != nil {
		return fmt.Errorf("capture: auditing a meeting's notes documents: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{})
}
