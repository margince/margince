// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ReplacedSeriesFinder answers the live meetings of seat's calendar that a
// split series replaced at meeting's slot. activities owns the rule; compose
// injects it.
type ReplacedSeriesFinder func(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, seat ids.UUID) ([]ids.ActivityID, error)

// WithReplacedSeries returns a copy that retires the meetings a split series
// replaced.
func (s *Sink) WithReplacedSeries(find ReplacedSeriesFinder) *Sink {
	c := *s
	c.replacedSeries = find
	return &c
}

// foldReplacedSeries folds the meetings a split series replaced onto the new
// meeting. Only the old row folds: the new row is the one later syncs of the
// series move and cancel. An old row that does not retire cleanly, such as
// one a human worked, stays, and the pair is logged.
func (s *Sink) foldReplacedSeries(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID, rec connector.NormalizedRecord) error {
	fields, isActivity := rec.Fields.(ActivityFields)
	if s.replacedSeries == nil || s.foldMeeting == nil || !isActivity || fields.Kind != meetingKind ||
		!rec.CrossDoorIdentity.Stated() {
		return nil
	}
	replaced, err := s.replacedSeries(ctx, tx, meeting, actorUserID(ctx))
	if err != nil {
		return err
	}
	for _, old := range replaced {
		folded, err := s.foldMeeting(ctx, tx, old, meeting, true)
		if err != nil {
			return err
		}
		if !folded {
			slog.WarnContext(ctx, "capture: a meeting split into a new series stays stored twice because the old row does not retire cleanly",
				"old", old.String(), "new", meeting.String())
		}
	}
	return nil
}
