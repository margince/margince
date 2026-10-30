// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func hasTypedProposalDisplay(kind string) bool {
	switch kind {
	case "capture_counterparty", "close_date_correction", "company_name_promotion", "contact_share", "deal_follow_up", "fx_rate_proposal", "held_draft", "linkedin_match", "ownership_transfer", "site_lead", "vcard_create", "volume_release":
		return true
	default:
		return false
	}
}

func unsupportedApprovalPreview(out *crmcontracts.ApprovalPreview) *crmcontracts.ApprovalPreview {
	out.State = previewUnavailableState
	out.Conflicts = &[]crmcontracts.ApprovalPreviewConflict{{Kind: "unsupported_approval_kind", BlocksApproval: true, Summary: ""}}
	return out
}
