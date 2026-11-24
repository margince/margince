// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// KindLegalReviewRequest tells one seat that may answer legal reviews that a
// deal waits for one. It carries no decision: the review is answered on its own
// screen, and the card leaves the inbox when the review is answered or withdrawn.
const KindLegalReviewRequest = "legal_review_request"

// ObjectLegalReview is the RBAC object a legal review is read and answered under.
const ObjectLegalReview = "legal_review"

// LegalReviewDecisionActions are the actions on ObjectLegalReview that answering a
// review takes. The answer, the reviewer list and the inbox entry all derive from
// this one declaration. Update alone would let a seat answer a review it cannot open.
func LegalReviewDecisionActions() []principal.Action {
	return []principal.Action{principal.ActionRead, principal.ActionUpdate}
}

// InAppOnly reports whether a kind's notice stays in the app whatever the
// reader's approval delivery preference says. The legal review card is one:
// reviewers are told on screen and in the inbox, and never by email.
func InAppOnly(kind string) bool { return kind == KindLegalReviewRequest }

// LegalReviewRequest is that card. OwnerID is the reviewer it is for, and it
// names the deal and the review but holds none of what the requester wrote.
type LegalReviewRequest struct {
	ReviewID ids.UUID `json:"review_id"`
	DealID   ids.UUID `json:"deal_id"`
	DealName string   `json:"deal_name"`
	OwnerID  ids.UUID `json:"owner_id"`
}
