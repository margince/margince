// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EmitCompanyErased stages company.erased in the transaction that erases the
// company. Its payload is empty: the subscriber learns the id and nothing
// about the company, so the event cannot carry what the erasure removed.
func EmitCompanyErased(ctx context.Context, tx pgx.Tx, auditID ids.UUID, id ids.CompanyID) error {
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventCompanyErased{})
}
