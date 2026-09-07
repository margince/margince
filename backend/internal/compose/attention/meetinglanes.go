// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two meeting seams and the rows they answer with.
//
// They live together rather than among the other lane interfaces because they
// are one subject asked from two sides: the same appointments, before and after
// they start. Both take the same scope and both name the same host, and a
// change to one that missed the other is how a day's meetings would come to be
// narrowed on one lane and left open on the next.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Meetings is today's booked meetings that have not happened yet.
//
// Optional as the other two are: nil means this feed does not read meetings,
// which is not the same as a day with none in it.
//
// It takes the same scope the task lane does, for the same reason. A meeting is
// somebody's: it came off one seat's calendar, or it names them as a
// participant. Answering "today's meetings" with everybody's put one seat's
// appointments in every colleague's brief, which is how a rep read the line
// "a meeting with Lucy" about a meeting that was never hers.
type Meetings interface {
	Today(ctx context.Context, from, until time.Time, limit int, scope TaskScope, owner ids.UUID) ([]Meeting, error)
}

// MeetingsAwaitingOutcome is today's meetings that have started and whose
// result nobody has recorded.
//
// A separate seam from Meetings rather than a second method on it, because it
// asks the opposite question of the same table and every stub of that interface
// would otherwise have to answer both. Optional the same way: nil means this
// feed does not read them, which is not a day with none.
//
// Scoped exactly as Meetings is: the same rows, asked about from the other side
// of their start time, so a lane that narrowed only one of the two would leak
// through whichever half was left open.
type MeetingsAwaitingOutcome interface {
	Since(ctx context.Context, from, until time.Time, limit int, scope TaskScope, owner ids.UUID) ([]MeetingAwaitingOutcome, error)
}

// MeetingAwaitingOutcome is one appointment that happened and owes an answer.
//
// It carries less than Meeting because the two rows ask for different things: a
// meeting still ahead offers preparation, which needs a person's page and a
// readable body to judge. This one offers recording what happened, which needs
// the activity and nothing else — so there is no PersonID or prep tri-state
// here, and adding them would be fields no caller reads.
type MeetingAwaitingOutcome struct {
	ID      ids.UUID
	Subject string
	// StartedAt is when it began, which is in the past for every row here. It
	// is not a due moment: a meeting that happened cannot be late, and stamping
	// it as due would put an overdue mark on a row whose whole point is that
	// the meeting is over.
	StartedAt time.Time

	// Version is what the answer is pinned to. The verb this row offers writes
	// the activity, and two readers answering the same meeting would otherwise
	// both succeed with the later one winning silently.
	//
	// A pointer for the reason the task lane's is: a row read back without one
	// takes no conditional write, and the client must be told that rather than
	// shown a zero it would send as a real version.
	Version *int64

	// HostUserID is whose calendar this came off, and it is what the row names
	// as its owner. Zero when no calendar claims it — booked in the app, or
	// captured before the host was recorded — and the row then names nobody
	// rather than guessing.
	HostUserID ids.UUID
}

// Meeting is one appointment still ahead of the reader.
type Meeting struct {
	ID       ids.UUID
	Subject  string
	StartsAt time.Time

	// PersonID is whose page the brief is read on, and it is zero whenever the
	// meeting names nobody this reader may see.
	//
	// The brief is not a page of its own: it opens as `?prep=<activity>` on a
	// PERSON's record, so the activity id the row already carries names the
	// meeting and says nothing about where to read it. Without this the lane
	// could describe a meeting and offer no way to prepare for it, which is
	// the one thing a rep opens the row to do.
	//
	// An internal meeting legitimately has none, and so does one whose only
	// attendees are people the reader cannot read. Both stay zero and the row
	// offers no verb rather than a link to somebody's page picked at random.
	PersonID ids.UUID

	// NeedsPrep is true when nothing has been written down for a meeting that
	// is about to happen: no agenda or notes body, and nobody outside this
	// organization recorded on it.
	//
	// It is a THREE-state answer squeezed into a bool plus its guard below, and
	// the third state is why: a meeting whose content this reader may not read
	// arrives with an empty body for a reason that has nothing to do with
	// preparation. Calling that "needs prep" would tell a rep to prepare a
	// meeting they cannot see, so the lane leaves PrepKnown false instead and
	// the surface says nothing rather than something false.
	NeedsPrep bool

	// PrepKnown reports whether NeedsPrep was answerable at all. False when the
	// row's content is withheld from this reader.
	PrepKnown bool

	// HostUserID is whose calendar this came off, and it is what the row names as
	// its owner. Zero when no calendar claims it — booked in the app, or captured
	// before the host was recorded — and the row then names nobody rather than
	// guessing.
	HostUserID ids.UUID
}
