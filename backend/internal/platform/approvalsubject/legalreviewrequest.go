// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvalsubject

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// KindLegalReviewRequest tells one seat that may answer legal reviews that a
// deal waits for one. It carries no decision: the review is answered on its own
// screen, and the card leaves the inbox when the review is answered or withdrawn.
const KindLegalReviewRequest = "legal_review_request"

// LegalReviewRequest is that card. OwnerID is the reviewer it is for, and it
// names the deal and the review but holds none of what the requester wrote.
type LegalReviewRequest struct {
	ReviewID ids.UUID `json:"review_id"`
	DealID   ids.UUID `json:"deal_id"`
	DealName string   `json:"deal_name"`
	OwnerID  ids.UUID `json:"owner_id"`
}
