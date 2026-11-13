// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func (s *InquiryService) existingTask(ctx context.Context, ex Executors, ev workflow.Event, receipt inquiryReceipt) (crmcontracts.Activity, bool, error) {
	if s.routing == nil {
		return crmcontracts.Activity{}, false, errors.New("automation: inquiry routing is not configured")
	}
	admit, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	return s.routing.ExistingTask(ctx, admit, receipt.Key)
}

func inquiryTaskHandled(task crmcontracts.Activity) bool {
	return task.ArchivedAt != nil || (task.IsDone != nil && *task.IsDone) || task.SourceActivityId != nil || (task.SourceSystem != nil && *task.SourceSystem == provenance.WebsiteInquirySource)
}
