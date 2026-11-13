// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
)

// A mail's move reads its thread through a gated set. The move column tests
// membership in that set and renders no gate itself.
func TestAMailThreadSetCarriesOneContentGate(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	readable, err := MailThreadReadableSQL(teamScopedCtx(), "candidates", arg)
	if err != nil {
		t.Fatalf("rendering the thread set: %v", err)
	}
	if got := auth.ContentGateCopies(readable); got != 1 {
		t.Fatalf("the thread set carries %d content gates, want 1", got)
	}
	move := MailMoveColumnSQL(teamScopedCtx(), arg, clocktest.Now(t), "readable_set")
	if got := auth.ContentGateCopies(move); got != 0 {
		t.Fatalf("the move column carries %d content gates of its own, want 0", got)
	}
}
