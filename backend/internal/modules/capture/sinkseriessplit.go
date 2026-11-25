// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SeriesSplitFolder folds the meetings a split series replaced at meeting's
// slot on seat's calendar onto the split series' meeting. activities owns the
// rule; compose injects it.
type SeriesSplitFolder func(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) error

// WithReplacedSeries returns a copy that folds the meetings a split series
// replaced.
func (s *Sink) WithReplacedSeries(fold SeriesSplitFolder) *Sink {
	c := *s
	c.replacedSeries = fold
	return &c
}

// foldReplacedSeries folds a split series' meetings at a new meeting's slot.
// Google sends nothing for the old series' instances a split replaced.
func (s *Sink) foldReplacedSeries(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, rec connector.NormalizedRecord) error {
	fields, isActivity := rec.Fields.(ActivityFields)
	if s.replacedSeries == nil || !isActivity || fields.Kind != meetingKind || !rec.CrossDoorIdentity.Stated() {
		return nil
	}
	return s.replacedSeries(ctx, tx, meeting, actorUserID(ctx))
}
