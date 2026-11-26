// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MeetingNotesDoc is one notes or transcript Doc a meeting carries, which the
// connection running the sync may read.
type MeetingNotesDoc struct {
	Meeting ids.UUID
	FileID  string
}

// MeetingNotesText is what a Doc said when it was read.
type MeetingNotesText struct {
	Title    string
	Revision string
	Text     string
}

// MeetingNotesKeeper is the Sink's meeting-notes verbs: which Docs the
// connection running the sync may read, and where their text lands. Separate
// from Sink for MeetingCanceller's reason: only the Google Docs connection asks.
type MeetingNotesKeeper interface {
	// MeetingNotesDue names the Docs to read now. A connection that is not on
	// the calendar's Google account, or holds no read grant, has none.
	MeetingNotesDue(ctx context.Context) ([]MeetingNotesDoc, error)
	// KeepMeetingNotes lands one Doc's text on its meeting. It checks again that
	// the meeting still carries the Doc, and lands one Doc once.
	KeepMeetingNotes(ctx context.Context, doc MeetingNotesDoc, notes MeetingNotesText) error
	// MeetingNotesRefused records that the provider refused the read grant, so
	// the connection asks its owner to connect again.
	MeetingNotesRefused(ctx context.Context) error
}
