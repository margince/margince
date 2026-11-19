// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// RemoveContactChoices, RemoveCompanyChoices and RemoveLeadChoices clear
// choices an admin removed from a field off every record of that object that
// holds one. Each record announces its own updated event, as an edit of the
// field does.
func RemoveContactChoices(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error) {
	return storekit.ClearRemovedChoices(ctx, tx, entityContact, column, removed, func(changed map[string]any) events.Payload {
		return crmcontracts.PublicEventContactUpdated{ChangedFields: changed}
	})
}

func RemoveCompanyChoices(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error) {
	return storekit.ClearRemovedChoices(ctx, tx, entityCompany, column, removed, func(changed map[string]any) events.Payload {
		return crmcontracts.PublicEventCompanyUpdated{ChangedFields: changed}
	})
}

func RemoveLeadChoices(ctx context.Context, tx pgx.Tx, column fieldcatalog.Column, removed []string) (int, error) {
	return storekit.ClearRemovedChoices(ctx, tx, entityLead, column, removed, func(changed map[string]any) events.Payload {
		return crmcontracts.PublicEventLeadUpdated{ChangedFields: changed}
	})
}
