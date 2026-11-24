// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"
)

// Work its user confirmed in the conversation names the code on every audit row
// it writes. The writer's own evidence stays intact.
func TestAnAuditRowNamesTheCodeThatConfirmedItsWork(t *testing.T) {
	own := map[string]any{"reason": "moved"}
	linked := withConfirmedByLink(WithConfirmingCode(context.Background(), "code-1"), own)
	if linked[EvidenceKeyConfirmedInConversation] != "code-1" || linked["reason"] != "moved" {
		t.Fatalf("evidence %v, want the code's id beside the writer's own evidence", linked)
	}
	if _, touched := own[EvidenceKeyConfirmedInConversation]; touched {
		t.Error("the writer's own evidence map must not be modified")
	}
	if got := withConfirmedByLink(context.Background(), own); len(got) != 1 {
		t.Errorf("work nobody confirmed in a conversation carries %v, want the writer's evidence alone", got)
	}
}
