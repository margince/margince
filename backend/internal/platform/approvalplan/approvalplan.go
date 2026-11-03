// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package approvalplan holds the one rule for when an approval shows nothing
// of its plan. The approvals wire, read_approval, the approval email and both
// cards read it, so no surface shows what another holds back.
package approvalplan

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// Withheld reports a preview that shows nothing of the plan: one in a version
// no reader knows, or in any state but ready.
func Withheld(preview *crmcontracts.ApprovalPreview) bool {
	return preview != nil && ((preview.Version != 1 && preview.Version != 2) || preview.State != stateReady)
}

const stateReady = "ready"

// NeedsWiderSight is the state of a preview whose effect touches a record the
// reader cannot open.
const NeedsWiderSight = "needs_wider_sight"

// PreviewRead is one preview as the approvals service read it.
type PreviewRead struct {
	Preview *crmcontracts.ApprovalPreview
	// BuiltAsAgent says the preview was read as the agent that runs the effect.
	BuiltAsAgent bool
	// HeldBack says the build replaced a value the reader's role masks.
	HeldBack bool
}

// PayloadWithheld reports an approval whose staged call, summary and evidence
// must stay behind. A stale, incomplete or unavailable proposal its decider
// runs keeps them. Its reader already writes the target and reads every cited
// activity, or the approval would not be listed for them. An agent wrote its
// staged call with its own sight, so its proposal keeps nothing unless ready.
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
	case "stale", "incomplete", "unavailable":
		return read.BuiltAsAgent
	default:
		return true
	}
}
