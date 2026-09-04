// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the product reports about DELIVERY and about itself.
//
// Three classifiers with one thing in common: none of them is a record a seller
// worked on. A bounce and an undelivered message are consequences a customer is
// on the other end of; the pipes are the product saying its own machinery
// stopped. They sit together because a reader meeting one of them is being told
// something did not happen rather than being handed something to do, and the
// difference decides both the words on the row and the screen it lands on.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The two system sources that are ONE person's: a mailbox belongs to whoever
// connected it, and a notice is addressed to whoever it names. Spelled as
// constants because both the consequence switch and the ownership switch read
// them, and a typo in either would silently move a row to the wrong answer.
const (
	sourceCaptureHealth = crmcontracts.AttentionItemSource("capture_health")
	sourceNotice        = crmcontracts.AttentionItemSource("notice")
)

// classifyBounce: a customer never received something the rep believes they
// sent. A consequence with a named customer and a verb, so it ranks as its own
// row rather than disappearing into an aggregate.
func classifyBounce(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "customer_never_received")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{
		item:       row,
		occurredAt: occurredOf(item, asOf),
		// The lane is bound to the acting user, so no other person's row could
		// have come back.
		ownerRef: ownedByWhoeverIsReading(),
	}
}

// classifyUndelivered: a message the rep believes they sent and which never
// left. The belief is the damage — they are waiting on a reply to something
// nobody has — so it sits with the broken promises rather than with the system
// news, exactly where a bounce sits.
//
// It is a separate consequence from the bounce beside it: nobody received this
// one because nobody was ever sent it, and the reader's move is to send it
// rather than to fix an address.
func classifyUndelivered(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelPromise, "system", "you_believe_it_happened")
	row.Because = []crmcontracts.WorklistReason{reason("approved_and_failed", nil)}
	return ranked{
		item:       row,
		occurredAt: occurredOf(item, asOf),
		// The lane is bound to the acting user, so no other person's row could
		// have come back.
		ownerRef: ownedByWhoeverIsReading(),
	}
}

// classifySystem: the pipes. A mailbox that stopped capturing makes every quiet
// claim on this page suspect, which is why it is here at all rather than only in
// an admin screen.
func classifySystem(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	consequence := crmcontracts.WorklistItemConsequence("work_blocked")
	if item.Source == sourceCaptureHealth || item.Source == "sync_health" {
		consequence = "mailbox_blind"
	}
	if item.Source == sourceNotice {
		consequence = valueNone
	}
	row := base(item, levelBlocking, "system", consequence)
	return ranked{
		item:       row,
		occurredAt: occurredOf(item, asOf),
		// ONE classifier, two different truths — which is why this branches
		// rather than stating a single answer for everything it draws.
		ownerRef: systemRowOwner(item.Source),
	}
}

// systemRowOwner separates the personal system rows from the workspace ones.
//
// classifySystem draws five sources and they do not agree about ownership. A
// notice is addressed to one person and a mailbox belongs to one person: both
// lanes read per-user, so the row is the reader's by construction. A failing
// sync, a broken AI task and a stopped automation are the WORKSPACE's — those
// services read no user at all, several admins see the same row, and naming
// whoever opened the page would make one shared failure look like five people's
// separate problems.
//
// Stated here rather than in the classifier's literal so the difference is
// visible: a source added to that switch has to answer this question too.
func systemRowOwner(source crmcontracts.AttentionItemSource) ownerRef {
	switch source {
	case sourceNotice, sourceCaptureHealth:
		return ownedByWhoeverIsReading()
	default:
		return unassigned()
	}
}
