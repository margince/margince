// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
)

// spellClearedBody reads a body sent as null as the empty text, so every rule
// on a text write (content, invitation, language, audit) sees the clear. The
// UPDATE stores the empty text as NULL, the body of an entry nobody wrote.
func (in *UpdateActivityInput) spellClearedBody() {
	if in.Body == nil && in.clears(fieldBody) {
		in.Body = new("")
	}
}

// patchedOwnEnd is the stored end of the body a patch writes. An empty body is
// stored as none, and so is its end.
func patchedOwnEnd(current crmcontracts.Activity, body *string) *int {
	if body == nil || *body == "" {
		return nil
	}
	return mailquote.OwnEnd(string(current.Kind), body)
}
