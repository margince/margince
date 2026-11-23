// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Requested decisions follow live membership. Departed owners' deals stay in
// team recovery, while their personal decisions leave the team inbox.
func (s *Service) approvalQuery(depth int) ApprovalQuery {
	query := ApprovalQuery{Status: "pending", Limit: depth}
	switch s.taskScope {
	case TasksMine:
		query.RequestedForReader = true
	case TasksUnassigned:
		query.OnlyUnassigned = true
	case TasksOwnedBy:
		query.RequestedForOwners = []ids.UUID{s.taskOwner}
	default:
		if s.noticeOwners != nil {
			query.RequestedForOwners = append([]ids.UUID{ids.Nil}, s.noticeOwners...)
		}
	}
	return query
}

// forOwner isolates the named owner's scope from concurrent requests.
func (s *Service) forOwner(owner ids.UUID) *Service {
	narrowed := *s
	narrowed.taskScope = TasksOwnedBy
	narrowed.taskOwner = owner
	return &narrowed
}

// forReader copies the shared service so one request cannot change another's scope.
func (s *Service) forReader() *Service {
	narrowed := *s
	narrowed.taskScope = TasksMine
	if s.personalZone != nil {
		narrowed.zone = zoneOncePerRead(s.personalZone)
	}
	return &narrowed
}

// withReadersSuggestions copies the shared service so its suggestion lane reads
// only the acting reader's companies, as GET /attention answers for the reader.
func (s *Service) withReadersSuggestions() *Service {
	narrowed := *s
	narrowed.suggestionsOfReader = true
	return &narrowed
}

// forUnowned isolates the unassigned scope from concurrent requests.
func (s *Service) forUnowned() *Service {
	narrowed := *s
	narrowed.taskScope = TasksUnassigned
	return &narrowed
}

// pendingApprovals is how many approvals wait in query's scope: the page's own
// length when it holds every one, the exact count otherwise.
func (s *Service) pendingApprovals(ctx context.Context, query ApprovalQuery, listed int, complete bool) (int, error) {
	if complete {
		return listed, nil
	}
	return s.approvals.CountPending(ctx, query)
}
