// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// spellClearedBody reads a body sent as null as the empty text, so every rule
// on a text write (content, invitation, language, audit) sees the clear. The
// UPDATE stores the empty text as NULL, the body of an entry nobody wrote.
func (in *UpdateActivityInput) spellClearedBody() {
	if in.Body == nil && in.clears(fieldBody) {
		in.Body = new("")
	}
}
