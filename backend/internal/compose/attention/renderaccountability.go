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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// legalDeadlineItem is one obligation whose clock the law started — a subject
// request or a disclosure duty. Both draw identically, so they share the draw.
//
// NO SUBJECT AND NO VERBS, deliberately, for both. The card's whole subject is
// the obligation rather than a record with a page, and meeting it happens on
// another screen — the case queue for a request, the person's own screen for a
// disclosure. A verb here would promise something this queue cannot complete.
//
// `kind` carries what a reader needs to decide anything: for a request, what was
// asked; for a duty, which ARTICLE obliges it, because an Art. 14 duty means the
// subject does not know we hold their data at all.
func legalDeadlineItem(
	id ids.UUID, kind string, due time.Time, source string, asOf time.Time,
) crmcontracts.AttentionItem {
	past := deadline.Passed(&due, asOf)
	return crmcontracts.AttentionItem{
		Id:      id.String(),
		Source:  crmcontracts.AttentionItemSource(source),
		Kind:    &kind,
		DueAt:   &due,
		Overdue: &past,
		Actions: []crmcontracts.AttentionItemActions{},
	}
}

// dsrItem is one data-subject request whose clock is running.
func dsrItem(request DSRCase, asOf time.Time) crmcontracts.AttentionItem {
	return legalDeadlineItem(request.ID, request.Kind, request.DueAt, "dsr", asOf)
}

// noticeCaseItem is one disclosure duty whose deadline is running.
//
// IT NAMES THE PERSON AND OFFERS `open`, where the DSR card beside it does
// neither, and the difference is not a style choice: a subject request is worked
// on the case queue's own screen, and a notice case has no screen — the
// disclosure is sent from the person's own page. A card carrying only the
// article and the deadline would prompt a privacy admin with nowhere to go, and
// several such cards would be indistinguishable from each other.
//
// A BLOCKED case draws exactly like any other. A duty nobody can discharge is
// the one most worth a reader's attention, and drawing it differently would
// invite a client to filter it out; the obstacle is read on the person's page.
func noticeCaseItem(owed NoticeCase, asOf time.Time) crmcontracts.AttentionItem {
	item := legalDeadlineItem(owed.ID, owed.Rule, owed.DueAt, "notice_case", asOf)
	item.Subject = subjectOf("person", owed.PersonID)
	if openableSubject(item.Subject) {
		item.Actions = []crmcontracts.AttentionItemActions{actionOpen}
	}
	return item
}
