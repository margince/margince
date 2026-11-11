// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The edge between capture's meeting-notes reader and the timeline store that
// holds the meeting and the transcript its notes become.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// MeetingNotesReading is the enqueue a role sets as CaptureConfig's
// TranscriptEnqueue. The notes land inside a sync's own transaction, so an
// insert-only client is all it needs.
func MeetingNotesReading(log *slog.Logger) (activities.TranscriptReadEnqueue, error) {
	inserter, err := jobs.NewTxInserter(log)
	if err != nil {
		return nil, fmt.Errorf("compose: the meeting-notes reading enqueue: %w", err)
	}
	return TranscriptReadOnLanding(inserter), nil
}

// meetingNotesWriter adapts the timeline store to capture's notes seam.
type meetingNotesWriter struct {
	store *activities.Store
}

var _ capture.MeetingNotesWriter = meetingNotesWriter{}

func (w meetingNotesWriter) DueTx(ctx context.Context, tx pgx.Tx, q capture.MeetingNotesQuery) ([]capture.MeetingNotesLink, error) {
	due, err := w.store.DueMeetingNotesTx(ctx, tx, activities.MeetingNotesQuery{
		Owner: q.Owner, Domain: q.Domain, Since: q.Since, Until: q.Until, Limit: q.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]capture.MeetingNotesLink, 0, len(due))
	for _, link := range due {
		out = append(out, capture.MeetingNotesLink{Meeting: link.Meeting, URL: link.URL})
	}
	return out, nil
}

func (w meetingNotesWriter) LandTx(ctx context.Context, tx pgx.Tx, in capture.MeetingNotesLanding) (bool, error) {
	return w.store.LandMeetingNotesTx(ctx, tx, activities.MeetingNotesInput{
		Meeting: in.Meeting, URL: in.URL, Owner: in.Owner, Domain: in.Domain,
		Title: in.Title, Revision: in.Revision, Text: in.Text,
	})
}
