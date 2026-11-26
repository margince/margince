// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package momentaction

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAClaimThatBecameATaskIsAskedOnlyThroughTheTask(t *testing.T) {
	claim, source, contact, task := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	if _, ok := ClaimDone(claim, source, contact, &task); ok {
		t.Fatal("a claim that became a task raised its own question")
	}
	promise, ok := ClaimDone(claim, source, contact, nil)
	if !ok || promise.ID != claim || promise.Source != source || promise.Contact != contact {
		t.Fatalf("an open claim of ours is not asked about as itself: %+v", promise)
	}
}
