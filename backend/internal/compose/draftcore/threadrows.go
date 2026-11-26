// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"context"
	"fmt"
	"slices"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ThreadRows is the thread a reply answers, newest first, one row per mail
// however many mailboxes captured it. The anchor and every mailbox copy of it
// are left out, as the reply carries the anchor already. When the thread is
// longer than a page, the addressee's newest mail follows it. Their own words
// then still decide the language of a long thread. limit is the first page's
// size.
func ThreadRows(ctx context.Context, store *activities.Store, anchor crmcontracts.Activity,
	delivery activities.ReplyAddressee, now time.Time, limit int,
) ([]crmcontracts.Activity, error) {
	kind, anchorID := string(crmcontracts.ActivityKindEmail), ids.UUID(anchor.Id)
	// A thread key is read from headers its sender writes, so it can name another
	// correspondence; ConversationOf keeps only rows shared with the anchor's correspondent.
	in := activities.ListActivitiesInput{
		ThreadKey: anchor.ThreadKey, ConversationOf: &anchorID, ConversationDeliveredTo: &delivery.Address,
		Kind: &kind, Limit: &limit, WrittenByHuman: true, ConversationAsOf: &now,
		CollapseSeatCopies: true, WithoutCopiesOf: &anchorID,
	}
	rows, page, err := store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	if page.HasMore && delivery.ContactID != "" {
		sender, err := ids.Parse(delivery.ContactID)
		if err != nil {
			return nil, fmt.Errorf("draftcore: reading the reply's addressee id: %w", err)
		}
		own := LanguageEvidenceTurns
		in.Limit, in.SentBy = &own, &sender
		older, _, err := store.ListActivities(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, row := range older {
			if !slices.ContainsFunc(rows, func(seen crmcontracts.Activity) bool { return seen.Id == row.Id }) {
				rows = append(rows, row)
			}
		}
	}
	return rows, nil
}
