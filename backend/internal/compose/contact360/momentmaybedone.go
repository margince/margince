// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// A promise card that asks instead of nagging. We may have kept the promise in
// a later mail on its thread to the contact. The card then offers Done and Not
// yet, and never closes the promise itself.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// doneAsks is what the page's promise cards may ask. A claim asks over our
// later mail on its thread to this contact. A task asks the question every
// surface asks of it (momentaction.TasksDoneTx).
type doneAsks struct {
	contact ids.ContactID
	claims  map[ids.UUID]activities.PromiseFollowUp
	tasks   map[ids.UUID]momentaction.TaskAsked
}

// doneAsksFor reads what the page's promise cards may ask. A reader who may
// not read activity sees no mail to cite, so their cards stay as they are.
func (s *Service) doneAsksFor(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, now time.Time, page *crmcontracts.Contact360) (doneAsks, error) {
	asks := doneAsks{contact: contactID}
	var claims []momentaction.DonePromise
	var tasks []momentaction.PromiseTask
	for _, item := range owedPromises(page) {
		switch promise := item.Ref.(type) {
		case crmcontracts.Activity:
			tasks = append(tasks, promiseTask(promise))
		case crmcontracts.ConversationClaim:
			if claim, ok := claimPromise(contactID, promise); ok {
				claims = append(claims, claim)
			}
		}
	}
	var err error
	if asks.claims, err = momentaction.DoneFollowUpsTx(ctx, tx, claims, now); err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return doneAsks{}, err
	}
	candidates, err := momentaction.DoneCandidates(ctx, s.contacts, tasks)
	if err != nil {
		return doneAsks{}, err
	}
	if asks.tasks, err = momentaction.TasksDoneTx(ctx, tx, candidates, now); err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
		return doneAsks{}, err
	}
	return asks, nil
}

func promiseTask(task crmcontracts.Activity) momentaction.PromiseTask {
	var system string
	if task.SourceSystem != nil {
		system = *task.SourceSystem
	}
	return momentaction.PromiseTask{ID: ids.UUID(task.Id), System: system, Source: (*ids.UUID)(task.SourceActivityId), Assignee: (*ids.UUID)(task.AssigneeId)}
}

func claimPromise(contactID ids.ContactID, claim crmcontracts.ConversationClaim) (momentaction.DonePromise, bool) {
	return momentaction.ClaimDone(ids.UUID(claim.Id), ids.UUID(claim.SourceActivityId), contactID.UUID, (*ids.UUID)(claim.TaskActivityId))
}

// itemBehind finds the owed item a promise card was built from. A card from
// any other rung has none.
func itemBehind(moment crmcontracts.ContactMoment, page *crmcontracts.Contact360) (owedwork.Item, bool) {
	taskCard := (moment.ClaimKey == overdueTaskKey || moment.ClaimKey == openTaskKey) && len(moment.Evidence) > 0 && moment.Evidence[0].Id != nil
	for _, item := range owedPromises(page) {
		switch promise := item.Ref.(type) {
		case crmcontracts.Activity:
			if taskCard && promise.Id == *moment.Evidence[0].Id {
				return item, true
			}
		case crmcontracts.ConversationClaim:
			if moment.ClaimKey == claimMomentKey(overdueClaimRung, promise) || moment.ClaimKey == claimMomentKey(openClaimRung, promise) {
				return item, true
			}
		}
	}
	return owedwork.Item{}, false
}

// mayBeDone turns a promise card into the question whether our later mail
// kept it.
func (said momentCopy) mayBeDone(moment crmcontracts.ContactMoment, page *crmcontracts.Contact360, asks doneAsks) (crmcontracts.ContactMoment, bool) {
	item, ok := itemBehind(moment, page)
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	switch promise := item.Ref.(type) {
	case crmcontracts.Activity:
		// The question is asked on the page of the recipient the mail reached,
		// because that page keeps its Not yet for every surface.
		asked, ok := asks.tasks[ids.UUID(promise.Id)]
		if !ok || asked.Contact != asks.contact.UUID {
			return crmcontracts.ContactMoment{}, false
		}
		return momentaction.AskTaskDone(said.Spoken, said.zone, moment, ids.UUID(promise.Id), asked), true
	case crmcontracts.ConversationClaim:
		claim, ok := claimPromise(asks.contact, promise)
		if !ok {
			return crmcontracts.ContactMoment{}, false
		}
		return momentaction.AskPromiseDone(said.Spoken, said.zone, moment, claim, asks.claims)
	default:
		return crmcontracts.ContactMoment{}, false
	}
}

// proposer asks the may-be-done question in place of a promise card. After the
// reader answers Not yet to it, the plain card comes back.
func (said momentCopy) proposer(
	page *crmcontracts.Contact360, asks doneAsks, dismissed func(crmcontracts.ContactMoment) bool,
) func(crmcontracts.ContactMoment) crmcontracts.ContactMoment {
	return func(moment crmcontracts.ContactMoment) crmcontracts.ContactMoment {
		if question, ok := said.mayBeDone(moment, page, asks); ok && !dismissed(question) {
			return question
		}
		return moment
	}
}

// proposing is a rung whose promise card becomes the may-be-done question
// whenever propose says so.
func proposing(rung ladderRung, propose func(crmcontracts.ContactMoment) crmcontracts.ContactMoment) ladderRung {
	return func(ctx context.Context, now time.Time, page *crmcontracts.Contact360) (crmcontracts.ContactMoment, bool) {
		moment, ok := rung(ctx, now, page)
		if !ok {
			return moment, false
		}
		return propose(moment), true
	}
}

// asAssembled proposes nothing: a page read without mail keeps its promise cards.
func asAssembled(moment crmcontracts.ContactMoment) crmcontracts.ContactMoment { return moment }
