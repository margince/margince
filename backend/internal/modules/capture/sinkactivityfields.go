// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// occurredPrecision is the activity column's word for DateOnly.
func (f ActivityFields) occurredPrecision() string {
	if f.DateOnly {
		return string(crmcontracts.OccurredPrecisionOccurredPrecisionDay)
	}
	return string(crmcontracts.OccurredPrecisionOccurredPrecisionInstant)
}

// settledFields fills in what the capture does not take as sent. That is the
// time and its day anchor, the storable length, and the envelope's judgement.
func settledFields(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields) (ActivityFields, error) {
	var err error
	if fields.DateOnly {
		if fields.OccurredAt, err = dayAnchor(fields.OccurredAt); err != nil {
			return fields, err
		}
	}
	fields.OccurredAt = defaultOccurredAt(fields.OccurredAt)
	fields.DurationSeconds = StorableDuration(fields.DurationSeconds)
	fields.judged, err = judgeEnvelopeTx(ctx, tx, rec, fields)
	return fields, err
}

// defaultOccurredAt fills a provider payload that carried no timestamp.
// Capture time is the fallback, because a coarse "when we saw it" beats a zero
// time that sorts the record to the beginning of history. A stamp outside the
// storable range is no timestamp either, since no zone can render it.
func defaultOccurredAt(occurredAt time.Time) time.Time {
	if occurredAt.IsZero() || !datasource.InstantInRange(occurredAt) {
		return time.Now().UTC()
	}
	return occurredAt
}
