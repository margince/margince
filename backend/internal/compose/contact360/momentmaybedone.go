// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// A promise card that asks instead of nagging. We may have kept the promise in
// a later mail on its thread to the contact. The card then offers Done and Not
// yet, and never closes the promise itself.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promiseFollowUps reads, per open promise on the page, our later mail on its
// thread to this contact (activities.PromiseFollowUpsTx).
func promiseFollowUps(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, now time.Time, page *crmcontracts.Contact360) (map[ids.UUID]activities.PromiseFollowUp, error) {
	var bases []activities.PromiseBasis
	add := func(promise openapi_types.UUID, source *openapi_types.UUID) {
		if source != nil {
			bases = append(bases, activities.PromiseBasis{PromiseID: ids.UUID(promise), SourceActivityID: ids.UUID(*source), RecipientContactID: contactID.UUID})
		}
	}
	if page.NextSteps != nil {
		// Only a task written for a promise read out of a message. A reply
		// reminder has its own judge, and a hand task names no promise.
		for _, task := range page.NextSteps.Data {
			if task.SourceSystem != nil && *task.SourceSystem == activities.CommitmentTaskSource {
				add(task.Id, task.SourceActivityId)
			}
		}
	}
	if page.Claims != nil {
		for _, claim := range *page.Claims {
			if claim.Kind == crmcontracts.ConversationClaimKindCommitmentOurs && claim.Status == crmcontracts.ConversationClaimStatusOpen {
				add(claim.Id, &claim.SourceActivityId)
			}
		}
	}
	return activities.PromiseFollowUpsTx(ctx, tx, bases, now)
}

// owedPromise is the promise a card speaks for.
type owedPromise struct {
	kind crmcontracts.ContactMomentMayBeDonePromiseType
	id   crmcontracts.Id
	// holder is the plain card's holder marker, so a Not yet given while a
	// colleague held the task lifts once it becomes the reader's.
	holder string
}

// promiseBehind finds the task or claim a promise card was built from. A card
// from any other rung has none.
func promiseBehind(ctx context.Context, moment crmcontracts.ContactMoment, page *crmcontracts.Contact360) (owedPromise, bool) {
	if moment.ClaimKey == overdueTaskKey || moment.ClaimKey == openTaskKey {
		if page.NextSteps == nil || len(moment.Evidence) == 0 || moment.Evidence[0].Id == nil {
			return owedPromise{}, false
		}
		for _, task := range page.NextSteps.Data {
			if task.Id == *moment.Evidence[0].Id {
				return owedPromise{kind: crmcontracts.ContactMomentMayBeDonePromiseTypeTask, id: task.Id, holder: heldMarker(ctx, task)}, true
			}
		}
		return owedPromise{}, false
	}
	if page.Claims == nil {
		return owedPromise{}, false
	}
	for _, claim := range *page.Claims {
		if moment.ClaimKey == claimMomentKey(overdueClaimRung, claim) || moment.ClaimKey == claimMomentKey(openClaimRung, claim) {
			return owedPromise{kind: crmcontracts.ContactMomentMayBeDonePromiseTypeClaim, id: claim.Id}, true
		}
	}
	return owedPromise{}, false
}

// mayBeDone turns a promise card into the question whether our later mail
// kept it.
func (said momentCopy) mayBeDone(ctx context.Context, moment crmcontracts.ContactMoment, page *crmcontracts.Contact360, followUps map[ids.UUID]activities.PromiseFollowUp) (crmcontracts.ContactMoment, bool) {
	promise, ok := promiseBehind(ctx, moment, page)
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	sent, ok := followUps[ids.UUID(promise.id)]
	if !ok {
		return crmcontracts.ContactMoment{}, false
	}
	q := momentaction.DoneQuestion{Plain: moment, Kind: promise.kind, PromiseID: promise.id, Sent: sent, Key: momentaction.DoneKey(promise.id)}
	evidence := momentaction.DoneEvidence(said.Spoken, q)
	return momentaction.AskDone(said.Spoken, said.zone, q, evidence, momentaction.DoneFingerprint(promise.id, sent, promise.holder)), true
}

// proposer asks the may-be-done question in place of a promise card. After the
// reader answers Not yet to it, the plain card comes back.
func (said momentCopy) proposer(
	ctx context.Context, page *crmcontracts.Contact360, followUps map[ids.UUID]activities.PromiseFollowUp, dismissed func(crmcontracts.ContactMoment) bool,
) func(crmcontracts.ContactMoment) crmcontracts.ContactMoment {
	return func(moment crmcontracts.ContactMoment) crmcontracts.ContactMoment {
		if question, ok := said.mayBeDone(ctx, moment, page, followUps); ok && !dismissed(question) {
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
