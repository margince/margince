// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EmitDealErased stages deal.erased in the transaction that erases the deal.
// Its payload is empty: the subscriber learns the id and nothing about the
// deal, so the event cannot carry what the erasure removed.
func EmitDealErased(ctx context.Context, tx pgx.Tx, auditID ids.UUID, id ids.DealID) error {
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventDealErased{})
}

// EmitDealMerged stages deal.merged, named for the merged-away deal, with the
// surviving deal's id so a subscriber can repoint its own reference.
func EmitDealMerged(ctx context.Context, tx pgx.Tx, auditID ids.UUID, from, into ids.DealID) error {
	return storekit.EmitEvent(ctx, tx, auditID, from.UUID, crmcontracts.PublicEventDealMerged{
		MergedFromId: openapi_types.UUID(from.UUID),
		MergedIntoId: openapi_types.UUID(into.UUID),
	})
}
