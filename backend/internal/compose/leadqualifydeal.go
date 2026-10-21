// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadDealOpener is the contacts→deals edge behind qualify-to-deal: the
// promote transaction asks for a deal, and the deals store writes it inside
// that same transaction (CreateDealTx), so the contact and its opportunity
// land together or not at all. Everything runs on the caller's transaction —
// the promote path holds a lead row lock and one pooled connection, and a
// second connection taken while waiting is how a pool deadlocks itself.
type leadDealOpener struct{ deals *deals.Store }

// OpenDealForLead resolves the pipeline and stage — the caller's choice, a
// named stage's own pipeline, or the default pipeline's first open stage —
// and opens the deal.
func (o leadDealOpener) OpenDealForLead(ctx context.Context, tx pgx.Tx, in contacts.QualifyDealInput) (ids.UUID, error) {
	create, err := qualifiedDealCreate(ctx, tx, in)
	if err != nil {
		return ids.Nil, err
	}
	deal, err := o.deals.CreateDealTx(ctx, tx, create, deals.CustomColumns{})
	if err != nil {
		return ids.Nil, err
	}
	return ids.UUID(deal.Id), nil
}

func qualifiedDealCreate(ctx context.Context, tx pgx.Tx, in contacts.QualifyDealInput) (deals.CreateDealInput, error) {
	pipelineID, stageID, err := deals.BirthStageTx(ctx, tx, in.PipelineID, in.StageID)
	if err != nil {
		return deals.CreateDealInput{}, err
	}
	return deals.CreateDealInput{
		Name: in.Name, AmountMinor: in.AmountMinor, Currency: in.Currency,
		PipelineID: pipelineID, StageID: stageID, Source: in.Source, OwnerID: in.OwnerID, OwnerExact: true,
	}, nil
}

func (o leadDealOpener) PreviewDealForLead(ctx context.Context, tx pgx.Tx, in contacts.QualifyDealInput) (contacts.QualifyDealInput, error) {
	create, err := qualifiedDealCreate(ctx, tx, in)
	if err != nil {
		return contacts.QualifyDealInput{}, err
	}
	ready, err := o.deals.PreviewCreateDealTx(ctx, tx, create)
	if err != nil {
		return contacts.QualifyDealInput{}, err
	}
	return contacts.QualifyDealInput{
		Name: ready.Name, AmountMinor: ready.AmountMinor, Currency: ready.Currency,
		PipelineID: new(ready.PipelineID.UUID), StageID: new(ready.StageID.UUID), OwnerID: ready.OwnerID, Source: ready.Source,
	}, nil
}
