// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheRunHistoryNamesWhyARunStoppedInACode(t *testing.T) {
	t.Parallel()
	reason := sanitizedReason(apperrors.ErrNotFound)
	targetMissing, blocked := RunReasonTargetMissing, RunReasonBlocked
	for _, tc := range []struct {
		status string
		want   *string
	}{
		{"failed", &targetMissing},
		{"blocked", &blocked},
		{"skipped", nil},
	} {
		run := wireAutomationRun(crmcontracts.Id(ids.NewV7()), AutomationRunRecord{ID: ids.NewV7(), Status: tc.status, Detail: &reason})
		switch {
		case tc.want == nil && run.ReasonCode != nil:
			t.Errorf("a %s run carries reason_code %q, want none", tc.status, *run.ReasonCode)
		case tc.want != nil && (run.ReasonCode == nil || *run.ReasonCode != *tc.want):
			t.Errorf("a %s run carries reason_code %v, want %q", tc.status, run.ReasonCode, *tc.want)
		}
	}
}
