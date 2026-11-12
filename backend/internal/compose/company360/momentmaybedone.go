// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The account card asks "Done?" over a promise read out of a message when we
// wrote to its recipient on that thread afterwards. Only a claim is asked about
// on the card; a promise task asks in its next-steps row (askTasksDone).
//
// momentaction decides which claim is asked, for this card and the contact
// page alike. Both keep its Not yet in the contact page's store, so an answer
// on one page holds on the other.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// DoneAnswers stores a reader's Not yet to a may-be-done question. The contact
// page's service is the one store; WithDoneAnswers binds it.
type DoneAnswers interface {
	DismissMoment(ctx context.Context, contactID ids.ContactID, in crmcontracts.DismissContactMomentRequest) error
	RestoreMoment(ctx context.Context, contactID ids.ContactID, in crmcontracts.DismissContactMomentRequest) error
}

// WithDoneAnswers binds where the card's Not yet is stored.
func (s *Service) WithDoneAnswers(answers DoneAnswers) *Service {
	s.doneAnswers = answers
	return s
}

// errNoDoneAnswers is a Not yet on a service built without WithDoneAnswers.
var errNoDoneAnswers = errors.New("company360: no store is bound for a Not yet to Done?")

// doneAsk is the question one claim raises and the contact it was made to,
// whose page keeps the answer.
type doneAsk struct {
	moment  crmcontracts.ContactMoment
	contact ids.ContactID
}

// answer is the Not yet the question takes.
func (q doneAsk) answer() crmcontracts.DismissContactMomentRequest {
	return crmcontracts.DismissContactMomentRequest{ClaimKey: q.moment.ClaimKey, EvidenceFingerprint: q.moment.EvidenceFingerprint}
}

// doneQuestions is, per claim on the card, the question its later mail raises.
func doneQuestions(ctx context.Context, tx pgx.Tx, now time.Time, said momentCopy, items []owedwork.Item) (map[string]doneAsk, error) {
	var promises []momentaction.DonePromise
	byClaim := map[ids.UUID]owedwork.Item{}
	for _, item := range items {
		claim, ok := item.Ref.(contacts.CompanyCommitment)
		if !ok {
			continue
		}
		// The account read leaves out a claim that became a task.
		if promise, ok := momentaction.ClaimDone(claim.ID, claim.ActivityID, claim.ContactID.UUID, nil); ok {
			promises, byClaim[claim.ID] = append(promises, promise), item
		}
	}
	followUps, err := momentaction.DoneFollowUpsTx(ctx, tx, promises, now)
	if err != nil || len(followUps) == 0 {
		return nil, err
	}
	// The zone only dates a question that exists, so a card with none reads it
	// not at all.
	loc, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	asked := map[string]doneAsk{}
	for _, promise := range promises {
		item := byClaim[promise.ID]
		plain := said.promiseCard(now, item, deadline.Passed(item.DueAt, now))
		if moment, ok := momentaction.AskPromiseDone(said.Spoken, loc, plain, promise, followUps); ok {
			asked[plain.ClaimKey] = doneAsk{moment: moment, contact: ids.From[ids.ContactKind](promise.Contact)}
		}
	}
	return asked, nil
}

// askDone swaps the card for its question unless this reader answered Not
// yet to it on either page.
func askDone(ctx context.Context, tx pgx.Tx, now time.Time, said momentCopy, items []owedwork.Item, moment crmcontracts.ContactMoment) (crmcontracts.ContactMoment, error) {
	asked, err := doneQuestions(ctx, tx, now, said, items)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return moment, nil
	}
	if err != nil {
		return moment, err
	}
	question, ok := asked[moment.ClaimKey]
	if !ok {
		return moment, nil
	}
	setAside, err := momentaction.DoneSetAsidesTx(ctx, tx, []string{question.moment.ClaimKey})
	if err != nil {
		return moment, err
	}
	if setAside[momentaction.DoneSetAside{Contact: question.contact.UUID, Key: question.moment.ClaimKey, Fingerprint: question.moment.EvidenceFingerprint}] {
		return moment, nil
	}
	return question.moment, nil
}

// doneQuestionWith is the question on this account whose fingerprint the
// reader answered, when one is asked.
func (s *Service) doneQuestionWith(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, within *ids.ProjectID, fingerprint string,
) (doneAsk, bool, error) {
	items, _, err := s.openPromises(ctx, tx, companyID, now, AssembleOptions{ProjectID: within})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return doneAsk{}, false, nil
	}
	if err != nil {
		return doneAsk{}, false, err
	}
	asked, err := doneQuestions(ctx, tx, now, momentCopyFor("en"), items)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return doneAsk{}, false, nil
	}
	if err != nil {
		return doneAsk{}, false, err
	}
	for _, question := range asked {
		if question.moment.EvidenceFingerprint == fingerprint {
			return question, true, nil
		}
	}
	return doneAsk{}, false, nil
}

// answerDone stores or clears a Not yet where the contact page keeps it.
func (s *Service) answerDone(ctx context.Context, question doneAsk, notYet bool) error {
	if s.doneAnswers == nil {
		return errNoDoneAnswers
	}
	if notYet {
		return s.doneAnswers.DismissMoment(ctx, question.contact, question.answer())
	}
	return s.doneAnswers.RestoreMoment(ctx, question.contact, question.answer())
}
