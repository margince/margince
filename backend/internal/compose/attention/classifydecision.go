// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a staged decision IS, for the queue that has to rank it against a
// customer.
//
// Split from the other classifiers on the file-length ceiling, and it is the
// right seam: everything here answers one question the rest do not — whether a
// decision holds up a person, or is the routine tidying that groups.

import (
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// classifyDecision: a staged proposal or a duplicate pair.
//
// The split is what keeps the queue honest. A decision about a SEND blocks
// customer work and sits at level 5; contact hygiene — capturing a counterparty,
// merging two records — is level 6 and never outranks a customer, however many
// of them are waiting. That is the whole answer to a queue of 188 identical
// contact questions burying an unanswered buyer.
func classifyDecision(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	level, consequence, why := levelRoutine, crmcontracts.WorklistItemConsequence("data_drifts"), "routine"
	if blocksCustomerWork(item) {
		level, consequence, why = levelBlocking, "work_blocked", "blocks_customer_work"
	}
	row := base(item, level, "decisions", consequence)
	row.Because = []crmcontracts.WorklistReason{reason(crmcontracts.WorklistReasonKind(why), nil)}
	// The two facts a routine contact decision is grouped by. They ride on the
	// wire item's `detail`, which the feed fills from the staged payload — the
	// verdict engine already decided who the address belongs to, and deriving
	// it again here would put one decision in two groups across two reads.
	facts := stagedFactsOf(item)
	return ranked{
		item:          row,
		occurredAt:    occurredOf(item, asOf),
		machineSender: facts.MachineSender,
		knownCompany:  facts.KnownCompany,
	}
}

// The markers the feed stamps on a contact decision's detail, so the queue can
// group it without reading the payload a second time. Two words rather than a
// structure, because `detail` is one line on the wire and this is all it needs
// to carry.
const (
	stagedMachineSender = "machine_sender"
	stagedKnownCompany  = "known_company"
)

// stagedFactsOf reads those markers back.
func stagedFactsOf(item crmcontracts.AttentionItem) StagedFacts {
	if item.Detail == nil {
		return StagedFacts{}
	}
	return StagedFacts{
		MachineSender: strings.Contains(*item.Detail, stagedMachineSender),
		KnownCompany:  strings.Contains(*item.Detail, stagedKnownCompany),
	}
}

// blocksCustomerWork reports whether a staged decision is holding up something
// a customer is waiting on, as opposed to tidying the database.
//
// The list is the approval kinds whose SUBJECT is an outbound act. A kind absent
// here is treated as hygiene, which is the safe direction: mislabelling a merge
// as urgent costs a reader their attention, while the reverse costs them only a
// place in a queue they are working through anyway.
func blocksCustomerWork(item crmcontracts.AttentionItem) bool {
	if item.Kind == nil {
		return false
	}
	switch *item.Kind {
	case "send_email", "send_account_email", "send_message",
		"book_meeting",
		"deal_follow_up", "transcript_proposal", "site_lead":
		return true
	case kindScheduledSend:
		// A message the rep already MEANT to send, stopped at send time. The
		// accept verb on it reads "yes, still send this", and somebody is
		// waiting at the other end — so it blocks a customer exactly as a send
		// does, and never groups.
		return true
	case kindHeldDraft:
		// A message the product WROTE and is holding: a suggestion nobody has
		// agreed to yet. Five hundred of them are one kind of work, and
		// treating each as its own urgent row is what made the old page
		// unreadable, so they group and the group sits where routine work sits.
		return false
	default:
		return false
	}
}
