// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

import (
	"strings"
	"testing"
)

// The mappable-seat predicate carries all three halves, on every alias.
//
// This exists because naming the predicate once made it INVISIBLE to the gate
// that used to watch it. TestOnlyOneSpellingOfALiveMember detects literal
// `status = 'active'` / `archived_at IS NULL` halves in statements that read
// app_user; overlay's three sites carried those literals, so the gate saw them
// (as ratified copies) and would have seen one of them lose a half. Now they
// call this helper, the literals are not in the statement text, and the gate
// treats the call as opaque — so dropping a half here would be silent.
//
// Which is the trade naming it once makes, and this is the other side of it.
func TestTheMappableSeatPredicateNamesEveryHalf(t *testing.T) {
	t.Parallel()
	for _, alias := range []string{"u", ""} {
		got := mappableSeatSQL(alias)
		prefix := ""
		if alias != "" {
			prefix = alias + "."
		}
		// Each half separately, with the reason it is there, because a failure
		// that says "the predicate changed" sends a reader to diff two strings
		// while a failure that says WHICH half went sends them to the seat it
		// would have started offering.
		for _, half := range []struct{ sql, opens string }{
			{
				"NOT " + prefix + "is_agent",
				"an agent seat is a passport identity with no incumbent counterpart to map",
			},
			{
				prefix + "status = 'active'",
				"a DEACTIVATED seat can no longer log in, and offering it a mapping grants mirror visibility to an account nobody is using (#2592)",
			},
			{
				prefix + "archived_at IS NULL",
				"an ARCHIVED seat can no longer log in either, and this half is the one that was always here",
			},
		} {
			if !strings.Contains(got, half.sql) {
				t.Errorf("mappableSeatSQL(%q) does not carry %q, so it would offer a mapping to a seat where %s\n\ngot: %s",
					alias, half.sql, half.opens, got)
			}
		}
	}
}

// And every statement in this package that decides mappability asks the helper
// rather than spelling a predicate beside it.
//
// The three call sites are the whole invariant, and they used to be three
// literal spellings held together by comments naming each other — which is what
// let them all diverge from their own stated reason in the same direction.
func TestEveryMappabilityStatementAsksTheHelper(t *testing.T) {
	t.Parallel()
	// Rendered from the constants themselves, so a statement that stopped
	// calling the helper shows up as a missing fragment rather than as prose
	// somebody has to re-read.
	for name, sql := range map[string]string{
		"listUserMapSQL":         listUserMapSQL,
		"selectUserMapTargetSQL": selectUserMapTargetSQL,
	} {
		if !strings.Contains(sql, mappableSeatSQL("u")) {
			t.Errorf("%s does not carry mappableSeatSQL(\"u\"). The three mappability sites are "+
				"one invariant; a predicate spelled beside the helper is how they diverged "+
				"before (#2592).\n\ngot: %s", name, sql)
		}
	}
}
