// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import "strings"

// firstName is what the greeting uses: everything before the first space, or
// the whole name when it has none. A one-word name is a name, not a mistake.
func firstName(full string) string {
	if cut, _, found := strings.Cut(strings.TrimSpace(full), " "); found && cut != "" {
		return cut
	}
	return strings.TrimSpace(full)
}

// lastName is what follows the first space, so "Dr. Anne Weiss" yields "Anne
// Weiss": the 360 contact carries only a full name. contactdraft.surname reads
// a Contact's own last_name instead. The prompt greets formally only when a
// surname was given, which covers the names this gets wrong.
func lastName(full string) string {
	if _, rest, found := strings.Cut(strings.TrimSpace(full), " "); found {
		return strings.TrimSpace(rest)
	}
	return ""
}
