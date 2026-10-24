// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/stateddeadline"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// commitmentRead is the admission verdict together with who supplied the evidence.
type commitmentRead struct {
	Reason       activities.CommitmentAdmissionReason
	LoggedByHand bool
}

// proposeLate is true when the horizon stopped a promise whose evidence a
// colleague logged by hand: it is put to its promiser instead of filed as history.
func (read commitmentRead) proposeLate() bool {
	return read.Reason == activities.CommitmentHistorical && read.LoggedByHand
}

// proposeLate adds the promise's own conditions to the evidence's: ours, from
// the original source, not private mail, and not withheld on its meaning.
func (c Commitment) proposeLate(read commitmentRead) bool {
	return read.proposeLate() && c.Theirs == nil && c.PrivateTo == nil && c.AdmissionSourceID == nil && c.Meaning.withheldOutcome() == ""
}

func (d *CommitmentDispatcher) admission(ctx context.Context, tx pgx.Tx, c Commitment) (commitmentRead, error) {
	var due *time.Time
	if c.DueDate != "" {
		at, err := commitmentDueInstant(ctx, tx, c.DueDate, c.DueZone)
		if err != nil {
			return commitmentRead{}, err
		}
		due = &at
	}
	return readCommitmentEvidence(ctx, tx, c, due, true)
}

func readCommitmentEvidence(ctx context.Context, tx pgx.Tx, c Commitment, due *time.Time, forDecision bool) (commitmentRead, error) {
	sourceID := c.SourceActivityID
	if c.AdmissionSourceID != nil {
		if _, err := activities.CommitmentAdmissionEvidenceTx(ctx, tx, activities.CommitmentAdmissionInput{
			SourceID: ids.From[ids.ActivityKind](sourceID), ReadMessages: c.ReadMessages, Preview: !forDecision,
		}); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return commitmentRead{Reason: activities.CommitmentSourceUnavailable}, nil
			}
			return commitmentRead{}, err
		}
		sourceID = *c.AdmissionSourceID
	}
	evidence, err := activities.CommitmentAdmissionEvidenceTx(ctx, tx, activities.CommitmentAdmissionInput{
		SourceID: ids.From[ids.ActivityKind](sourceID), ReadMessages: c.ReadMessages, Preview: !forDecision,
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return commitmentRead{Reason: activities.CommitmentSourceUnavailable}, nil
	}
	if err != nil {
		return commitmentRead{}, err
	}
	if evidence.OccurredAt, err = commitmentSaidAt(ctx, tx, evidence); err != nil {
		return commitmentRead{}, err
	}
	read := commitmentRead{Reason: activities.AdmitAutomaticCommitment(evidence, c.AsOf, due), LoggedByHand: evidence.LoggedByHand}
	if read.Reason != activities.CommitmentCurrent || !evidence.CapturedMail {
		return read, nil
	}
	read.Reason, err = mailboxBaselineAdmission(ctx, tx, c, sourceID, evidence.RawCaptureID)
	return read, err
}

// commitmentSaidAt is when the source was said: captured mail's own stated
// time, not when capture stored it.
func commitmentSaidAt(ctx context.Context, tx pgx.Tx, evidence activities.CommitmentAdmissionEvidence) (time.Time, error) {
	if evidence.CapturedMail && evidence.SourceOccurredAt != nil {
		return *evidence.SourceOccurredAt, nil
	}
	if evidence.CapturedMail {
		return commitmentOriginalTime(ctx, tx, evidence.RawCaptureID)
	}
	return evidence.OccurredAt, nil
}

// withStatedWeek gives a promise made "this week" the last working day of the
// week it was said in. The words are the deadline quote the meaning validator
// already found in the source; any other range stays undated.
func withStatedWeek(ctx context.Context, tx pgx.Tx, c Commitment) (Commitment, error) {
	if c.DueDate != "" || c.Meaning == nil || c.Meaning.DeadlinePrecision != commitmentPrecisionRange {
		return c, nil
	}
	// The words stating the deadline are in the message being read: the
	// renewal when one renewed the promise, else the original.
	stated := c.SourceActivityID
	if c.AdmissionSourceID != nil {
		stated = *c.AdmissionSourceID
	}
	evidence, err := activities.CommitmentAdmissionEvidenceTx(ctx, tx, activities.CommitmentAdmissionInput{
		SourceID: ids.From[ids.ActivityKind](stated), ReadMessages: c.ReadMessages, Preview: true,
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	said, err := commitmentSaidAt(ctx, tx, evidence)
	if err != nil {
		return c, err
	}
	zone, workdays, err := promiserWeek(ctx, tx, c.Seat)
	if err != nil {
		return c, err
	}
	if day, ok := stateddeadline.ThisWeek(c.Meaning.DeadlineQuote, stateddeadline.SentDay(said, zone), workdays); ok {
		c.DueDate, c.DueZone = day.String(), zone.String()
	}
	return c, nil
}

// promiserWeek is the zone and working days of the colleague who made the
// promise, so "this week" ends on their own last working day. With no named
// promiser, the installation's zone and a Monday-to-Friday week apply.
func promiserWeek(ctx context.Context, tx pgx.Tx, seat *ids.UUID) (*time.Location, []int, error) {
	if seat == nil {
		zone, err := installationZoneTx(ctx, tx)
		return zone, nil, err
	}
	hours, _, err := identity.WorkingHoursOf(ctx, tx, ids.From[ids.UserKind](*seat))
	if errors.Is(err, apperrors.ErrNotFound) {
		zone, err := installationZoneTx(ctx, tx)
		return zone, nil, err
	}
	if err != nil {
		return nil, nil, err
	}
	zone, err := hours.Location()
	return zone, hours.Days, err
}

// mailboxBaselineAdmission withholds current mail work when the mailbox of the
// seat it would go to set a later history baseline. Another seat's baseline never applies.
//
// The date is the one that seat's own copy stated. Another seat's copy may carry
// an earlier Date header, so the shared occurrence time is not used.
func mailboxBaselineAdmission(ctx context.Context, tx pgx.Tx, c Commitment, sourceID ids.UUID, rawID *ids.UUID) (activities.CommitmentAdmissionReason, error) {
	holder := c.Seat
	if holder == nil {
		holder = c.PrivateTo
	}
	if holder == nil {
		return activities.CommitmentCurrent, nil
	}
	seen, err := capture.SeatHistoryBaselinesTx(ctx, tx, ids.From[ids.ActivityKind](sourceID), *holder)
	if err != nil {
		return "", err
	}
	var sentAt time.Time
	switch {
	case seen.SourceAt != nil:
		sentAt = *seen.SourceAt
	case seen.FirstCapture:
		if sentAt, err = commitmentOriginalTime(ctx, tx, rawID); err != nil {
			return "", err
		}
	}
	if sentAt.IsZero() {
		return activities.CommitmentCurrent, nil
	}
	excluded, err := seen.Excludes(sentAt)
	if err != nil || !excluded {
		return activities.CommitmentCurrent, err
	}
	return activities.CommitmentBeforeBaseline, nil
}

func commitmentOriginalTime(ctx context.Context, tx pgx.Tx, rawID *ids.UUID) (time.Time, error) {
	if rawID == nil {
		return time.Time{}, nil
	}
	payload, err := capture.GetRawCapturePayloadTx(ctx, tx, *rawID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return originalMessageTime(payload), nil
}

func originalMessageTime(payload []byte) time.Time {
	raw, err := capture.DecodeStoredOriginal(payload)
	if err != nil {
		return time.Time{}
	}
	message, err := mailmap.Parse(raw, "")
	if err != nil {
		return time.Time{}
	}
	fields, ok := message.ToRecord("", nil).Fields.(capture.ActivityFields)
	if !ok || !datasource.InstantInRange(fields.OccurredAt) {
		return time.Time{}
	}
	return fields.OccurredAt
}

func (d *CommitmentDispatcher) renewTask(ctx context.Context, tx pgx.Tx, c Commitment, reason activities.CommitmentAdmissionReason) (CommitmentOutcome, ids.UUID, error) {
	if c.Meaning.withheldOutcome() != "" || c.NeedsConfirmation || c.AdmissionSourceID == nil || c.DueDate == "" || c.Confidence < CommitmentTaskConfidence {
		return CommitmentRemembered, ids.UUID{}, nil
	}
	if reason != activities.CommitmentCurrent {
		return CommitmentRemembered, ids.UUID{}, nil
	}
	held, _, err := activities.CommitmentAdmissionStateTx(ctx, tx, ids.From[ids.ActivityKind](c.SourceActivityID), c.Locator, c.AsOf, c.AdmissionSourceID, c.RenewedAt)
	if err != nil || held {
		return CommitmentRemembered, ids.UUID{}, err
	}
	due, err := commitmentDueInstant(ctx, tx, c.DueDate, c.DueZone)
	if err != nil {
		return CommitmentRemembered, ids.UUID{}, err
	}
	changed, err := d.tasks.RenewCommitmentTaskTx(extractorContext(ctx, c.Extractor), tx, c.Locator, c.Summary, c.Body, due, c.PrivateTo)
	if err != nil || !changed {
		return CommitmentRemembered, ids.UUID{}, err
	}
	for _, recipient := range commitmentRecipients(c) {
		if _, _, err := d.fileClaim(extractorContext(ctx, c.Extractor), tx, c, recipient, claimKindOurs, reason); err != nil {
			return CommitmentRemembered, ids.UUID{}, err
		}
	}
	return CommitmentRetained, ids.UUID{}, nil
}

func (outcome CommitmentOutcome) wroteWork() bool {
	return outcome == CommitmentWatched || outcome == CommitmentTaskWritten || outcome == CommitmentProposed || outcome == CommitmentHistoricalProposed
}

func (outcome CommitmentOutcome) withheld() bool {
	return outcome != CommitmentRemembered && outcome != CommitmentRetained && !outcome.wroteWork()
}
