// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package offernetdeals holds the data task that re-prices deals which took the
// gross of an accepted offer. A deal's value is the figure without tax.
package offernetdeals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// DataTask re-prices the deals that took the gross of an accepted offer to its
// net, and tells each deal's owner in the same transaction.
func DataTask(store *deals.Store, noticeStore *notices.Store) datatask.Task {
	return datatask.Task{
		ID: "core/1795589912_offer_net_deal_values", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795589912_offer_net_deal_values"),
		Steps: []datatask.Step{{
			Name: "reprice_to_offer_net", Limit: 100, MinLimit: 5,
			Audit:         datatask.AuditPolicy{PerRecord: true},
			EmptyWhenDone: true,
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				count, err := deals.EstimateGrossPricedDeals(ctx, q, maximum)
				return min(count, maximum), count > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				page, err := store.RepriceFromAcceptedOfferNetTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				for _, corrected := range page.Corrected {
					if err := tellOwnerOfNetValue(ctx, tx, noticeStore, corrected); err != nil {
						return datatask.BatchOut{}, err
					}
				}
				return datatask.AdvanceUUID(in, page.Scanned, len(page.Corrected))
			},
		}},
	}
}

// tellOwnerOfNetValue writes the owner's notice. A deal with no owner, or whose
// owner can no longer be handed work, gets none: automatic work has no fallback
// owner.
func tellOwnerOfNetValue(ctx context.Context, tx pgx.Tx, store *notices.Store, c deals.NetCorrection) error {
	if c.Owner == nil {
		return nil
	}
	err := auth.EnsureAssignee(ctx, tx, *c.Owner)
	if errors.As(err, new(*auth.AssigneeNotAllowedError)) {
		return nil
	}
	if err != nil {
		return err
	}
	was := values.MajorUnits(c.FromMinor, c.Currency) + " " + c.Currency
	now := values.MajorUnits(c.ToMinor, c.Currency) + " " + c.Currency
	message := &systemtext.Message{
		Subject: systemtext.Sentence("notice.deal_net_value.subject"),
		Body:    systemtext.Sentence("notice.deal_net_value.body", systemtext.Text(c.Name), systemtext.Text(was), systemtext.Text(now)),
	}
	subject, err := message.Subject.Render(textlang.English)
	if err != nil {
		return err
	}
	body, err := message.Body.Render(textlang.English)
	if err != nil {
		return err
	}
	if _, err := store.CreateTx(ctx, tx, notices.NewNotice{
		Recipient: ids.From[ids.UserKind](*c.Owner), Kind: notices.KindDealValueNetBasis,
		Subject: subject, Body: body, SystemText: message,
		DedupeKey: "deal-net-value:" + c.Deal.String(),
		Target:    notices.Target{Type: "deal", ID: c.Deal.UUID},
	}); err != nil {
		return fmt.Errorf("telling the owner of deal %s its value now leaves out tax: %w", c.Deal, err)
	}
	return nil
}
