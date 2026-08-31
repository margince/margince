// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The audience rule for a reader that COUNTS messages rather than showing them.
//
// A held message is not only unreadable — it must not be visible in the shape
// of an aggregate either. A relationship-strength score that counts a founder's
// correspondence with their lawyer tells every colleague the relationship is
// strong; a last-activity timestamp that moves when a private message arrives
// tells them when it arrived. Neither shows a word of the message, and both
// disclose it.
//
// Spelled once here rather than per module because four readers ask the same
// question — relationship strength, deal health, the meeting brief and the
// weekly digest — and a fifth will be written by somebody who greps for it. The
// graph projection asked it first and carried its own copy; that copy moved
// here rather than being joined by a second.

// AudienceWorkspaceOnly filters an activity aliased by name down to the
// messages the whole workspace may see.
//
// It takes no placeholder, so it renumbers nothing at a call site: a caller
// concatenates it into a WHERE clause and the surrounding arguments keep their
// positions.
//
// Deliberately NOT the reader's own audience. An aggregate is read by
// colleagues who did not receive the mail, and a per-caller answer would give
// two people different numbers for the same account — which reads as a bug and
// discloses the difference between them. The question an aggregate asks is
// "may EVERYONE see this", and the answer is the same for everybody.
func AudienceWorkspaceOnly(alias string) string {
	return " AND " + alias + ".audience = 'workspace'"
}
