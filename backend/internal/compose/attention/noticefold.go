// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Decisions own their announcements even beyond the loaded inbox page. The
// approval engine checks the same responsibility and authority as its list.
func (s *Service) noticesWithoutDecisions(ctx context.Context, notices []UnreadNotice) ([]UnreadNotice, error) {
	named := make([]ids.ApprovalID, 0, len(notices))
	for _, notice := range notices {
		if notice.ApprovalID != nil {
			named = append(named, *notice.ApprovalID)
		}
	}
	if len(named) == 0 {
		return notices, nil
	}
	visible, err := s.approvals.PendingInScope(ctx, s.approvalQuery(0), named)
	if err != nil {
		return nil, err
	}
	kept := make([]UnreadNotice, 0, len(notices))
	for _, notice := range notices {
		if notice.ApprovalID != nil && visible[*notice.ApprovalID] {
			continue
		}
		kept = append(kept, notice)
	}
	return kept, nil
}
