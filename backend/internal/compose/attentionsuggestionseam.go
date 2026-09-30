// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// attentionDealSuggestions binds the needs_you lane's suggestion source to the
// deals store, whose list and count carry the suggestion visibility rule. The
// rows are shaped by the same wire mapping GET /deal-suggestions answers with.
type attentionDealSuggestions struct{ store *deals.Store }

func (a attentionDealSuggestions) OpenSuggestions(ctx context.Context, limit int) ([]crmcontracts.DealSuggestion, error) {
	list, _, err := a.store.ListSuggestions(ctx, deals.SuggestionQuery{Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.DealSuggestion, 0, len(list))
	for _, suggestion := range list {
		out = append(out, deals.SuggestionWire(suggestion))
	}
	return out, nil
}

func (a attentionDealSuggestions) CountOpen(ctx context.Context) (int, error) {
	return a.store.CountOpenSuggestions(ctx)
}
