// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// An undecided domain, drawn and ranked as the judgement it is.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// sourceDomainQuestion names the undecided-domain producer. A constant for the
// reason the other sources are: the classifier, the destination map and the
// verb census each spell it, and a misspelt literal in any of them would file
// the row on a screen nobody chose, silently.
const sourceDomainQuestion = crmcontracts.AttentionItemSource("domain_question")

// The two verbs an open domain question offers, which are the whole answer a
// human owes it. Named because the renderer advertises them and the census
// checks them against what the client performs.
const (
	verbKeep    = crmcontracts.AttentionItemActions("keep")
	verbDiscard = crmcontracts.AttentionItemActions("discard")
)

// domainQuestionItem draws one open question.
//
// The DOMAIN is the title, because it is the entire subject: the reader is
// being asked about a name they will recognise or not, and any sentence built
// around it would be the product talking about itself. The machine's reason is
// the supporting line.
//
// It carries no `subject`. The row is about a domain, and a domain is not a
// record with a page — there is nothing to open, which is why the two verbs
// settle it in place rather than linking somewhere.
func domainQuestionItem(q DomainQuestion) crmcontracts.AttentionItem {
	domain := q.Domain
	reason := q.Reason
	asked := q.AskedAt
	reasonCode := q.ReasonCode
	actions := []crmcontracts.AttentionItemActions{verbKeep, verbDiscard, crmcontracts.AttentionItemActionsReopen}
	if q.ReasonCode == contacts.PendingForwardedIdentity {
		actions = []crmcontracts.AttentionItemActions{verbDiscard, crmcontracts.AttentionItemActionsReopen}
	}
	return crmcontracts.AttentionItem{
		Id:               q.Domain,
		Source:           sourceDomainQuestion,
		DomainReasonCode: &reasonCode,
		Title:            &domain,
		Detail:           &reason,
		OccurredAt:       &asked,
		SubjectMovedAt:   q.EvidenceAt,
		Actions:          actions,
	}
}
