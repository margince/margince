// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import "strings"

// firstName is what the greeting uses. Everything before the first space, or
// the whole name when it has none — a one-word name is a name, not a mistake.
func firstName(full string) string {
	if cut, _, found := strings.Cut(strings.TrimSpace(full), " "); found && cut != "" {
		return cut
	}
	return strings.TrimSpace(full)
}

// lastName is what follows the FIRST space in the display name, which is the
// surname for a two-word name and not always one otherwise: "Dr. Anne Weiss"
// yields "Anne Weiss", a given name inside the result. No rule over display
// names gets every name right, and this input carries no name columns to
// consult — the 360 contact it folds has a full name and nothing else.
//
// A single-word name yields empty, which sends the greeting to the familiar
// form rather than to a formal one addressed to a first name.
//
// contactdraft.surname answers the same question from a Contact record and
// prefers that record's own last_name, which is the better answer where it
// exists. The two are not shared because their INPUTS differ, not their
// answer: unifying them would mean passing a Contact into a fold that has none.
//
// The prompt rule is what protects the output where this is wrong: a formal
// greeting is used only where a surname was given, and the model is told never
// to complete or invent one.
func lastName(full string) string {
	if _, rest, found := strings.Cut(strings.TrimSpace(full), " "); found {
		return strings.TrimSpace(rest)
	}
	return ""
}
