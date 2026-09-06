// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "strings"

// TWO QUESTIONS, TWO SETS, AND THE DIFFERENCE IS ONE KIND.
//
// "Who was in the room" and "does this count as warmth" were one set until a
// group chat arrived, because until then the answers never differed: a task is
// intent and a note is a record of thinking, so neither has a room OR a warmth.
// A chat message has a room full of people and an unsettled claim to warmth —
// nobody has decided whether a hundred one-line replies mean a relationship the
// way a meeting does — and folding it into one set would have answered that
// unasked question by changing every installation's deal-health and
// person-strength numbers.
//
// So the sets are separate and participantKinds is the WIDER one. A kind may be
// worth recording the people on without being worth scoring; the reverse would
// be incoherent, and the test beside this file holds that direction.

// interactionKinds is the closed set of activity kinds that represent a real
// exchange worth SCORING: the deal-health window and person strength.
//
// A task is intent and a note is a record of thinking; neither means two people
// spoke, and counting them would let a rep's own to-do list score as a
// relationship.
var interactionKinds = []string{"email", "call", "meeting"}

// participantKinds is the closed set of kinds that HAVE participants — an
// activity where it is meaningful to ask who was there.
//
// It is unexported for the reason the other set is: a caller that could append
// to it would change what four different writers stamp. Those four must agree,
// or a captured conversation carries the people on it and an identical
// hand-logged one does not — live capture stamping, hand-logged stamping, the
// historical backfill's SQL, and the replay pass.
//
// A message is here and not above. A group chat names everyone who was in it,
// and recording them is what answers "who on our team knows this contact" and
// what puts the third human in the group on the timeline at all. Whether that
// traffic is also warmth is a question about scoring, and it is not this set's
// to answer.
var participantKinds = []string{"email", "call", "meeting", "message"}

// IsInteractionKind answers whether an activity of this kind means two people
// spoke, for the purpose of SCORING a relationship.
func IsInteractionKind(kind string) bool {
	return contains(interactionKinds, kind)
}

// IsParticipantKind answers whether it is meaningful to record who was on an
// activity of this kind. Every writer of activity_participant asks this one.
func IsParticipantKind(kind string) bool {
	return contains(participantKinds, kind)
}

func contains(kinds []string, kind string) bool {
	for _, k := range kinds {
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
