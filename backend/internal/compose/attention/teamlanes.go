// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The readers the TEAM surfaces need, which the per-reader lanes cannot answer.
// Their own file because each asks about somebody other than the caller, so each
// carries its own reason for being safe to ask.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Teammates answers whether a named user is on a team with the reader. The
// reader is not passed — the module takes it from the principal — so the
// question can only be asked about an edge the asker is an end of. Asked only
// when a TEAM-scoped reader names somebody else's queue.
type Teammates interface {
	SharesLiveTeamWithCaller(ctx context.Context, other ids.UUID) (bool, error)
	// Enumerates what the method above answers one edge of. On ONE interface so
	// a binding cannot supply the yes/no half without the roster, which would
	// list a name the other read then refuses. The bool reports the roster was
	// CUT, so a board can say it shows part of a team rather than all of one.
	LiveTeammatesOfCaller(ctx context.Context) ([]TeamMember, bool, error)
}

// FormerTeammates lists the colleagues who left and still sit on a live team
// with the reader. A team view keeps their open work, marked as needing a new
// owner; unbound, that work is dropped as it was.
type FormerTeammates interface {
	FormerTeammatesOfCaller(ctx context.Context) ([]ids.UUID, error)
}

// TeamMember is one human seat on a team the caller may read.
type TeamMember struct {
	UserID      ids.UUID
	DisplayName string
	// Invited is a seat that has not signed in yet. Only a named team's roster
	// carries one; its workload is not measured.
	Invited bool
}

// OverdueLoad counts each contact's open tasks already past due. Its own reader
// rather than a bucketing of Tasks, because that lane's page stops at a dozen: a
// board counting its rows would report every loaded rep as holding exactly
// twelve. The waiting column needs no such reader — it buckets the same list the
// queue renders, which is what keeps the two agreeing.
//
// Optional: nil draws no overdue column, rather than zeros reading as a team
// that is up to date.
type OverdueLoad interface {
	OverduePerAssignee(ctx context.Context, asOf time.Time) (map[ids.UUID]int, error)
	CommitmentsDuePerAssignee(ctx context.Context, asOf time.Time) (map[ids.UUID]int, error)
}

// rosterRead is one roster answer: its members, and whether it stopped at its cap.
type rosterRead struct {
	members []TeamMember
	former  []ids.UUID
	cut     bool
}

func (r *rosterRead) seats() []ids.UUID { return teamSeats(r.members, r.former) }

// degradableRoster is the roster read for a page that names its failure: the
// team scope reports a refused roster as unavailable, so the read must not
// abort the snapshot on its way to saying so. A page whose scope already read
// it reuses that answer.
func (s *Service) degradableRoster(ctx context.Context) (roster []TeamMember, cut bool, err error) {
	if s.teamRoster != nil {
		return s.teamRoster.members, s.teamRoster.cut, nil
	}
	err = s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		roster, cut, err = s.teammates.LiveTeammatesOfCaller(ctx)
		return err
	})
	return roster, cut, err
}
