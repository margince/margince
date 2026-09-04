// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// What a task is CALLED, held apart from what it is KEYED by.
//
// Its own file because the rule has more reasoning than mechanism: the name
// exists for one surface, the reason it is required is that surface's fallback,
// and the shapes refused are refused because they are the key again.

import (
	"fmt"
	"regexp"
	"strings"
)

// checkDisplayName holds what a task is CALLED against being another spelling
// of what it is KEYED by.
//
// Required, because the surface that needs one has no fallback worth having:
// the AI-work incident row falls back to a generic phrase, and the obvious
// substitute — the task key — is the leak the name exists to close. A task
// added without a name would silently rejoin that fallback.
//
// Refused when it looks like the key: snake_case, or the key itself with the
// underscores swapped for spaces. Both are the vocabulary wearing a hat, and
// `TestAMintedLabelIsWordsAndNotVocabulary` refuses the first downstream — this
// refuses it at the source, where the author can still pick words.
func checkDisplayName(name, display string) error {
	if strings.TrimSpace(display) == "" {
		return fmt.Errorf("task %q: display_name is required — it is what a reader is shown when this task fails", name)
	}
	if display != strings.TrimSpace(display) {
		return fmt.Errorf("task %q: display_name %q has surrounding whitespace", name, display)
	}
	if snakeCaseRE.MatchString(display) || strings.EqualFold(display, strings.ReplaceAll(name, "_", " ")) {
		return fmt.Errorf("task %q: display_name %q is the task key in words — say what the task DOES, in the language a rep reads", name, display)
	}
	return nil
}

// snakeCaseRE recognises a display name that is simply the key pasted in.
var snakeCaseRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
