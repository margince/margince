// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An approval notice opens the decision its key names; nothing else does.
func TestOnlyAnApprovalNoticeOpensADecision(t *testing.T) {
	approval := ids.NewV7()
	got, ok := approvalOfNoticeKey(KindApprovalPending, ApprovalNoticeKey(approval))
	if !ok || got != approval {
		t.Fatalf("the approval notice opens %v (ok=%v), want %v", got, ok, approval)
	}
	for _, tc := range []struct{ kind, key string }{
		{"automation", ApprovalNoticeKey(approval)},
		{KindApprovalPending, "approval_pending:not-an-id"},
		{KindApprovalPending, ""},
		{KindApprovalPending, "lead_sla:" + approval.String()},
	} {
		if _, ok := approvalOfNoticeKey(tc.kind, tc.key); ok {
			t.Errorf("kind %q with key %q opened a decision", tc.kind, tc.key)
		}
	}
}
