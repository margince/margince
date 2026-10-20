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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (d *CommitmentDispatcher) admission(ctx context.Context, tx pgx.Tx, c Commitment) (activities.CommitmentAdmissionReason, error) {
	var due *time.Time
	if c.DueDate != "" {
		at, err := commitmentDueInstant(ctx, tx, c.DueDate)
		if err != nil {
			return "", err
		}
		due = &at
	}
	sourceID := c.SourceActivityID
	if c.AdmissionSourceID != nil {
		if _, err := activities.CommitmentAdmissionEvidenceTx(ctx, tx, activities.CommitmentAdmissionInput{
			SourceID: ids.From[ids.ActivityKind](sourceID), ReadMessages: c.ReadMessages,
		}); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return activities.CommitmentSourceUnavailable, nil
			}
			return "", err
		}
		sourceID = *c.AdmissionSourceID
	}
	evidence, err := activities.CommitmentAdmissionEvidenceTx(ctx, tx, activities.CommitmentAdmissionInput{
		SourceID: ids.From[ids.ActivityKind](sourceID), ReadMessages: c.ReadMessages,
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return activities.CommitmentSourceUnavailable, nil
	}
	if err != nil {
		return "", err
	}
	if evidence.CapturedMail && evidence.SourceOccurredAt != nil {
		evidence.OccurredAt = *evidence.SourceOccurredAt
	} else if evidence.CapturedMail {
		evidence.OccurredAt, err = commitmentOriginalTime(ctx, tx, evidence.RawCaptureID)
		if err != nil {
			return "", err
		}
	}
	reason := activities.AdmitAutomaticCommitment(evidence, c.AsOf, due)
	if reason != activities.CommitmentCurrent || !evidence.CapturedMail {
		return reason, nil
	}
	return mailboxBaselineAdmission(ctx, tx, c, sourceID, evidence.RawCaptureID)
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
	due, err := commitmentDueInstant(ctx, tx, c.DueDate)
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
	return outcome == CommitmentWatched || outcome == CommitmentTaskWritten || outcome == CommitmentProposed
}

func (outcome CommitmentOutcome) withheld() bool {
	return outcome != CommitmentRemembered && outcome != CommitmentRetained && !outcome.wroteWork()
}
