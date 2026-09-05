// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Which screen a row belongs on, decided once.
//
// A worklist row is not one kind of thing. A buyer waiting on a reply is work a
// seller executes; a duplicate pair is a judgement somebody makes before work
// continues; a stopped mailbox is a source an administrator restores. Putting
// all three in one list under one vocabulary is what made the queue read as a
// feed: a rep scanning for the next call had to step over the product's own
// housekeeping to find it.
//
// THE SERVER DECIDES, ONCE, HERE. The counts, the folding and the page cut all
// read the value the row carries, so a client that groups by it cannot disagree
// with the figures above it. The alternative — a client-side map over `source`
// — was rejected because it is a second authority: the day the two spellings
// drift, a row sits on one screen while the count above it says another, and
// nothing fails.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The four screens a row can belong to.
const (
	// destinationToday is work a seller executes: a reply to write, a task to
	// finish, a meeting to prepare, a deal to move.
	destinationToday = crmcontracts.WorklistDestinationToday
	// destinationReview is a judgement a human makes before work can continue —
	// an approval, a duplicate pair, an introduction to accept.
	destinationReview = crmcontracts.WorklistDestinationReview
	// destinationSystemHealth is a source or an automation an administrator
	// restores. A seller can rarely act on one, and a queue that asked them to
	// spent their attention on somebody else's job.
	destinationSystemHealth = crmcontracts.WorklistDestinationSystemHealth
)

// destinationOfSource is the whole mapping, and the only one.
//
// EXHAUSTIVE over the source enum, held by TestEverySourceHasADestination: a
// twenty-second source fails that test until somebody decides where it belongs.
// The failure is the point. A source added without a destination would default
// to whatever the zero value is, and the reader would find it on a screen
// nobody chose for it.
//
// `dsr` is deliberately absent from seller work: a subject request is a legal
// obligation with its own authorized queue and its own clock, and a seller
// usually cannot act on one at all. It is `review` here so the count above the
// queue stays honest about it existing, and the sales screens filter it out by
// destination rather than by naming the source a second time.
//
// `batch` has NO entry, and cannot: it stands for a group of other rows, so its
// destination is its members'. `destinationOfBatch` answers for it.
var destinationOfSource = map[crmcontracts.WorklistItemSource]crmcontracts.WorklistItemDestination{
	// Seller work. Somebody is waiting, a date is arriving, or revenue moves.
	crmcontracts.WorklistItemSourceCustomerWaiting:   destinationToday,
	crmcontracts.WorklistItemSourceLeadResponse:      destinationToday,
	crmcontracts.WorklistItemSourceDealAtRisk:        destinationToday,
	crmcontracts.WorklistItemSourceMeeting:           destinationToday,
	crmcontracts.WorklistItemSourceMeetingOutcome:    destinationToday,
	crmcontracts.WorklistItemSourceTask:              destinationToday,
	crmcontracts.WorklistItemSourceConversationClaim: destinationToday,
	crmcontracts.WorklistItemSourceBriefItem:         destinationToday,
	// A contact going quiet is a seller's call to make — reach out, or decide
	// this relationship is over. The dismissal path exists because it is a
	// judgement about their own week, not a queue of somebody else's.
	crmcontracts.WorklistItemSourceRelationshipDecay: destinationToday,
	// A bounced or undelivered message is a customer consequence: this named
	// person did not receive what was sent to them, and the seller is the one
	// who notices. The mailbox that carried it may be healthy.
	crmcontracts.WorklistItemSourceBounce:      destinationToday,
	crmcontracts.WorklistItemSourceUndelivered: destinationToday,
	// The rep pressed Accept and believes it happened, and that belief is the
	// damage. It is classified at the promise level and carried back to the
	// APPROVER rather than to an administrator, so it is that person's own
	// broken promise — the same shape as the undelivered message beside it, and
	// not a pipe anybody else can restore.
	crmcontracts.WorklistItemSourceFailedApproval: destinationToday,
	// A notice is addressed to one reader and offers `acknowledge`. Nothing
	// waits on a judgement and nothing is broken: reading it IS the work, and it
	// leaves the lane when they do.
	crmcontracts.WorklistItemSourceNotice: destinationToday,

	// Judgements. Work waits on a person deciding.
	crmcontracts.WorklistItemSourceApproval:            destinationReview,
	crmcontracts.WorklistItemSourceDedupeCandidate:     destinationReview,
	crmcontracts.WorklistItemSourceIntroductionRequest: destinationReview,
	crmcontracts.WorklistItemSourceDsr:                 destinationReview,

	// The product reporting on itself. An administrator restores these.
	crmcontracts.WorklistItemSourceSyncHealth:    destinationSystemHealth,
	crmcontracts.WorklistItemSourceCaptureHealth: destinationSystemHealth,
	crmcontracts.WorklistItemSourceAiWorkHealth:  destinationSystemHealth,
	crmcontracts.WorklistItemSourceAutomationRun: destinationSystemHealth,
}

// destinationOf answers for one row.
//
// A batch asks its members, because a group's destination is not a property of
// the word `batch` — it is whatever the rows inside it were. Folding is only
// ever applied to rows that already agree (see destinationOfBatch), so the
// first member answers for all of them.
//
// A source with no entry answers `review` rather than `today`. That is the safe
// direction: an unclassified row on the review screen is one row somebody has
// to look at, while the same row in Today claims to be executable seller work
// and offers a verb nobody wrote. The census test means this fallback should
// never run in production — it is what happens if it ever does.
func destinationOf(row ranked) crmcontracts.WorklistItemDestination {
	if row.item.Batch != nil {
		return destinationOfBatch(row)
	}
	if at, ok := destinationOfSource[row.item.Source]; ok {
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

// ClassifiedSources is which sources this map decides a screen for.
//
// Exported for the gate that holds the map exhaustive against the contract's
// own enum. It is the map's key set rather than a second list: a gate that
// hard-codes part of its subject has become a copy of it, and the copy is what
// goes stale.
func ClassifiedSources() []crmcontracts.WorklistItemSource {
	out := make([]crmcontracts.WorklistItemSource, 0, len(destinationOfSource))
	for source := range destinationOfSource {
		out = append(out, source)
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
	return destinationOfSource[source]
}

// destinationPtr is the wire field's shape: optional, so an older client that
// never heard of the field keeps working, and always sent by this server.
func destinationPtr(at crmcontracts.WorklistItemDestination) *crmcontracts.WorklistItemDestination {
	return &at
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
