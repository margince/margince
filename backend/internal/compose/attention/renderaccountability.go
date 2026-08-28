// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The cards of the two accountability lanes: work a decision released that
// did not run, and requests whose deadlines the law set. Both report; the
// acting happens on the surfaces that own the rows.

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
)

// failedItem is one approved decision whose released work did not run,
// carried back to the person who approved it. The sentence was written for
// the reader when the failure was recorded; `open` is offered only when the
// decision named a record the client can route to.
func failedItem(failed FailedEffect) crmcontracts.AttentionItem {
	kind := failed.Kind
	sentence := failed.Sentence
	occurred := failed.FailedAt
	subject := subjectOf(failed.TargetType, failed.TargetID)
	actions := []crmcontracts.AttentionItemActions{}
	if openableSubject(subject) {
		actions = append(actions, actionOpen)
	}
	return crmcontracts.AttentionItem{
		Id:         failed.ID.String(),
		Source:     crmcontracts.AttentionItemSource("failed_approval"),
		Kind:       &kind,
		Title:      &sentence,
		Subject:    subject,
		OccurredAt: &occurred,
		Actions:    actions,
	}
}

// dsrItem is one data-subject request whose clock is running. The card
// carries no subject and no verbs on purpose: the subject of a DSR is the
// REQUEST, not a record with a page, and fulfilment lives on the case
// queue's own screen — this card only makes sure somebody is prompted to
// open it before the deadline does.
func dsrItem(request DSRCase, asOf time.Time) crmcontracts.AttentionItem {
	kind := request.Kind
	due := request.DueAt
	past := deadline.Passed(&due, asOf)
	return crmcontracts.AttentionItem{
		Id:      request.ID.String(),
		Source:  crmcontracts.AttentionItemSource("dsr"),
		Kind:    &kind,
		DueAt:   &due,
		Overdue: &past,
		Actions: []crmcontracts.AttentionItemActions{},
	}
}
