// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The readers the TEAM surfaces need, which the per-reader lanes cannot answer.
//
// Membership, and a count that is not a page. Their own file because they share
// one property the lanes beside them do not: each answers a question about
// somebody other than the caller, so each carries its own reason for being safe
// to ask.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Teammates answers whether a named user is on a team with the reader.
//
// The reader is the authenticated caller and is not passed: the module behind
// this takes it from the principal, so the question can only ever be asked
// about an edge the asker is themselves an end of.
//
// Asked only when a TEAM-scoped reader names somebody else's queue. An
// unbounded reader reaches every row and needs no such question; an own-scoped
// reader is refused before it is asked.
type Teammates interface {
	SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error)
	// LiveTeammatesOfCaller enumerates what the method above answers one edge
	// of. The two live on ONE interface so a binding cannot supply the yes/no
	// half without the roster: a board listing a name the other read then
	// refuses would show a manager a person they cannot open.
	//
	// The bool reports that the roster was CUT — more teammates exist than the
	// answer names — so a board can say it is showing part of a team rather
	// than presenting the cut as the whole of one.
	LiveTeammatesOfCaller(ctx context.Context) ([]TeamMember, bool, error)
}

// TeamMember is one live human seat sharing a live team with the caller.
type TeamMember struct {
	UserID      ids.UUID
	DisplayName string
}

// OverdueLoad counts each person's open tasks already past due.
//
// Its own reader rather than a bucketing of what Tasks returns, and the reason
// is the bound: the task lane's page stops at a dozen, so a board built by
// counting its rows would report every loaded rep as holding exactly twelve and
// say the whole team is equally busy.
//
// The waiting column needs no such reader. That lane returns every candidate it
// read and the board buckets the SAME list the queue renders, which is what
// keeps the two agreeing — a count taken from a second query would drift from
// the page it summarises the first time either changed.
//
// Optional: nil means the board draws no overdue column at all, rather than a
// column of zeros that would read as a team that is up to date.
type OverdueLoad interface {
	OverduePerAssignee(ctx context.Context, asOf time.Time) (map[ids.UUID]int, error)
}

// PromiseLoad counts each person's commitments due by an instant.
//
// Read from extracted claims, like the rep's own commitments lane, so the board
// and the rep's day count the same thing. A count taken from open tasks instead
// would be a different question wearing the same column heading: a task carries
// a date and no provenance, and a promise is a thing somebody SAID.
//
// Optional, and for a sharper reason than the others: an installation that
// extracts no claims binds nothing here, and a zero column would tell a lead
// their team promised nothing — when in truth nobody was listening.
// It takes the OWNERS to count rather than answering for everyone, because the
// promise store answers one owner at a time — the claim's owner is the owner of
// the PERSON it was made to, and that ladder lives in the store's own query. A
// board asking for a roster it already has is cheaper than a second aggregate
// restating who owns a promise.
type PromiseLoad interface {
	DuePerOwner(ctx context.Context, owners []ids.UUID, by time.Time) (map[ids.UUID]int, error)
}
