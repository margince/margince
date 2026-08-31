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
func SplitGreetingLine(body, recipient string) string {
	name := strings.TrimSpace(recipient)
	if name == "" {
		return body
	}
	trimmed := strings.TrimLeft(body, " \t")
	if !strings.HasPrefix(trimmed, name) {
		return body
	}
	rest := trimmed[len(name):]
	if rest == "" {
		return body
	}
	// The separator the greeting ends on. A comma is the ordinary one; a full
	// stop is what the terser registers produce ("Greven. Wir haben…").
	separator := rest[0]
	if separator != ',' && separator != '.' {
		return body
	}
	after := rest[1:]
	// Already broken: the greeting owns its line and there is nothing to move.
	if strings.HasPrefix(after, "\n") {
		return body
	}
	message := strings.TrimLeft(after, " \t")
	if message == "" {
		return body
	}
	return name + string(separator) + "\n\n" + message
}
