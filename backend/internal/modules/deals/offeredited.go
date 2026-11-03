// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// offerLineItemsField is what a line edit names among the fields it changed.
const offerLineItemsField = "line_items"

// announceOfferEdit stages offer.updated for a draft edit, in the transaction of
// the audit row it follows. Every draft edit announces itself this way, header
// and lines alike, so a subscriber hears each change the audit trail records.
func announceOfferEdit(ctx context.Context, tx pgx.Tx, auditID ids.UUID, offer crmcontracts.Offer, changed []string) error {
	slices.Sort(changed)
	return storekit.EmitEvent(ctx, tx, auditID, ids.UUID(offer.Id), crmcontracts.PublicEventOfferUpdated{
		OfferId: offer.Id, DealId: offer.DealId, ChangedFields: changed,
	})
}

// lineEditFields names what a line edit moved: the lines, and each total that
// changed with them.
func lineEditFields(before, after OfferFigures) []string {
	changed := []string{offerLineItemsField}
	for column, moved := range map[string]bool{
		offerKeyNet: before.NetMinor != after.NetMinor, offerKeyTax: before.TaxMinor != after.TaxMinor,
		offerKeyGross: before.GrossMinor != after.GrossMinor,
	} {
		if moved {
			changed = append(changed, column)
		}
	}
	return changed
}
