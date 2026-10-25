// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two meeting seams and the rows they answer with. Together rather than
// among the other lane interfaces because they are one subject asked from two
// sides — the same appointments, before and after they start — so a change to
// one that missed the other narrows a day on one lane and not the next.

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Meetings is today's booked meetings that have not happened yet. Optional: nil
// means this feed does not read meetings, which is not a day with none in it.
//
// Scoped like the task lane, because a meeting is somebody's — off one seat's
// calendar, or naming them as a participant. Answering it unscoped puts one
// seat's appointments in every colleague's brief.
type Meetings interface {
	Today(ctx context.Context, window MeetingWindow, limit int, scope TaskScope, owner ids.UUID) ([]Meeting, error)
}

// MeetingWindow is the stretch a meetings read covers: a timed meeting starting
// in [From, Until), and a day meeting whose anchor is in [DaysFrom, DaysUntil).
// A day has no hour to fall inside the first range.
type MeetingWindow struct {
	From, Until         time.Time
	DaysFrom, DaysUntil time.Time
}

// MeetingsAwaitingOutcome is today's meetings that have started and whose result
// nobody has recorded. A separate seam rather than a second method on Meetings,
// because every stub would otherwise have to answer both. Scoped exactly as
// Meetings is, or whichever half was left open leaks.
type MeetingsAwaitingOutcome interface {
	Since(ctx context.Context, window MeetingWindow, limit int, scope TaskScope, owner ids.UUID) ([]MeetingAwaitingOutcome, error)
}

// MeetingAwaitingOutcome is one appointment that happened and owes an answer. It
// carries less than Meeting because a meeting still ahead offers preparation;
// this one offers recording what happened, which needs the activity and who it
// was with.
type MeetingAwaitingOutcome struct {
	ID      ids.UUID
	Subject string
	// Not a due moment: a meeting that happened cannot be late, and an overdue
	// mark on it would contradict the row's whole point.
	StartedAt time.Time
	// Precision says StartedAt is a date's anchor when it is day.
	Precision crmcontracts.OccurredPrecision

	// What the answer is pinned to: two readers answering one meeting would
	// otherwise both succeed, the later winning silently. A pointer because a
	// row read back without one takes no conditional write, and the client must
	// be told that rather than shown a zero it would send as a real version.
	Version *int64

	// Whose calendar this came off, and what the row names as its owner. Zero
	// when no calendar claims it, and the row then names nobody rather than
	// guessing.
	HostUserID ids.UUID

	// Who the meeting was with: the same counterparty Meeting.ContactID names.
	// Zero for an internal meeting, or one whose outside attendees are withheld.
	ContactID ids.UUID
}

// Meeting is one appointment still ahead of the reader.
type Meeting struct {
	ID       ids.UUID
	Subject  string
	StartsAt time.Time
	// Precision says StartsAt is a date's anchor when it is day.
	Precision crmcontracts.OccurredPrecision

	// Who the meeting is with, and so whose page the brief is read on. The
	// brief opens as `?prep=<activity>` on a CONTACT's record, so the activity
	// id names the meeting and says nothing about where to read it. Zero for an
	// internal meeting, or one whose outside attendees are withheld: the row
	// then offers no verb rather than a link to a page picked at random.
	ContactID ids.UUID

	// True when nothing has been written down for a meeting about to happen.
	// Three-state, with the guard below: a meeting whose content this reader may
	// not read also arrives empty, and calling that "needs prep" would tell a rep
	// to prepare a meeting they cannot see.
	NeedsPrep bool

	// Whether NeedsPrep was answerable at all; false when content is withheld.
	PrepKnown bool

	// Whose calendar this came off, and what the row names as its owner. Zero
	// when no calendar claims it, and the row then names nobody rather than
	// guessing.
	HostUserID ids.UUID
}

// precisionOf is a row's wire precision: what the lane said, else an instant.
func precisionOf(p *crmcontracts.OccurredPrecision) crmcontracts.OccurredPrecision {
	if p == nil || *p == "" {
		return crmcontracts.OccurredPrecisionOccurredPrecisionInstant
	}
	return *p
}

// dayOnly carries a day precision onto a lane item, and nothing for an
// instant, which the item's absent field already says.
func dayOnly(p crmcontracts.OccurredPrecision) *crmcontracts.OccurredPrecision {
	if p != crmcontracts.OccurredPrecisionOccurredPrecisionDay {
		return nil
	}
	return &p
}
