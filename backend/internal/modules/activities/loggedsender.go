// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

const fieldSenderContactID = "sender_contact_id"

// SenderNotLinkedError refuses a named sender who is not among the activity's
// linked contacts. The sender must be on the record the message is filed under.
type SenderNotLinkedError struct{}

func (e *SenderNotLinkedError) Error() string {
	return "sender_contact_id must name one of the contacts in links; add that contact's link or name a linked contact"
}

// FieldFault makes the refusal a 422 against sender_contact_id on both the
// REST and the MCP surface.
func (e *SenderNotLinkedError) FieldFault() (field, code, message string) {
	return fieldSenderContactID, "sender_not_linked", e.Error()
}

// checkLoggedSender admits a named sender before anything is written. Only a
// received interaction has an outside author, and the author must be linked.
func checkLoggedSender(in LogActivityInput) error {
	if in.SenderContactID == nil {
		return nil
	}
	if !relstrength.IsParticipantKind(in.Kind) || in.Direction == nil || *in.Direction != directionInbound {
		return &KindFieldError{Field: fieldSenderContactID, Only: "an inbound message, call, meeting or email"}
	}
	if !slices.Contains(linkedContactIDs(in.Links), *in.SenderContactID) {
		return &SenderNotLinkedError{}
	}
	return nil
}
