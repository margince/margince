// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The reversal's own row names the entry it undoes; a row about another record
// in the same write does not, because it reverses nothing.
func TestAReversalNamesTheEntryOnlyOnItsOwnRecordsRow(t *testing.T) {
	lead, contact, promotion := ids.NewV7(), ids.NewV7(), ids.NewV7()
	ctx := WithReversal(context.Background(), "lead", lead, promotion)

	own := withReversalLink(ctx, "lead", lead, map[string]any{"reason": "undo"})
	if own[EvidenceKeyUndidAuditLog] != promotion.String() || own["reason"] != "undo" {
		t.Errorf("the lead's row carries %v, want the promotion named beside its own evidence", own)
	}
	other := withReversalLink(ctx, "contact", contact, nil)
	if _, named := other[EvidenceKeyUndidAuditLog]; named {
		t.Errorf("the contact's row names the lead's promotion: %v", other)
	}
	plain := withReversalLink(context.Background(), "lead", lead, nil)
	if plain != nil {
		t.Errorf("a write outside any reversal gained evidence: %v", plain)
	}
}

// A caller that already names the entry it reverses keeps its own answer.
func TestAReversalNeverOverwritesTheEntryACallerNamed(t *testing.T) {
	lead, mine := ids.NewV7(), ids.NewV7()
	ctx := WithReversal(context.Background(), "lead", lead, ids.NewV7())
	got := withReversalLink(ctx, "lead", lead, map[string]any{EvidenceKeyUndidAuditLog: mine.String()})
	if got[EvidenceKeyUndidAuditLog] != mine.String() {
		t.Errorf("the caller's link was replaced: %v", got)
	}
}
