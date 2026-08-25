// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the query vocabulary. See toolcopy.go for what each field
// answers.

var describeQueryVocabularyCopy = toolCopy{
	Purpose: "Answer what a query plan may SAY in this workspace: the record types that can be " +
		"asked about, the fields nameable on each, the operators each field admits, and the one " +
		"relationship hop a plan may take. It is the vocabulary query_workspace refuses against, " +
		"so it holds the spelling of a field whose name a plan got wrong.",
	Limits: "It describes the vocabulary; it returns no records — query_workspace does that. " +
		"What comes back is narrowed to what you may already read, so it names nothing you could " +
		"not otherwise reach.",
	Instead: "Call query_workspace once you know the names. This tool answers the same document " +
		"as the margince://schema/query resource, for a client that reads tools rather than " +
		"resources.",
	Retain: "Take the field and operator names from `targets` verbatim — a plan naming anything " +
		"outside them is refused rather than approximated, so guessing at a spelling costs a " +
		"round trip. `grammar` says how the clauses are assembled, and `version` is the value a " +
		"plan's own `version` member must carry.",
}
