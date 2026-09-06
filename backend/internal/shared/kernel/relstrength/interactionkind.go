// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "strings"

// interactionKinds is the closed set of activity kinds that represent a real
// exchange worth SCORING: the deal-health window, person strength, and the
// organization signal scan.
//
// A task is intent and a note is a record of thinking; neither means two people
// spoke, and counting them would let a rep's own to-do list score as a
// relationship.
//
// It is unexported and reached only through the SQL renderers below, because
// every reader of it is a query.
var interactionKinds = []string{"email", "call", "meeting"}

// participantKinds is the closed set of kinds that HAVE participants — an
// activity where it is meaningful to ask who was there.
//
// TWO QUESTIONS, TWO SETS, AND THE DIFFERENCE IS ONE KIND. "Who was in the
// room" and "does this count as warmth" were one set until a group chat
// arrived, because until then the answers never differed: a task and a note
// have no room AND no warmth. A chat message has a room full of people and an
// unsettled claim to warmth — nobody has decided whether a hundred one-line
// replies mean a relationship the way a meeting does — so folding it into one
// set would have answered that unasked question by moving every installation's
// deal-health and person-strength numbers.
//
// This set is therefore the WIDER one, and only ever that way round: a kind may
// be worth recording the people on without being worth scoring, while a kind
// scored with nobody recorded on it would be a relationship with no one in it.
// backend/gates/activitykindsets_test.go holds that direction, and holds both
// sets against the contract's own kind vocabulary.
//
// Unexported for the same reason: a caller that could append to it would change
// what four different writers stamp, and those four must agree or a captured
// conversation carries the people on it while an identical hand-logged one does
// not — live capture stamping, hand-logged stamping, the historical backfill's
// SQL, and the replay pass.
var participantKinds = []string{"email", "call", "meeting", "message"}

// IsParticipantKind answers whether it is meaningful to record who was on an
// activity of this kind. Every Go writer of activity_participant asks this one.
func IsParticipantKind(kind string) bool {
	for _, k := range participantKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// InteractionKindSQLList renders the scoring set as a SQL IN list, so a query
// filters on the same set the Go paths do instead of restating it as a literal
// that can drift. The values are compile-time constants of this package, never
// input.
func InteractionKindSQLList() string {
	return sqlList(interactionKinds)
}

// InteractionKindSQLGroup is the same list already parenthesised, for the
// `kind IN %s` shape the deal-health and person-strength queries use.
func InteractionKindSQLGroup() string {
	return "(" + InteractionKindSQLList() + ")"
}

// ParticipantKindSQLList renders the participant set for the backfill, which
// selects the activities that should have participant rows and does not.
func ParticipantKindSQLList() string {
	return sqlList(participantKinds)
}

func sqlList(kinds []string) string {
	return "'" + strings.Join(kinds, "','") + "'"
}
