// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Google's meeting-notes mail: the notes Gemini writes and the list of a
// meeting's records. Both are about a meeting, not about the colleague who
// received them. So the mail is held to its participants and filed under no
// contact. The text of a Gemini mail goes to the activities module, which lands
// it as the transcript of the meeting the mail names.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// actionMeetingNotesMailFault is the system_log action of a notes mail whose
// transcript could not be landed. The mail itself is still kept.
const actionMeetingNotesMailFault = "capture_meeting_notes_mail_fault"

// meetingNotesMailSenders are the Google senders whose mail is about a meeting.
// The calendar sender is left out: an invitation is not notes.
var meetingNotesMailSenders = []string{geminiNotesSender, meetingRecordsSender}

// MeetingNotesMail is a Gemini notes mail, for the meeting it names.
type MeetingNotesMail struct {
	// MessageID is the mail's own identity, which keys the transcript it becomes.
	MessageID string
	Owner     ids.UserID
	Subject   string
	SentAt    time.Time
	Text      string
}

// isMeetingNotesMail reports whether rec is authenticated Google mail about a
// meeting. The sender stamp is what makes the address count.
func isMeetingNotesMail(rec connector.NormalizedRecord) bool {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != kindEmail || !recordIsGoogleMeetingMail(rec, fields) {
		return false
	}
	return slices.Contains(meetingNotesMailSenders, correspondence.Fold(rec.Counterparty.Email))
}

// withNotesMailHold marks the decision for a notes mail. It creates no record
// and files the mail under no contact. The mail stays with its participants
// however the sender is judged.
func withNotesMailHold(rec connector.NormalizedRecord, decision counterpartyDecision) counterpartyDecision {
	if isMeetingNotesMail(rec) {
		decision.create, decision.linkOnly, decision.notesMail = false, false, true
	}
	return decision
}

// landNotesMail hands a Gemini notes mail to the meeting it names. A fault rolls
// back to a savepoint and is logged, and the mail is kept.
func (s *Sink) landNotesMail(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields, decision counterpartyDecision,
) error {
	owner := actorUserID(ctx)
	if !decision.notesMail || s.meetingNotes == nil || owner.IsZero() ||
		correspondence.Fold(rec.Counterparty.Email) != geminiNotesSender {
		return nil
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return fmt.Errorf("capture: opening the notes-mail savepoint: %w", err)
	}
	_, landErr := s.meetingNotes.LandMailTx(ctx, sp, MeetingNotesMail{
		MessageID: rec.NaturalKey.SourceID, Owner: ids.From[ids.UserKind](owner),
		Subject: fields.Subject, SentAt: fields.OccurredAt, Text: fields.Body,
	})
	if landErr == nil {
		return sp.Commit(ctx)
	}
	if rbErr := sp.Rollback(ctx); rbErr != nil {
		return errors.Join(landErr, rbErr)
	}
	if storekit.IsLockCycle(landErr) {
		return landErr
	}
	return s.logBreadcrumbTx(ctx, tx, actionMeetingNotesMailFault, rec, landErr.Error())
}
