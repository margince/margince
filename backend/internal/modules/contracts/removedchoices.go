// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// RemoveCustomChoices clears choices an admin removed from a contract field off
// every contract that holds one. Each contract announces it as
// contract.updated, as an edit of the field does.
func RemoveCustomChoices(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error) {
	return storekit.ClearRemovedChoices(ctx, tx, contractObject, column, removed, func(changed map[string]any) events.Payload {
		return crmcontracts.PublicEventContractUpdated{ChangedFields: changed}
	})
}
