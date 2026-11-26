// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// settledFields fills in what the capture does not take as sent. That is the
// time and its day anchor, the storable length and subject, and the envelope's
// judgement.
func settledFields(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields) (ActivityFields, error) {
	var err error
	if fields.DateOnly {
		if fields.OccurredAt, err = dayAnchor(fields.OccurredAt); err != nil {
			return fields, err
		}
	}
	if fields.OccurredAt, err = settledOccurredAt(ctx, tx, fields.OccurredAt); err != nil {
		return fields, err
	}
	fields.DurationSeconds = StorableDuration(fields.DurationSeconds)
	fields.Subject = textbounds.Fit("public.activity.subject", fields.Subject)
	fields.judged, err = judgeEnvelopeTx(ctx, tx, rec, fields)
	return fields, err
}

// settledOccurredAt is the provider's instant, or for a payload without a
// usable one, the capture time held to the database's now() (UndatedSQL).
func settledOccurredAt(ctx context.Context, tx pgx.Tx, occurredAt time.Time) (time.Time, error) {
	fallback := defaultOccurredAt(occurredAt)
	if fallback.Equal(occurredAt) {
		return occurredAt, nil
	}
	var settled time.Time
	if err := tx.QueryRow(ctx, `SELECT `+occurredday.UndatedSQL("$1"), fallback).Scan(&settled); err != nil {
		return time.Time{}, fmt.Errorf("capture: dating an undated activity: %w", err)
	}
	return settled.UTC(), nil
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
