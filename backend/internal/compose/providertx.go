// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// recordUpdaterTx is the optional capability of a system of record whose
// updates can join a caller's Postgres transaction. A caller probes for it with
// a type assertion and keeps plain Update when the provider lacks it.
//
// It is declared here, not as a V2 interface on the datasource port. The shared
// layer may not import pgx, and an adapter over another system has no
// Postgres transaction to join.
type recordUpdaterTx interface {
	// PrepareUpdateTx does the reads that take their own connection, and
	// answers the write that runs on the caller's transaction.
	PrepareUpdateTx(ctx context.Context, t datasource.EntityType) (updateTx, error)
}

// updateTx is one prepared update, run on the caller's transaction.
type updateTx = func(ctx context.Context, tx pgx.Tx, in datasource.UpdateInput) (datasource.EntityRef, error)

var _ recordUpdaterTx = (*Provider)(nil)

// PrepareUpdateTx is recordUpdaterTx's, routed as Update routes. It serves the
// record types that carry an owner, and refuses the rest as Update refuses an
// unknown type.
func (p *Provider) PrepareUpdateTx(ctx context.Context, t datasource.EntityType) (updateTx, error) {
	switch t {
	case datasource.EntityContact, datasource.EntityCompany, datasource.EntityLead:
		return p.contacts.PrepareUpdateTx(ctx, t)
	case datasource.EntityDeal:
		return p.deals.PrepareUpdateTx(ctx, t)
	case datasource.EntityProject:
		return p.projects.PrepareUpdateTx(ctx, t)
	default:
		return nil, &datasource.UnsupportedEntityError{Type: string(t)}
	}
}
