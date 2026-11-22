// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// PrepareUpdateTx is Update split around a caller's transaction. The grant is
// asked now. The returned write reads the catalog on the caller's transaction
// and runs UpdateDealTx there.
//
// It serves deals, the one type here that carries an owner. Others answer
// UnsupportedEntityError, as Update does for a type it does not serve.
func (p *Provider) PrepareUpdateTx(ctx context.Context, t datasource.EntityType) (
	func(context.Context, pgx.Tx, datasource.UpdateInput) (datasource.EntityRef, error), error,
) {
	if t != datasource.EntityDeal {
		return nil, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return nil, err
	}
	return func(ctx context.Context, tx pgx.Tx, in datasource.UpdateInput) (datasource.EntityRef, error) {
		if in.Ref.Type != datasource.EntityDeal {
			return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
		}
		raw, err := datasource.RawFields(in.Patch)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		update, err := dealUpdateFrom(raw, in)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		active, err := p.store.activeColumnsIn(ctx, tx)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.UpdateDealTx(ctx, tx, ids.From[ids.DealKind](in.Ref.ID), update, CustomColumns{cols: active})
		return ref(datasource.EntityDeal, v.Id), err
	}, nil
}

// dealUpdateFrom decodes one deal patch, for Update and its Tx twin. It
// refuses the closing fields, which only a stage move may set.
func dealUpdateFrom(raw json.RawMessage, in datasource.UpdateInput) (UpdateDealInput, error) {
	var req crmcontracts.UpdateDealRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateDealInput{}, err
	}
	if err := refuseClosingFields(req); err != nil {
		return UpdateDealInput{}, err
	}
	update := dealUpdateInput(req, in.IfVersion)
	update.Trail = in.Trail
	update.Clear = in.Clear
	return update, nil
}
