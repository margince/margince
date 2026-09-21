// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The words a queue row carries for which lane produced it and which subject it
// names. Named rather than written as literals, because the readers are spread —
// the classifier sets them, the scope filters, bounds table and category map ask
// about them — and a misspelt literal matches nothing, drops nothing, and reads
// green.

const (
	sourceWaiting = "customer_waiting"
	sourceTask    = "task"
	// A claim has no assignee column, so the row arrives ownerless while already
	// belonging to the rep whose query produced it. keepUnowned reads it.
	sourceClaim  = "conversation_claim"
	sourceAtRisk = "deal_at_risk"
	// The brief already ranks against the reader's own responsibility, so a row
	// here is bound to the contact asking. Judging it again by DEAL OWNER would
	// drop the assist case: the night picks a colleague's deal because this rep
	// has work on it, and the owner filter removes it before they see it.
	sourceBriefItem = "brief_item"
	sourceDuplicate = "dedupe_candidate"
	// Contact-linked disclosure duties, across ranking and scope.
	sourceNoticeCase = "notice_case"
)

const (
	subjectCompany = "company"
	subjectDeal    = "deal"
	subjectContact = "contact"
)
