// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/stateddeadline"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// RecordZone is the installation's zone, read inside the caller's transaction.
type RecordZone func(ctx context.Context, tx pgx.Tx) (*time.Location, error)

// WithRecordZone wires the installation zone: where a stated deadline day ends
// and which date is today. Unbound, an automatic reminder stays undated, and an
// instant is refused a move to a day rather than dated in a guessed zone.
func (s *Store) WithRecordZone(zone RecordZone) *Store {
	s.recordZone = zone
	return s
}

// WithRecordZone wires the installation zone for this transport's store.
func (h Handlers) WithRecordZone(zone RecordZone) Handlers {
	h.store = h.store.WithRecordZone(zone)
	return h
}

// WithRecordZone wires the installation zone for the tool surface's store.
func (p *Provider) WithRecordZone(zone RecordZone) *Provider {
	p.store = p.store.WithRecordZone(zone)
	return p
}

// statedRequestDue is the end of the day the request's own words name as its
// deadline. It is false when none is stated or that day is over by asOf.
// Only the sender's current message is read: quoted history states someone
// else's deadline.
func (s *Store) statedRequestDue(ctx context.Context, tx pgx.Tx, source crmcontracts.Activity, asOf time.Time) (time.Time, bool, error) {
	if s.recordZone == nil {
		return time.Time{}, false, nil
	}
	zone, err := s.recordZone(ctx, tx)
	if err != nil {
		return time.Time{}, false, err
	}
	text := ""
	if source.Subject != nil {
		text = *source.Subject
	}
	if source.Body != nil {
		text += "\n" + textlang.CurrentMessage(*source.Body)
	}
	day, ok := stateddeadline.In(text, stateddeadline.SentDay(source.OccurredAt, zone))
	if !ok {
		return time.Time{}, false, nil
	}
	due := time.Date(day.Year, day.Month, day.Day, 23, 59, 59, 0, zone)
	return due, !deadline.Passed(&due, asOf), nil
}
