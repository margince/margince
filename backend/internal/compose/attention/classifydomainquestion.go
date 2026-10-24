// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// An undecided domain, drawn and ranked as the judgement it is.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
)

// sourceDomainQuestion names the undecided-domain producer. A constant for the
// reason the other sources are: the classifier, the destination map and the
// verb census each spell it, and a misspelt literal in any of them would file
// the row on a screen nobody chose, silently.
const sourceDomainQuestion = crmcontracts.AttentionItemSource("domain_question")

// The verbs an open domain question offers, which are the whole answer a
// human owes it. Named because the renderer advertises them and the census
// checks them against what the client performs. Only a vendor question offers
// keep_as_vendor, which agrees with the triage instead of creating a company.
const (
	verbKeep         = crmcontracts.AttentionItemActions("keep")
	verbKeepAsVendor = crmcontracts.AttentionItemActions("keep_as_vendor")
	verbDiscard      = crmcontracts.AttentionItemActions("discard")
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
	var note *string
	switch q.ReasonCode {
	case contacts.PendingForwardedIdentity:
		actions = []crmcontracts.AttentionItemActions{verbDiscard, crmcontracts.AttentionItemActionsReopen}
	case contacts.PendingVendorVerdict:
		// No reopen: it would read the same front page and ask the same question.
		actions = []crmcontracts.AttentionItemActions{verbKeep, verbKeepAsVendor, verbDiscard}
		if q.Note != "" {
			note = &q.Note
		}
	}
	return crmcontracts.AttentionItem{
		Id:               q.Domain,
		Source:           sourceDomainQuestion,
		DomainReasonCode: &reasonCode,
		DomainNote:       note,
		Title:            &domain,
		Detail:           &reason,
		OccurredAt:       &asked,
		SubjectMovedAt:   q.EvidenceAt,
		Actions:          actions,
	}
}

// classifyDomainQuestion: a judgement, ranked with the other judgements.
//
// Routine rather than blocking, and that is the honest level. Nothing is
// waiting on the answer the way a customer waits on a reply — the mail was
// already captured or already dropped, and what hangs on this is whether a
// company record exists. Ranking it as blocking would put a domain nobody has
// heard of above a buyer who is waiting, which is the failure the decision
// levels exist to prevent.
func classifyDomainQuestion(item crmcontracts.AttentionItem, asOf time.Time) ranked {
	row := base(item, levelRoutine, "decisions", "data_drifts")
	row.Because = []crmcontracts.WorklistReason{reason("routine", nil)}
	return ranked{
		item:       row,
		occurredAt: occurredOf(item, asOf),
		// The reader's own, by construction of the read. OpenDomainQuestions
		// binds to the acting human, so no colleague's question could have come
		// back — the same claim the capture lane makes about a mailbox, and the
		// reason this row needs no owner field of its own.
		ownerRef: ownedByWhoeverIsReading(),
	}
}
