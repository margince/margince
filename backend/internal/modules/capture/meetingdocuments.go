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
	"slices"
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
// event that lists all its documents archives the rows of those taken off it,
// and brings back a row it archived once the Doc is listed again. The notes
// reader then opens only a Doc the meeting carries now.
//
// A Doc counts only when the organizer is on a trusted own domain. A shared
// webmail domain is never one, so an outsider on the seat's own webmail
// cannot attach notes. A later change is audited on the meeting and announced
// as activity.updated; on the first capture activity.captured covers it.
func (s *Sink) recordMeetingDocuments(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, later bool,
) error {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != meetingKind || s.files == nil {
		return nil
	}
	links, listed := meetingDocumentLinks(fields)
	if len(links) > 0 {
		trusted, err := organizerTrustedTx(ctx, tx, rec)
		if err != nil {
			return err
		}
		if !trusted {
			links, listed = nil, nil
		}
	}
	from := FileSource{
		System: meetingDocumentSystem, MessageID: id.String(), CapturedBy: rec.CapturedBy, Category: fileCategoryFor(fields),
	}
	var change meetingDocumentChange
	var err error
	if len(links) > 0 {
		if change.added, err = s.files.RecordLinks(ctx, tx, id, from, links); err != nil {
			return fmt.Errorf("capture: recording a meeting's notes documents: %w", err)
		}
	}
	if later && fields.MeetingDocumentsListed {
		if change.removed, err = s.files.RetireLinks(ctx, tx, id, from, listed); err != nil {
			return fmt.Errorf("capture: archiving the notes documents taken off a meeting: %w", err)
		}
		if change.restored, err = s.files.RestoreLinks(ctx, tx, id, from, listed); err != nil {
			return fmt.Errorf("capture: restoring the notes documents listed on a meeting again: %w", err)
		}
	}
	if !later {
		return nil
	}
	return change.announceTx(ctx, tx, id)
}

// meetingDocumentLinks are the link rows the event's documents become, and
// their addresses.
func meetingDocumentLinks(fields ActivityFields) ([]MailLink, []string) {
	links := make([]MailLink, 0, len(fields.MeetingDocuments))
	addresses := make([]string, 0, len(fields.MeetingDocuments))
	for _, doc := range fields.MeetingDocuments {
		address := meetingDocumentURL(doc.FileID)
		links = append(links, MailLink{URL: address, Text: doc.Title})
		addresses = append(addresses, address)
	}
	return links, addresses
}

// organizerTrustedTx reports whether the meeting's organizer is on one of the
// workspace's trusted own domains, the answer the rest of capture gives to
// "is this our company".
func organizerTrustedTx(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	for _, party := range slices.Concat(rec.Participants, rec.WithheldParties) {
		if party.Role != connector.ParticipantRoleOrganizer {
			continue
		}
		trusted, err := TrustedOwnDomainsTx(ctx, tx)
		if err != nil {
			return false, fmt.Errorf("capture: reading the trusted own domains: %w", err)
		}
		return trusted.Covers(party.Email), nil
	}
	return false, nil
}

// meetingDocumentChange counts what one later sync changed on a meeting's
// notes links.
type meetingDocumentChange struct {
	added, removed, restored int
}

// announceTx audits and announces a change on the meeting, when there is one.
func (c meetingDocumentChange) announceTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	if c.added+c.removed+c.restored == 0 {
		return nil
	}
	after := map[string]any{"meeting_documents_added": c.added}
	if c.removed > 0 {
		after["meeting_documents_removed"] = c.removed
	}
	if c.restored > 0 {
		after["meeting_documents_restored"] = c.restored
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", id.UUID, after, nil)
	if err != nil {
		return fmt.Errorf("capture: auditing a meeting's notes documents: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{})
}
