// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A meeting face to face, a card handed over at a stand included, qualifies the
// same mail whichever door records it. The doors are the answer to a refused
// send (RecordContext) and the source review of a contact (AssessNoticeSource).
// Both write it through recordFaceToFaceTx.
//
// It supports personal one-to-one follow-up mail and never marketing or a
// newsletter. It is written as an in_person exchange, which the follow-up
// validators read and the marketing claim does not.
//
// Only a meeting inside the reply window qualifies mail, as for any recorded
// exchange.
//
// A meeting owes no Art. 13 notice: the contact was told at the meeting who we
// are and that we will follow up. So a confirmed meeting settles the contact's
// source review with no duty and sends no notice (faceToFaceSettled). Settling
// is a privacy officer's write, so a rep without that grant leaves the review
// as it is.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// faceToFaceSource is the source kind a meeting face to face confirms. It owes
// no notice (DutyFor).
const faceToFaceSource = "in_person_permission"

// faceToFaceSettled is the resolution a source review settled by a meeting
// carries.
const faceToFaceSettled = "The information was already given at the meeting."

// faceToFaceMeeting is one meeting as a door states it. AboutDeal names the
// deal a refused send was about, or is zero.
type faceToFaceMeeting struct {
	entity    *ids.UUID
	note      string
	met       time.Time
	aboutDeal ids.UUID
}

// recordFaceToFaceTx writes a meeting face to face on the caller's
// transaction; the notice source and the source review both call it. It
// records the in_person exchange only while the meeting is inside the reply
// window the verdict reads (exchangeStillCountsTx). A row older than that would
// be written and never read.
func (s *Store) recordFaceToFaceTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, meeting faceToFaceMeeting) error {
	counts, _, err := exchangeStillCountsTx(ctx, tx, contactID, meeting.met, s.now())
	if err != nil || !counts {
		return err
	}
	_, err = s.recordQualifyingEventTx(ctx, tx, contactID, RecordQualifyingEventInput{
		Kind: kindInPerson, Note: meeting.note, OccurredAt: meeting.met, AboutDeal: meeting.aboutDeal,
		sendingEntity: meeting.entity,
	})
	return err
}

// exchangeStillCountsTx reports whether an exchange at occurredAt is inside
// the reply window of the contact's country, and that window. Not-before, as
// recordedQualifyingEvent reads `occurred_at >= since`.
func exchangeStillCountsTx(
	ctx context.Context, tx pgx.Tx, subject ids.ContactID, occurredAt, now time.Time,
) (bool, time.Duration, error) {
	country, err := recipientCountryTx(ctx, tx, ruleSubject{Kind: entityContact, ID: subject.String()})
	if err != nil {
		return false, 0, err
	}
	window := packRulesFor(country).reply
	return !occurredAt.Before(now.Add(-window)), window, nil
}
