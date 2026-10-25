// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// RecordZone is the installation's zone, read inside the capture transaction.
type RecordZone func(ctx context.Context, tx pgx.Tx) (*time.Location, error)

// WithRecordZone returns a copy that stores a date-only capture at its date's
// anchor in zone, the instant every other door stores that date at.
func (s *Sink) WithRecordZone(zone RecordZone) *Sink {
	c := *s
	c.recordZone = zone
	return &c
}

// errNoRecordZone refuses a date-only capture on a sink built without a record
// zone. A guessed zone would file it under another date.
var errNoRecordZone = errors.New("capture: a date-only record needs the record zone, and this sink has none (WithRecordZone)")

// dayAnchor is where the date named's own year, month and day name is stored.
func (s *Sink) dayAnchor(ctx context.Context, tx pgx.Tx, named time.Time) (time.Time, error) {
	if s.recordZone == nil {
		return time.Time{}, errNoRecordZone
	}
	zone, err := s.recordZone(ctx, tx)
	if err != nil {
		return time.Time{}, err
	}
	if zone == nil {
		return time.Time{}, errNoRecordZone
	}
	return occurredday.Anchor(named, zone), nil
}
