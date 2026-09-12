// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which staged proposals belong to ONE seat rather than to the shared inbox.
//
// The inbox is a shared surface by design — a manager triages what a rep staged
// — so everything here is an exception to that, and each entry says which kind
// of exception it is. Split out of authority.go because it is one concept with
// one predicate, and because that file had grown past the length a reader can
// hold at once.

package approvals

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// selfOnlyKinds are the staging kinds whose proposal is nobody's business but
// the member it was staged for.
//
// The inbox is a SHARED surface by design — a manager triages what a rep
// staged — and for almost every kind that is the point. It is wrong for one:
// a LinkedIn match names a third party out of one member's imported address
// book, contacts who never agreed to be in this CRM at all. The endpoints this
// kind replaced were owner-only and said so; routing the same question through
// a shared inbox would have handed every admin a readable copy of a
// colleague's contact list, which is a bigger disclosure than the feature it
// enables.
//
// So a self-only kind adds one predicate to the two below: the deciding human
// must BE the member it was staged for. It is the inbox's mirror of the
// webhooks module's selfOnlyEvents, which keeps the same three LinkedIn facts
// off the workspace fan-out for the same reason.
//
// A step-up is the other: "may this agent keep reading" is a question about ONE
// connection, and the only colleague who can answer it is the human whose authority
// that connection borrows.
// A held scheduled send is the third: the message is one rep's, the decision is
// whether to retry it or abandon it, and nobody else has standing to answer.
var selfOnlyKinds = map[string]bool{
	kindLinkedInMatch:     true,
	KindVolumeRelease:     true,
	KindScheduledSendHeld: true,
	// A vCard review is one member's own uploaded address book, exactly the
	// LinkedIn-match shape: the staged card names a third party who never
	// agreed to be in this CRM, and a shared inbox would hand every
	// contact:create holder a readable copy of a colleague's contacts.
	"vcard_create": true,
	// A held draft is the fourth, and it is about WHOSE MAILBOX the message
	// leaves from rather than who may read it. Releasing one sends it, and the
	// send stamps its identity from the approving human: comms.stagingUser
	// takes the sending credential from the authenticated principal, and the
	// display name and signature come from that same actor. So a colleague who
	// approved a rep's draft did not authorise the rep's message — they sent
	// their own, into a customer thread they were never part of, signed by
	// themselves.
	//
	// The narrowing puts the decision back with the contact the message would go
	// out as. It is also what kindHeldDraft's own doc has always claimed ("held
	// for the rep it was written for") and what nothing enforced.
	kindHeldDraft: true,
}

// decidedByTheSeatStagedFor is the WEAKER narrowing beside selfOnlyKinds: a
// proposal that is one rep's morning work, decided by the rep it was staged
// for.
//
// The difference from a self-only kind is what a missing stager means. These
// carry no third party's data and no rep's mailbox, so a proposal recording
// nobody is not a disclosure — it is a deal nobody owns, and it stays shared.
//
// Both entries are the overnight sweeps' output about ONE rep's deal. A manager
// holding activity:create saw every rep's, which is not oversight: it is one
// contact's day appearing on somebody else's queue, where answering it takes the
// question away from the rep who was going to act on it.
var decidedByTheSeatStagedFor = map[string]bool{
	// The nightly reconciliation's "this conversation left no next step" card,
	// staged for the deal's owner.
	kindDealFollowUp: true,
	// A next step a transcript recorded somebody committing to, staged for the
	// rep who asked for the recording to be read.
	kindTranscriptProposal: true,
}

// withheldFromOtherSeats is the self-only narrowing of decidable, spelled once
// because three reads apply it: the inbox scan through decidable, and the two
// target-filtered reads (inbox.listForTarget, Service.PendingForTarget) which
// settle target visibility for the record instead of per row and so cannot call
// decidable itself. It reports the rows this caller must NOT see — true when the
// staging is bound to one seat and p is not it.
//
// Two routes to the same predicate: a kind whose subject is one member's own
// business, and a staged create against a table whose rows belong to one human
// each — where no row exists yet for an ownership probe to ask. Fail-closed on a
// missing stager: a proposal nobody is recorded for is one nobody may read, not
// one everybody may.
//
// Held by: TestEveryApprovalsGrantFilterAlsoAppliesTheSelfOnlyNarrowing
// (backend/gates/approvalselfonlyreaders_test.go) — it fails when a reader
// filters rows with requireDecisionGrants and does not also call this.
func withheldFromOtherSeats(p principal.Principal, a row) bool {
	if selfOnlyKinds[a.Kind] || stagedForStagerOnly(a.TargetType, a.TargetID != nil) {
		return a.OnBehalfOf == nil || p.UserID == ids.Nil || a.OnBehalfOf.UUID != p.UserID
	}
	if decidedByTheSeatStagedFor[a.Kind] {
		// The nil case is the OPPOSITE of the arm above, and the difference is
		// the point. A LinkedIn match with no stager recorded is one nobody may
		// read: there is a third party in it and nothing says whose address
		// book they came from, so absence fails closed. A follow-up on a deal
		// nobody owns is honestly everybody's — no seat was skipped, because
		// none exists — so absence leaves it shared.
		return a.OnBehalfOf != nil && (p.UserID == ids.Nil || a.OnBehalfOf.UUID != p.UserID)
	}
	return false
}
