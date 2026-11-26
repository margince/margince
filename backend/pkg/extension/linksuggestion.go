// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "context"

// LinkSuggestionRepo is the door a unit proposes where a message belongs
// through: `tx.Core().LinkSuggestions().Propose(…)`.
//
// A proposal attaches nothing. It waits in a review queue until a reader
// accepts or rejects it, and only an accept files the message. Core makes no
// suggestion of its own, so the queue holds what units supplied.
type LinkSuggestionRepo interface {
	// Propose suggests one record for one message, under the caller's own rights.
	//
	// The caller must be able to update the message and to attach to the
	// target. A message or target the caller cannot see answers ErrNotFound, so
	// the door cannot be used to learn which records exist. A proposal of the
	// same target for the same message is answered LinkSuggestionExisting and
	// writes nothing, whatever became of the first.
	Propose(ctx context.Context, in LinkSuggestionRequest) (LinkSuggestionOutcome, error)
}

// LinkSuggestionRequest is one proposal. It carries no score about a user,
// and core keeps none.
type LinkSuggestionRequest struct {
	// Activity is the id of the message.
	Activity string
	// Target is the record the message may belong to. Its Type is contact,
	// company, deal or project.
	Target RecordRef
	// Reason is one sentence, at most 200 characters, shown to the reader who
	// decides.
	Reason string
	// Score is how sure the unit is, from 0 to 1. The queue shows it and does
	// not act on it.
	Score float64
}

// LinkSuggestionOutcome says what a proposal did.
type LinkSuggestionOutcome string

const (
	// LinkSuggestionCreated is a new suggestion waiting for a reader.
	LinkSuggestionCreated LinkSuggestionOutcome = "created"
	// LinkSuggestionExisting is a repeat of an earlier proposal.
	LinkSuggestionExisting LinkSuggestionOutcome = "existing"
	// LinkSuggestionExcluded is the message's own author named as the target.
	// Core drops it and writes nothing.
	LinkSuggestionExcluded LinkSuggestionOutcome = "excluded"
)
