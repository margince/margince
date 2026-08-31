// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import "strings"

// SplitGreetingLine puts the opening greeting on its own line.
//
// The prompt asks for it and a model obeys inconsistently: the same request
// returns "Greven, kurz und schmerzlos. Was ist der Stand?" as often as the
// two-line form, and the composer renders exactly the breaks it is given. So
// the rep opens a wall of text for a reason no rule in the prompt can be
// tightened enough to prevent — the same request, twice, gives both answers.
//
// This is a formatting repair and never a rewrite. It moves ONE break and
// changes not a character of what the model wrote, which is what makes it safe
// to run over generated prose: the words that go out are still the model's.
//
// It fires only where the split is unambiguous — the body opens with the
// recipient's own name, followed by a comma or a full stop, and the message
// continues on the same line. A greeting already on its own line is left
// alone, and so is a body that opens with anything else, because a break
// inserted at the wrong place is worse than the run-on it was meant to fix.
func SplitGreetingLine(body string, names ...string) string {
	for _, candidate := range names {
		if split, ok := splitOn(body, candidate); ok {
			return split
		}
	}
	return body
}

// splitOn attempts the repair for ONE candidate name, reporting whether it
// applied. Several are tried because the greeting register decides which name
// opens the message: the familiar form takes the first name, the formal form
// the surname, and the repair has to recognise whichever the model wrote.
func splitOn(body, recipient string) (string, bool) {
	name := strings.TrimSpace(recipient)
	if name == "" {
		return body, false
	}
	trimmed := strings.TrimLeft(body, " \t")
	// Case-insensitively, because a model that lowercases the opening
	// ("greven, kurz und schmerzlos") wrote the same greeting and the rep sees
	// the same run-on line. The body keeps ITS spelling — only the match is
	// relaxed, so nothing the model wrote is rewritten.
	if !strings.HasPrefix(strings.ToLower(trimmed), strings.ToLower(name)) {
		return body, false
	}
	// Sliced off the BODY by the name's byte length, so the greeting keeps the
	// capitalisation the model chose. ToLower can change a string's length in
	// general, so the two are compared above and never mixed here.
	if len(trimmed) < len(name) {
		return body, false
	}
	rest := trimmed[len(name):]
	greeting := trimmed[:len(name)]
	if rest == "" {
		return body, false
	}
	// The separator the greeting ends on. A comma is the ordinary one; a full
	// stop is what the terser registers produce ("Greven. Wir haben…").
	separator := rest[0]
	if separator != ',' && separator != '.' {
		return body, false
	}
	after := rest[1:]
	// Already broken: the greeting owns its line and there is nothing to move.
	// CRLF counts — a body that arrives with Windows line endings is already
	// formatted, and inserting a break in front of them stacks blank lines.
	if strings.HasPrefix(after, "\n") || strings.HasPrefix(after, "\r\n") {
		return body, false
	}
	message := strings.TrimLeft(after, " \t")
	if message == "" {
		return body, false
	}
	return greeting + string(separator) + "\n\n" + message, true
}
