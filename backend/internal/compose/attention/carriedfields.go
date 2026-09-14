// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Carrying a lane item's fields across to the queue row.
//
// The attention feed and the worklist queue declare the same values as their
// OWN types in the contract, so every field crosses by an explicit conversion
// rather than by sharing a schema. That is a deliberate cost: the drift gate is
// what holds the two enums to the same words, and a field that stops being
// forwarded here reads on screen as a feature that quietly does nothing.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// carriedDueGroup moves the lane's grouping onto the queue row. The two schemas
// declare the same five words as their own enums, so the crossing is a cast —
// but the contract drift gate is what keeps them the same five, and a value the
// queue's enum does not know would be a heading no client has copy for.
func carriedDueGroup(group *crmcontracts.AttentionItemDueGroup) *crmcontracts.WorklistItemDueGroup {
	if group == nil {
		return nil
	}
	carried := crmcontracts.WorklistItemDueGroup(*group)
	return &carried
}

// carriedUndo moves the way-back onto the queue row. The two schemas declare
// the same shape as their own types, so the crossing is a copy — held to one
// shape by the contract drift gate rather than by this function.
func carriedUndo(undo *crmcontracts.AppliedUndo) *crmcontracts.AppliedUndo {
	if undo == nil {
		return nil
	}
	carried := *undo
	return &carried
}

// carriedActions passes the lane feed's verbs through unchanged. The queue adds
// no authority of its own: every verb still routes to the endpoint that owns it.
func carriedActions(actions []crmcontracts.AttentionItemActions) []crmcontracts.WorklistItemActions {
	out := make([]crmcontracts.WorklistItemActions, 0, len(actions))
	for _, action := range actions {
		out = append(out, crmcontracts.WorklistItemActions(action))
	}
	return out
}
