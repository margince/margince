// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "strings"

// interactionKinds is the closed set of activity kinds that represent a real
// exchange worth SCORING: the deal-health window, contact strength, and the
// company signal scan.
//
// The membership test is whether two contacts spoke. A task is intent and a note
// is a record of thinking; neither means two contacts spoke, and counting them
// would let a rep's own to-do list score as a relationship. A chat message
// passes that test the same way an email does, and an account whose whole
// relationship runs over a channel read as having no interactions at all while
// it was excluded.
//
// What a message does NOT share with the others is its unit: one row is one
// exchange for an email, a call and a meeting, and one line of a conversation
// for a message. InteractionUnitSQL below is where that difference is answered,
// and it is answered there rather than by keeping the kind out, because the
// membership question and the counting question have different answers.
//
// It is unexported and reached only through the SQL renderers below, because
// every reader of it is a query.
var interactionKinds = []string{"email", "call", "meeting", "message"}

// participantKinds is the closed set of kinds that HAVE participants — an
// activity where it is meaningful to ask who was there.
//
// TWO QUESTIONS, TWO SETS. "Who was in the room" and "does this count as
// warmth" hold the same four kinds today, and they are still two sets because
// they are still two questions: a kind may be worth recording the contacts on
// without being worth scoring, while a kind scored with nobody recorded on it
// would be a relationship with no one in it. That direction is the one a diff
// may move them apart in, and only that one.
// Held by: TestEveryScoredKindHasParticipants (backend/gates/activitykindsets_test.go),
// which also holds both sets against the contract's own kind vocabulary.
//
// Unexported for the same reason: a caller that could append to it would change
// what four different writers stamp, and those four must agree or a captured
// conversation carries the contacts on it while an identical hand-logged one does
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
// `kind IN %s` shape the deal-health and contact-strength queries use.
func InteractionKindSQLGroup() string {
	return "(" + InteractionKindSQLList() + ")"
}

// InteractionUnitSQL renders what ONE interaction is, for a query that counts
// them: `count(DISTINCT <unit>)` in place of `count(*)`.
//
// An email, a call and a meeting are one interaction per row, because each row
// is one exchange somebody decided to have. A channel message is not: a
// conversation arrives as dozens of one-line replies, and counting rows makes an
// afternoon of chat outweigh a quarter of meetings. FreqSaturation is 20, so
// twenty lines typed in five minutes would fill a ninety-day quota of contact.
//
// So a message counts once per conversation per day. That is the unit a contact
// uses out loud — "we talked on Tuesday" — and unlike a per-message weight it
// introduces no constant, so there is no number that has to be tuned against a
// real channel before the count is honest. Twenty days of talking across the
// quarter saturates frequency, which is a relationship; twenty lines in an
// afternoon is one day of one.
//
// The directed counts read the same unit, so a conversation-day with traffic
// both ways yields the unit under both filters and reads as balanced. That is
// why Inputs does not require the two directions to sum to the total.
//
// UTC decides the day. The boundary only changes whether a conversation running
// past midnight counts once or twice, every fixed zone is arbitrary for a
// workspace spanning several, and UTC is the one a reader can reproduce from the
// stored value alone.
//
// A message with no thread_key falls back to its own id, so it counts as one
// rather than dropping out of the count: a unit that renders NULL is skipped by
// count(DISTINCT), and a count that can silently shrink is the wrong way for
// this to fail. The two prefixes keep the spaces apart, because a thread_key is
// free text from a provider and could otherwise equal a uuid rendered as text.
//
// alias is the statement's alias for `activity`, always a compile-time literal
// at the call site as the other renderers here require.
func InteractionUnitSQL(alias string) string {
	return "CASE WHEN " + alias + ".kind = 'message'" +
		" THEN 'conversation-day:' || coalesce(" + alias + ".thread_key, " + alias + ".id::text)" +
		" || ':' || (" + alias + ".occurred_at AT TIME ZONE 'UTC')::date" +
		" ELSE 'activity:' || " + alias + ".id::text END"
}

// ParticipantKindSQLList renders the participant set for the backfill, which
// selects the activities that should have participant rows and does not.
func ParticipantKindSQLList() string {
	return sqlList(participantKinds)
}

func sqlList(kinds []string) string {
	return "'" + strings.Join(kinds, "','") + "'"
}
