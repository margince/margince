// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

// The account card asks "Done?" over a promise read out of a message when we
// wrote to its recipient on that thread afterwards. A task row on this page
// names no source message, so only a claim can be asked about here.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// doneQuestions is, per claim on the card, the question its later mail raises.
func doneQuestions(ctx context.Context, tx pgx.Tx, now time.Time, said momentCopy, items []owedwork.Item) (map[string]crmcontracts.ContactMoment, error) {
	var bases []activities.PromiseBasis
	claims := map[ids.UUID]owedwork.Item{}
	for _, item := range items {
		if claim, ok := item.Ref.(contacts.CompanyCommitment); ok {
			claims[claim.ID] = item
			bases = append(bases, activities.PromiseBasis{PromiseID: claim.ID, SourceActivityID: claim.ActivityID, RecipientContactID: claim.ContactID.UUID})
		}
	}
	followUps, err := activities.PromiseFollowUpsTx(ctx, tx, bases, now)
	if err != nil || len(followUps) == 0 {
		return nil, err
	}
	// The zone only dates a question that exists, so a card with none reads it
	// not at all.
	loc, err := identity.ZoneAppliedTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	asked := map[string]crmcontracts.ContactMoment{}
	for id, sent := range followUps {
		item := claims[id]
		plain := said.promiseCard(now, item, deadline.Passed(item.DueAt, now))
		q := momentaction.DoneQuestion{
			Plain: plain, Kind: crmcontracts.ContactMomentMayBeDonePromiseTypeClaim, PromiseID: openapi_types.UUID(id), Sent: sent,
			Key: momentKey("moment:account_may_be_done", openapi_types.UUID(id)),
		}
		evidence := momentaction.DoneEvidence(said.Spoken, q)
		fingerprint := accountFingerprint(evidence, owedwork.Mark{Kind: "claim", ID: id.String(), At: item.DueAt})
		asked[plain.ClaimKey] = momentaction.AskDone(said.Spoken, loc, q, evidence, fingerprint)
	}
	return asked, nil
}

// askDone swaps the card for its question unless this reader answered Not
// yet to it. A caller with no screen of their own has no answer to honour.
func (s *Service) askDone(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, now time.Time, said momentCopy, items []owedwork.Item, moment crmcontracts.ContactMoment) (crmcontracts.ContactMoment, error) {
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
	if auth.RequireHuman(ctx) == nil {
		dismissed, err := s.dismissedFingerprints(ctx, tx, companyID, []string{question.EvidenceFingerprint})
		if err != nil {
			return moment, err
		}
		if dismissed[question.EvidenceFingerprint] {
			return moment, nil
		}
	}
	return question, nil
}
