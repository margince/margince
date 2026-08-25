// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The identity every write already carries and nothing published.
var whoamiCopy = toolCopy{
	Purpose: "Name the human this passport acts for: their id, display name, email and language.",
	Limits:  "It reads only, and answers this call's acting user — not a directory.",
	Retain: "acting_user_id is what owner_id and assignee_id take for \"me\". prose_language is " +
		"the language every stored sentence is written in — a note, a description, a summary — " +
		"whatever language the conversation itself is in; it is always answered, where locale is " +
		"absent until this person chooses one.",
}
