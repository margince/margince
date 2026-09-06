// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength_test

// The two kind sets and the ONE direction they may differ in.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// everyKind is the activity vocabulary this package has an opinion about, plus
// the two it must keep refusing. Spelled out rather than derived, because a
// derived list would grow silently and this test's whole job is to notice.
var everyKind = []string{"email", "call", "meeting", "message", "note", "task"}

// THE INVARIANT THE SPLIT RESTS ON: anything worth scoring is worth recording
// the people on. The reverse is allowed — a group chat has a room full of
// people and an unsettled claim to warmth — but a kind that scored a
// relationship while nobody was recorded as being on it would leave the
// interaction graph unable to say who the relationship is with.
func TestEveryScoredKindHasParticipants(t *testing.T) {
	t.Parallel()
	for _, kind := range everyKind {
		if relstrength.IsInteractionKind(kind) && !relstrength.IsParticipantKind(kind) {
			t.Errorf("%q is scored as an interaction but records no participants — the graph would have a relationship with nobody in it", kind)
		}
	}
}

// A message has a room and no settled warmth. Both halves are asserted because
// each one is a decision somebody could undo without noticing the other: adding
// it to the scoring set moves every installation's deal-health and
// person-strength numbers, and removing it from the participant set puts a
// group chat back to naming nobody.
func TestAMessageHasParticipantsAndIsNotScored(t *testing.T) {
	t.Parallel()
	if !relstrength.IsParticipantKind("message") {
		t.Error("a message records no participants — a group chat names everyone who was in it, and this is where that is kept")
	}
	if relstrength.IsInteractionKind("message") {
		t.Error("a message scores as an interaction — whether chat traffic is warmth is an open product question, and answering it here moves every deal-health and person-strength number")
	}
}

// Neither set admits intent or thinking. A rep's own to-do list is not a
// relationship, and an unlinked note is not a conversation with anybody.
func TestNeitherSetAdmitsANoteOrATask(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"note", "task"} {
		if relstrength.IsInteractionKind(kind) || relstrength.IsParticipantKind(kind) {
			t.Errorf("%q is in a set; it is one person's intent rather than an exchange", kind)
		}
	}
}

// The SQL renderings are the same sets. A query that filtered on a literal
// would drift from the Go paths silently, which is what these renderers exist
// to prevent — so they have to actually agree.
func TestTheSQLRenderingsAreTheSameSets(t *testing.T) {
	t.Parallel()
	renderings := map[string]func(string) bool{
		relstrength.InteractionKindSQLList(): relstrength.IsInteractionKind,
		relstrength.ParticipantKindSQLList(): relstrength.IsParticipantKind,
	}
	for list, admits := range renderings {
		rendered := strings.Split(strings.ReplaceAll(list, "'", ""), ",")
		for _, kind := range everyKind {
			inList := false
			for _, got := range rendered {
				if got == kind {
					inList = true
				}
			}
			if inList != admits(kind) {
				t.Errorf("the list %s and the Go check disagree about %q", list, kind)
			}
		}
	}
}

// The parenthesised form is the list and nothing else. It exists so a caller
// writes `kind IN %s` rather than remembering the brackets, and a second
// spelling of the set inside it is exactly the drift this package prevents.
func TestTheGroupIsTheListInBrackets(t *testing.T) {
	t.Parallel()
	if want := "(" + relstrength.InteractionKindSQLList() + ")"; relstrength.InteractionKindSQLGroup() != want {
		t.Errorf("the group is %q, want %q", relstrength.InteractionKindSQLGroup(), want)
	}
}
