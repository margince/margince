// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The staged decisions below the ranked rows. The day reads approvals to the
// census depth. The tail continues the same inbox read in its newest-first
// keyset, from the last approval the day kept.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
)

// ApprovalBacklog continues the staged decisions past the census depth.
// OPTIONAL: unbound, scrolling ends at the ranked rows for approvals.
type ApprovalBacklog interface {
	// ApprovalsAfter reads up to in.Limit approvals after a position, each with
	// the position after it. It uses the query and decidability of the lane's
	// own read. more says approvals lie past the batch.
	ApprovalsAfter(ctx context.Context, in ApprovalQuery, after string) (
		staged []Positioned[crmcontracts.Approval], more bool, err error)
}

// WithApprovalBacklog binds the approval continuation, an option for the
// reason WithWaiting is one.
func (s *Service) WithApprovalBacklog(b ApprovalBacklog) *Service {
	s.backlog.approvals = b
	return s
}

// approvalTailStart is the inbox position after the last approval the lane
// kept. interleave keeps a prefix of each producer, so that is staged[kept-1].
// Empty when the lane kept none, which starts the tail at the newest approval.
func approvalTailStart(lane []crmcontracts.AttentionItem, staged []crmcontracts.Approval) (string, error) {
	kept := shownFrom(lane, string(crmcontracts.WorklistItemSourceApproval))
	if kept == 0 {
		return "", nil
	}
	return approvals.PositionAfter(staged[kept-1])
}

// moreApprovals reads the next staged decisions under the lane's own query, so
// each page is decided by the same authority as the ranked rows. An approval
// has no place in the tail's time order, so it is undated and follows the
// dated sources.
func (s *Service) moreApprovals(ctx context.Context, _ tailPage, walk tailWindow, after string, limit int) (tailBatch, error) {
	read, more, err := s.backlog.approvals.ApprovalsAfter(ctx, s.approvalQuery(limit), after)
	if err != nil {
		return tailBatch{}, err
	}
	items := make([]crmcontracts.AttentionItem, len(read))
	for i, staged := range read {
		items[i] = approvalItem(staged.Row, s.machine)
	}
	// appendLane draws one row per approval, in order.
	rows := appendLane(nil, crmcontracts.WorklistItemSourceApproval, &items, walk.AsOf, classifyDecision)
	batch := tailBatch{more: more, rows: make([]tailRow, len(read))}
	for i, staged := range read {
		batch.rows[i] = tailRow{row: rows[i], after: staged.After}
	}
	return batch, nil
}
