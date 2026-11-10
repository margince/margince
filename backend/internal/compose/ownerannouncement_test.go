// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOwnerAnnouncementsRefuseIncompleteScopeBeforeReadingRecords(t *testing.T) {
	workspace, approval, decider := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, tc := range []struct {
		name string
		args ApprovalOwnerNoticeArgs
	}{
		{"workspace", ApprovalOwnerNoticeArgs{ApprovalID: approval, DeciderID: decider}},
		{"request", ApprovalOwnerNoticeArgs{Workspace: workspace, DeciderID: decider}},
		{"recipient", ApprovalOwnerNoticeArgs{Workspace: workspace, ApprovalID: approval}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			worker := approvalOwnerNoticeWorker{}
			err := worker.Work(t.Context(), &river.Job[ApprovalOwnerNoticeArgs]{Args: tc.args})
			if _, cancelled := errors.AsType[*river.JobCancelError](err); !cancelled {
				t.Fatalf("invalid announcement was not cancelled: %v", err)
			}
			if tc.name != "workspace" && !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Fatalf("missing recipient/request did not fail validation: %v", err)
			}
		})
	}
}
