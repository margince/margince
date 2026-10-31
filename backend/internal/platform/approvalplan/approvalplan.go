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
	return preview != nil && ((preview.Version != 1 && preview.Version != 2) || preview.State != "ready")
}

// NeedsWiderSight is the state of a preview whose effect touches a record the
// reader cannot open.
const NeedsWiderSight = "needs_wider_sight"

// MaskedKind is the value kind a preview shows for a value this reader's role
// masks, or one it may not name.
const MaskedKind = "unavailable_name"

// PayloadWithheld reports an approval whose staged call, summary and evidence
// must stay behind. Its preview needs wider sight, has a version no reader
// knows, or held back a value the staged call repeats. An agent's proposal
// also keeps them back whenever its preview is not ready. The agent wrote them
// with its own sight, which no check has matched to the reader's.
func PayloadWithheld(preview *crmcontracts.ApprovalPreview, builtAsAgent bool) bool {
	if preview == nil {
		return false
	}
	if (builtAsAgent && Withheld(preview)) || preview.State == NeedsWiderSight || (preview.Version != 1 && preview.Version != 2) {
		return true
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
	for _, value := range values {
		if value.Kind == MaskedKind {
			return true
		}
	}
	return false
}
