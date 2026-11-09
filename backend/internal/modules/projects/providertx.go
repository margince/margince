// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// PrepareUpdateTx is Update split around a caller's transaction. The catalog
// read takes its own connection, so it runs now, and the returned write runs
// inside the caller's transaction through UpdateProjectTx.
func (p *Provider) PrepareUpdateTx(ctx context.Context, t datasource.EntityType) (
	func(context.Context, pgx.Tx, datasource.UpdateInput) (datasource.EntityRef, error), error,
) {
	if t != datasource.EntityProject {
		return nil, &datasource.UnsupportedEntityError{Type: string(t)}
	}
	if err := auth.Require(ctx, projectObject, principal.ActionUpdate); err != nil {
		return nil, err
	}
	active, err := p.store.catalogColumns(ctx)
	if err != nil {
		return nil, err
	}
	cols := CustomColumns{cols: active}
	return func(ctx context.Context, tx pgx.Tx, in datasource.UpdateInput) (datasource.EntityRef, error) {
		if in.Ref.Type != datasource.EntityProject {
			return datasource.EntityRef{}, &datasource.UnsupportedEntityError{Type: string(in.Ref.Type)}
		}
		update, err := projectUpdateFrom(in)
		if err != nil {
			return datasource.EntityRef{}, err
		}
		v, err := p.store.UpdateProjectTx(ctx, tx, ids.From[ids.ProjectKind](in.Ref.ID), update, cols)
		return ref(v.Id), err
	}, nil
}

// projectUpdateFrom decodes one project patch, for Update and its Tx twin.
func projectUpdateFrom(in datasource.UpdateInput) (UpdateProjectInput, error) {
	raw, err := datasource.RawFields(in.Patch)
	if err != nil {
		return UpdateProjectInput{}, err
	}
	if err := datasource.ClaimKeys(raw, claimCallerChosenKey); err != nil {
		return UpdateProjectInput{}, err
	}
	var req crmcontracts.UpdateProjectRequest
	if err := datasource.StrictDecode(raw, &req); err != nil {
		return UpdateProjectInput{}, err
	}
	update, err := projectUpdateInput(req, in.IfVersion)
	if err != nil {
		return UpdateProjectInput{}, err
	}
	update.Trail = in.Trail
	update.Clear = in.Clear
	return update, nil
}
