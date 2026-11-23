// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import "github.com/margince/margince/backend/internal/platform/approvalsubject"

// legalReviewGrants is the legal review card's decision grants, derived from the
// declaration the review's own answer uses.
func legalReviewGrants() []DecisionGrant {
	var out []DecisionGrant
	for _, action := range approvalsubject.LegalReviewDecisionActions() {
		out = append(out, DecisionGrant{approvalsubject.ObjectLegalReview, action})
	}
	return out
}
