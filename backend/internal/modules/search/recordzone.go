// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// RecordZone is the installation's zone, read inside the caller's transaction.
type RecordZone func(ctx context.Context, tx pgx.Tx) (*time.Location, error)

// WithRecordZone binds the zone a day entry's date is read in, the zone every
// writer anchored it in.
func (s *Store) WithRecordZone(zone RecordZone) *Store {
	s.recordZone = zone
	return s
}

// WithRecordZone binds the record zone for this surface and its retriever.
func (h Handlers) WithRecordZone(zone RecordZone) Handlers {
	h.store.WithRecordZone(zone)
	return h
}

// errNoRecordZone refuses to date a day entry on a store built without the
// zone, since a guessed zone names another date.
var errNoRecordZone = errors.New("search: a date-only activity is dated in the record zone, and this store has none (WithRecordZone)")

func (s *Store) dayZone(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	if s.recordZone == nil {
		return nil, errNoRecordZone
	}
	zone, err := s.recordZone(ctx, tx)
	if err == nil && zone == nil {
		err = errNoRecordZone
	}
	return zone, err
}
