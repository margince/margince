// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"
)

func TestAStatedReasonRidesEveryRowItsWriteLands(t *testing.T) {
	own := map[string]any{"source": "site_read"}
	stated := withStatedReason(WithStatedReason(context.Background(), "Anna Weber asked for March."), own)
	if stated[EvidenceKeyStatedReason] != "Anna Weber asked for March." || stated["source"] != "site_read" {
		t.Fatalf("the row's evidence reads %v", stated)
	}
	if len(own) != 1 {
		t.Fatal("stamping the reason changed the writer's own evidence map")
	}
	if got := withStatedReason(context.Background(), own); len(got) != 1 {
		t.Fatalf("a write with no stated reason gained evidence: %v", got)
	}
}
