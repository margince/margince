// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// TargetRetainedEmailSignature is the target type of a notice about a departed
// member's signature. The frontend routes this value to the review screen.
const TargetRetainedEmailSignature = "retained_email_signature"

// Target is the record a notice is about, when it is about one.
//
// Both halves or neither, as the table's own constraint holds. A type with no
// id names a kind of thing and cannot be opened. An id with no type cannot be
// routed to a screen. The zero value is a notice about no record, such as a
// capture backlog or a coach's word, and most notices are.
type Target struct {
	Type string
	ID   ids.UUID
}

// Named reports whether this notice points at a record.
func (t Target) Named() bool { return t.Type != "" && !t.ID.IsZero() }

// Earlier retained-signature notices carry their references in the descriptor.
// Resolving the link keeps their stored text and delivery identity intact.
func resolvedNoticeTarget(stored Target, kind string, message *systemtext.Message) Target {
	if stored.Named() || kind != KindPrivacyReviewRequired || message == nil || message.Version != 1 || message.Body == nil {
		return stored
	}
	body := message.Body
	if body.Code != "privacy.review.inactive" || len(body.Args) != 2 || body.Args[0].Kind != "text" || body.Args[1].Kind != "text" {
		return stored
	}
	signature, err := ids.Parse(body.Args[0].Text)
	if err != nil || signature.IsZero() {
		return stored
	}
	owner, err := ids.Parse(body.Args[1].Text)
	if err != nil || owner.IsZero() {
		return stored
	}
	return Target{Type: TargetRetainedEmailSignature, ID: signature}
}
