// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength_test

// The SQL renderers, against the Go predicate that shares their sets.
//
// The relationship BETWEEN the two sets, and their coverage of the contract's
// kind vocabulary, is held in backend/gates/activitykindsets_test.go — this
// package cannot reach the contract, and a list of kinds typed out here would
// be a census that stops growing the day somebody adds a seventh.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// A renderer exists so a query filters on the same set the Go path does. That
// only holds if the rendering and the predicate agree, which is what this asks:
// every kind the list names is one IsParticipantKind admits, and nothing else
// is.
func TestTheParticipantListAndThePredicateAreOneSet(t *testing.T) {
	t.Parallel()
	rendered := sqlListValues(t, relstrength.ParticipantKindSQLList())
	if len(rendered) == 0 {
		t.Fatal("the participant list renders nothing; this test would judge nothing")
	}
	for _, kind := range rendered {
		if !relstrength.IsParticipantKind(kind) {
			t.Errorf("the list names %q and the predicate refuses it", kind)
		}
	}
	// And the other direction, over values the predicate might admit that the
	// list forgot. Spelled here rather than derived because this is the SMALL
	// half of the check — the gate holds the vocabulary — and its whole job is
	// to catch a set edited on one side.
	for _, kind := range []string{"email", "call", "meeting", "message", "note", "task"} {
		named := false
		for _, got := range rendered {
			if got == kind {
				named = true
			}
		}
		if relstrength.IsParticipantKind(kind) != named {
			t.Errorf("the predicate and the list disagree about %q", kind)
		}
	}
}

// The scoring set has no Go predicate — every reader of it is a query — so the
// two renderings are all there is, and the parenthesised one must be the list
// and nothing else. Asserted as SHAPE rather than by recomputing the expression
// under test, which would only prove the function equals itself.
func TestTheScoringGroupIsTheListInBrackets(t *testing.T) {
	t.Parallel()
	group := relstrength.InteractionKindSQLGroup()
	if !strings.HasPrefix(group, "(") || !strings.HasSuffix(group, ")") {
		t.Fatalf("the group renders as %q, which a `kind IN %%s` caller cannot use", group)
	}
	if inner := group[1 : len(group)-1]; inner != relstrength.InteractionKindSQLList() {
		t.Errorf("the group wraps %q and the list is %q", inner, relstrength.InteractionKindSQLList())
	}
	if len(sqlListValues(t, relstrength.InteractionKindSQLList())) == 0 {
		t.Fatal("the scoring list renders nothing; the queries reading it would match no row")
	}
}

// sqlListValues reads a rendered `'a','b'` list back, failing rather than
// guessing when it is not one: a renderer that stopped quoting would otherwise
// yield one long value and every comparison above would quietly find nothing.
func sqlListValues(t *testing.T, list string) []string {
	t.Helper()
	if !strings.HasPrefix(list, "'") || !strings.HasSuffix(list, "'") {
		t.Fatalf("%q is not a quoted SQL list", list)
	}
	return strings.Split(strings.Trim(list, "'"), "','")
}
