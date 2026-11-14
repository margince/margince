// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which screen a row belongs on: work a seller executes, a judgement before
// work continues, or a source an administrator restores. Each kind's screen is
// stated once, in api/worklist-kinds.yaml, and the counts, the folding and the
// page cut all read the value the row carries.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The four screens a row can belong to.
const (
	// destinationToday is work a seller executes: a reply to write, a task to
	// finish, a meeting to prepare, a deal to move.
	destinationToday = crmcontracts.WorklistItemDestinationWorklistDestinationToday
	// destinationReview is a judgement a human makes before work can continue —
	// an approval, a duplicate pair, an introduction to accept.
	destinationReview = crmcontracts.WorklistItemDestinationWorklistDestinationReview
	// destinationSystemHealth is a source or an automation that stopped. An
	// administrator restores most of them, so a seller can rarely act on one.
	// A queue that asked them to act would spend their attention on somebody
	// else's job.
	destinationSystemHealth = crmcontracts.WorklistItemDestinationWorklistDestinationSystemHealth
)

// destinationOf answers for one row; a batch asks its members. A source with
// no screen answers review: one row somebody looks at, never a claim of seller
// work with a verb nobody wrote.
func destinationOf(row ranked) crmcontracts.WorklistItemDestination {
	if row.item.Batch != nil {
		return destinationOfBatch(row)
	}
	if at := worklistKinds[row.item.Source].destination; at != destinationNone {
		return at
	}
	return destinationReview
}

// destinationOfBatch is the destination a folded group carries.
//
// The group's own row already holds the answer once it is minted: batchRow
// copies its members' destination onto it, and the fold refuses to group rows
// that disagree. Reading the row's own field keeps the group and its members
// saying one thing.
func destinationOfBatch(row ranked) crmcontracts.WorklistItemDestination {
	if row.item.Destination != nil {
		return *row.item.Destination
	}
	return destinationReview
}

// ClassifiedSources is which sources the catalog decides a screen for: every
// kind but a fold. Exported for the gate that holds it exhaustive against the
// contract's own enum, as the catalog's key set rather than a second list.
func ClassifiedSources() []crmcontracts.WorklistItemSource {
	out := make([]crmcontracts.WorklistItemSource, 0, len(worklistKinds))
	for source, kind := range worklistKinds {
		if kind.destination != destinationNone {
			out = append(out, source)
		}
	}
	return out
}

// DestinationOfSource is the screen one source's rows belong on.
//
// Exported beside ClassifiedSources for the same gate: it holds the ANSWERS
// against the contract's declared vocabulary, so a destination spelled here
// that the schema does not carry fails at the gate rather than as a validation
// error on a reader's queue.
func DestinationOfSource(source crmcontracts.WorklistItemSource) crmcontracts.WorklistItemDestination {
	return worklistKinds[source].destination
}

// sameDestination says whether these rows may be folded into one.
//
// A fold puts one row on the page in place of many, so the group has to belong
// to ONE screen. Two rows that would sit on different screens are not the same
// pile however alike they read: folding an approval into a system incident
// would take a judgement somebody owes off the review screen, count it as
// system trouble, and leave nothing behind to notice.
func sameDestination(members []ranked) bool {
	if len(members) < 2 {
		return true
	}
	first := destinationOf(members[0])
	for _, member := range members[1:] {
		if destinationOf(member) != first {
			return false
		}
	}
	return true
}

// destinationOfGroup is the screen a folded row belongs on, asked of its
// members.
//
// The fold refuses to group rows that disagree, so members already sharing one
// screen is the only shape that reaches here — and this asks anyway. The guard
// is eight lines away in another function, which is exactly the distance over
// which a claim stops being true without anybody noticing: a second caller
// reaching batchRow without it would silently file a group wherever its first
// member happened to sit.
//
// Disagreement answers `review` for the reason the unmapped case does: an
// unexpected group on the review screen is one row somebody looks at, while the
// same group in Today claims to be executable seller work.
func destinationOfGroup(members []ranked) crmcontracts.WorklistItemDestination {
	if len(members) == 0 || !sameDestination(members) {
		return destinationReview
	}
	return destinationOf(members[0])
}
