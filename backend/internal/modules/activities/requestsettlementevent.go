// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// emitSettlementChanged announces a settlement write on the request it judges,
// in the audit row's transaction, as every write carries its event.
func emitSettlementChanged(ctx context.Context, tx pgx.Tx, auditID, request ids.UUID) error {
	return storekit.EmitEvent(ctx, tx, auditID, request, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{RequestSettlementChanged: new(true)},
	})
}
