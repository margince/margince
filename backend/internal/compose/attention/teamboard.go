// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// WHO on the team is carrying what.
//
// The ranked queue assembles ONE person's day, so widening its scope cannot
// produce a colleague's waiting mail: those sources were never read for anybody
// else, and scope.go says so where it explains what `team` and `all` can and
// cannot do. A board over that assembly would therefore be a page of the
// reader's own work wearing the team's name.
//
// So this reads COUNTS from the sources directly, and stays counts on purpose.
// A lead reading it picks a person and opens that person's own day through the
// queue's `owner` parameter, which is the read that already exists and already
// carries the authority check.

import (
	"context"
	"sort"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamBoard answers what each teammate is carrying.
//
// Refused outright below a row scope of `team`. An own-scoped reader has no
// team question to ask — the board would be one row, their own, which the queue
// already answers better — and offering it would advertise a surface whose every
// count is zero for them.
//
// The counts are read ONCE for the whole board and bucketed by owner here,
// rather than per member. Per member would be one query per person and would
// give a large team a page load proportional to its size; it would also let two
// members' counts be read at different instants, so a message answered mid-read
// could be absent from both columns.
func (s *Service) TeamBoard(ctx context.Context) (crmcontracts.TeamBoard, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.TeamBoard{}, apperrors.ErrPermissionDenied
	}
	switch actor.Permissions.RowScope {
	case principal.RowScopeTeam, principal.RowScopeAll:
	default:
		return crmcontracts.TeamBoard{}, apperrors.ErrPermissionDenied
	}
	if s.teammates == nil {
		// Nil is a REFUSAL here, matching resolveOwner: a board assembled
		// without the membership reader would have no roster to draw, and
		// answering it as an empty team would report that nobody works here.
		return crmcontracts.TeamBoard{}, apperrors.ErrPermissionDenied
	}
	roster, rosterCut, err := s.teammates.LiveTeammatesOfCaller(ctx)
	if err != nil {
		return crmcontracts.TeamBoard{}, err
	}
	asOf := s.now()
	load, err := s.teamLoad(ctx, asOf)
	if err != nil {
		return crmcontracts.TeamBoard{}, err
	}
	board := crmcontracts.TeamBoard{
		AsOf:       asOf,
		Members:    make([]crmcontracts.TeamBoardMember, 0, len(roster)),
		Unassigned: load.counts[ids.UUID{}],
		// Either a count read to its bound OR a roster cut at its own: both mean
		// the board is showing less than there is, and a reader who must not
		// trust the figures as totals needs one flag rather than the difference
		// between two ways of falling short.
		Truncated: load.truncated || rosterCut,
	}
	for _, member := range roster {
		board.Members = append(board.Members, crmcontracts.TeamBoardMember{
			UserId:      openapi_types.UUID(member.UserID),
			DisplayName: member.DisplayName,
			Counts:      load.counts[member.UserID],
		})
	}
	// By name, because a manager scans the board for a person they already have
	// in mind. Ordering by load would move a row every time the numbers moved,
	// so the same person sits somewhere new on each read.
	sort.Slice(board.Members, func(a, b int) bool {
		if board.Members[a].DisplayName != board.Members[b].DisplayName {
			return board.Members[a].DisplayName < board.Members[b].DisplayName
		}
		return board.Members[a].UserId.String() < board.Members[b].UserId.String()
	})
	return board, nil
}

// teamCounts holds the three figures per owner, plus whether any of them is a
// floor rather than a total.
type teamCounts struct {
	counts    map[ids.UUID]crmcontracts.TeamBoardCounts
	truncated bool
}

// teamLoad reads the three sources and buckets them by owner.
//
// A source this reader may not read is an ERROR rather than a column of zeros.
// The queue can name a withheld source on the wire because it returns rows and
// has somewhere to say it; a board is nothing but numbers, and a zero that means
// "you may not see this" is indistinguishable from one that means "they are
// clear" — which is the reading that would tell a lead their team is fine.
func (s *Service) teamLoad(ctx context.Context, asOf time.Time) (teamCounts, error) {
	load := teamCounts{counts: map[ids.UUID]crmcontracts.TeamBoardCounts{}}
	if s.waiting != nil {
		// The SAME read the ranked queue takes, bucketed by owner. A count from
		// a second query would drift from the page it summarises the first time
		// either changed — a manager reads eleven, the rep opens their day and
		// sees nine, and nothing says which is right.
		waiting, cut, err := s.waiting.Unanswered(ctx, asOf)
		if err != nil {
			return teamCounts{}, err
		}
		for _, customer := range waiting {
			row := load.counts[customer.OwnerID]
			row.Waiting++
			load.counts[customer.OwnerID] = row
		}
		// The lane's OWN answer, as the at-risk source's is, and for the same
		// reason: this lane filters after it scans. The seam drops machine
		// senders and folds duplicate threads out of what SQL returned, so a
		// hundred and eighty rows can be the survivors of a full two hundred —
		// and a hundred and eighty is what a smaller, complete installation
		// returns too. Comparing the count against the bound therefore read a
		// truncated scan as a total.
		load.truncated = load.truncated || cut
	}
	if s.overdueLoad != nil {
		overdue, err := s.overdueLoad.OverduePerAssignee(ctx, asOf)
		if err != nil {
			return teamCounts{}, err
		}
		for owner, count := range overdue {
			row := load.counts[owner]
			row.Overdue = count
			load.counts[owner] = row
		}
	}
	if s.atRisk != nil {
		risky, cut, err := s.atRisk.Quiet(ctx)
		if err != nil {
			return teamCounts{}, err
		}
		for _, deal := range risky {
			owner := ids.UUID{}
			if deal.OwnerID != nil {
				owner = *deal.OwnerID
			}
			row := load.counts[owner]
			row.AtRisk++
			load.counts[owner] = row
		}
		// The lane's OWN answer, not a count of its rows.
		//
		// This source filters after it scans — two bounded sweeps of the deal
		// list, then a union keeping only what is quiet or overdue — so a lane
		// returning ten may have read fifty and stopped with more behind it.
		// Comparing len(risky) against the scan bound therefore fails in the one
		// direction that must not fail: a truncated scan whose survivors are few
		// looks exactly like a complete one, and the board would call a floor a
		// total. The bound belongs to the reader that applied it.
		load.truncated = load.truncated || cut
	}
	return load, nil
}
