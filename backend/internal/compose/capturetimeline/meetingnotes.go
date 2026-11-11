// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capturetimeline

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// NotesWriter adapts the timeline store to capture's notes seam.
type NotesWriter struct {
	Store *activities.Store
}

var _ capture.MeetingNotesWriter = NotesWriter{}

// DueTx lists the meetings whose notes Doc is due a reading.
func (w NotesWriter) DueTx(ctx context.Context, tx pgx.Tx, q capture.MeetingNotesQuery) ([]capture.MeetingNotesLink, error) {
	due, err := w.Store.DueMeetingNotesTx(ctx, tx, activities.MeetingNotesQuery{
		Owner: q.Owner, Domains: q.Domains, Since: q.Since, Until: q.Until, Limit: q.Limit,
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

// LandTx lands one notes Doc as the meeting's transcript.
func (w NotesWriter) LandTx(ctx context.Context, tx pgx.Tx, in capture.MeetingNotesLanding) (bool, error) {
	return w.Store.LandMeetingNotesTx(ctx, tx, activities.MeetingNotesInput{
		Meeting: in.Meeting, URL: in.URL, Owner: in.Owner, Domains: in.Domains,
		Title: in.Title, Revision: in.Revision, Text: in.Text,
	})
}
