// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// RemoveCustomChoices clears choices an admin removed from an activity field
// off every activity that holds one. Each activity announces activity.updated
// with no typed field, as an edit of a custom field alone does.
func RemoveCustomChoices(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error) {
	return storekit.ClearRemovedChoices(ctx, tx, linkEntityActivity, column, removed, func(map[string]any) events.Payload {
		return crmcontracts.PublicEventActivityUpdated{}
	})
}
