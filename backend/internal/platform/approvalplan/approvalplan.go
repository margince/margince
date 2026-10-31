// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package approvalplan holds the one rule for when an approval shows nothing
// of its plan. The approvals wire, read_approval, the approval email and both
// cards read it, so no surface shows what another holds back.
package approvalplan

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Withheld reports a preview that shows nothing of the plan: one in a version
// no reader knows, or in any state but ready.
func Withheld(preview *crmcontracts.ApprovalPreview) bool {
	return preview != nil && ((preview.Version != 1 && preview.Version != 2) || preview.State != stateReady)
}

const stateReady = "ready"

// NeedsWiderSight is the state of a preview whose effect touches a record the
// reader cannot open.
const NeedsWiderSight = "needs_wider_sight"

// MaskedKind is the value kind a preview shows for a value this reader's role
// masks, or one it may not name.
const MaskedKind = "unavailable_name"

// PreviewRead is one preview as the approvals service read it.
type PreviewRead struct {
	Preview *crmcontracts.ApprovalPreview
	// BuiltAsAgent says the preview was read as the agent that runs the effect.
	BuiltAsAgent bool
	// HeldBack is HeldBack read off the preview before any clearing.
	HeldBack bool
}

// PayloadWithheld reports an approval whose staged call, summary and evidence
// must stay behind. Only a stale or incomplete proposal its decider runs keeps
// them: its decider's own read stands behind them. An agent wrote its staged
// call with its own sight, and an unavailable preview may be the reader's lack
// of access.
func (read PreviewRead) PayloadWithheld() bool {
	preview := read.Preview
	if preview == nil {
		return false
	}
	if read.HeldBack || (preview.Version != 1 && preview.Version != 2) {
		return true
	}
	switch preview.State {
	case stateReady:
		return false
	case "stale", "incomplete":
		return read.BuiltAsAgent
	default:
		return true
	}
}

// HeldBack reports a preview that shows a value as masked or unnamed, which
// the staged call would repeat. Read it before a preview is cleared.
func HeldBack(preview *crmcontracts.ApprovalPreview) bool {
	if preview == nil {
		return false
	}
	values := []crmcontracts.ApprovalPreviewValue{}
	collect := func(changes []crmcontracts.ApprovalPreviewChange, details []crmcontracts.ApprovalPreviewDetail) {
		for _, change := range changes {
			values = append(values, change.Before, change.After)
		}
		for _, detail := range details {
			values = append(values, detail.Value)
		}
	}
	if preview.Details != nil {
		collect(preview.Changes, *preview.Details)
	} else {
		collect(preview.Changes, nil)
	}
	if preview.ImportRows != nil {
		for _, row := range *preview.ImportRows {
			collect(row.Changes, row.Details)
		}
	}
	return slices.ContainsFunc(values, func(value crmcontracts.ApprovalPreviewValue) bool { return value.Kind == MaskedKind })
}
