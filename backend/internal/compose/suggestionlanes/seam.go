// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package suggestionlanes binds the Worklist's deal and tag suggestion lanes,
// and their scroll tails, to the stores that own the suggestions.
package suggestionlanes

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// Deals binds the needs_you lane's suggestion source to the
// deals store, whose list and count carry the suggestion visibility rule. The
// rows are shaped by the same wire mapping GET /deal-suggestions answers with.
//
// Both suggestion sources reuse their generic plans (database.ReuseGenericPlans).
// Their statements bind only the visibility rule's values, owners and a limit.
// The open state is a literal, so one plan serves every reader.
type Deals struct{ Store *deals.Store }

func (a Deals) OpenSuggestions(
	ctx context.Context, owners *storekit.OwnerSet, limit int,
) ([]crmcontracts.DealSuggestion, error) {
	var list []deals.Suggestion
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		list, _, err = a.Store.ListSuggestions(ctx, deals.SuggestionQuery{ScopeOwners: owners, Limit: limit})
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.DealSuggestion, 0, len(list))
	for _, suggestion := range list {
		out = append(out, deals.SuggestionWire(suggestion))
	}
	return out, nil
}

// SuggestionsAfter continues the lane's read past its depth through the list's
// own keyset, under the same owners and visibility rule.
func (a Deals) SuggestionsAfter(
	ctx context.Context, owners *storekit.OwnerSet, after string, limit int,
) ([]attention.Positioned[crmcontracts.DealSuggestion], bool, error) {
	var list []deals.Suggestion
	var next string
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		list, next, err = a.Store.ListSuggestions(ctx, deals.SuggestionQuery{ScopeOwners: owners, Cursor: after, Limit: limit})
		return err
	})
	if err != nil {
		return nil, false, err
	}
	out := make([]attention.Positioned[crmcontracts.DealSuggestion], len(list))
	for i, suggestion := range list {
		position, err := deals.SuggestionPositionAfter(suggestion.CreatedAt, suggestion.ID)
		if err != nil {
			return nil, false, err
		}
		out[i] = attention.Positioned[crmcontracts.DealSuggestion]{Row: deals.SuggestionWire(suggestion), After: position}
	}
	return out, next != "", nil
}

func (a Deals) CountOpen(ctx context.Context, owners *storekit.OwnerSet) (int, error) {
	var open int
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		open, err = a.Store.CountOpenSuggestions(ctx, deals.SuggestionQuery{ScopeOwners: owners})
		return err
	})
	return open, err
}

// Tags binds the needs_you lane's tag suggestions to the
// collections store that owns them.
type Tags struct{ Store *collections.Store }

func (a Tags) OpenTagSuggestions(ctx context.Context, limit int) ([]crmcontracts.TagSuggestion, error) {
	var list []collections.TagSuggestion
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		list, err = a.Store.OpenTagSuggestions(ctx, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.TagSuggestion, 0, len(list))
	for _, suggestion := range list {
		out = append(out, collections.WireTagSuggestion(suggestion))
	}
	return out, nil
}

// TagSuggestionsAfter continues the lane's read past its depth through the
// store's keyset, under the same visibility rule.
func (a Tags) TagSuggestionsAfter(
	ctx context.Context, after string, limit int,
) ([]attention.Positioned[crmcontracts.TagSuggestion], bool, error) {
	var list []collections.TagSuggestion
	var more bool
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		list, more, err = a.Store.OpenTagSuggestionsAfter(ctx, after, limit)
		return err
	})
	if err != nil {
		return nil, false, err
	}
	out := make([]attention.Positioned[crmcontracts.TagSuggestion], len(list))
	for i, suggestion := range list {
		position, err := collections.TagSuggestionPositionAfter(suggestion.CreatedAt, suggestion.ID)
		if err != nil {
			return nil, false, err
		}
		out[i] = attention.Positioned[crmcontracts.TagSuggestion]{Row: collections.WireTagSuggestion(suggestion), After: position}
	}
	return out, more, nil
}

func (a Tags) CountOpenTagSuggestions(ctx context.Context) (int, error) {
	var open int
	err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
		var err error
		open, err = a.Store.CountOpenTagSuggestions(ctx)
		return err
	})
	return open, err
}
