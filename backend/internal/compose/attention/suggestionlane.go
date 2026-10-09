// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Deal Scout's open suggestions, the third source of the needs_you lane beside
// staged approvals and duplicate pairs: a deal the evidence says should exist,
// waiting for a rep to open it or say it is not one.

import (
	"context"
	"errors"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

const sourceDealSuggestion = "deal_suggestion"

// DealSuggestions reads the open suggestions THIS reader may see, newest first,
// and how many there are. Both answer under the deals store's own visibility
// rule, so the count is never of suggestions the page could not show.
type DealSuggestions interface {
	OpenSuggestions(ctx context.Context, limit int) ([]crmcontracts.DealSuggestion, error)
	CountOpen(ctx context.Context) (int, error)
}

// WithDealSuggestions binds the suggestion reader. Unbound, the lane carries
// no suggestions and no count, which is what an installation without the
// scout honestly has.
func (s *Service) WithDealSuggestions(d DealSuggestions) *Service {
	s.suggestions = d
	return s
}

// suggestionLanes reads both suggestion producers and adds their totals and
// failures to count.
func (s *Service) suggestionLanes(ctx context.Context, depth int, count *laneCount) (deal, tag []crmcontracts.AttentionItem) {
	var failed, tagFailed *crmcontracts.WorklistSourceUnavailable
	deal, count.suggestions, failed = s.openSuggestionItems(ctx, depth)
	tag, count.tagSuggestions, tagFailed = s.openTagSuggestionItems(ctx, depth)
	for _, one := range []*crmcontracts.WorklistSourceUnavailable{failed, tagFailed} {
		if one != nil {
			count.failed = append(count.failed, one)
		}
	}
	for _, open := range []*int{count.suggestions, count.tagSuggestions} {
		if open != nil {
			count.items += *open
		}
	}
	return deal, tag
}

// openSuggestionItems renders up to depth deal suggestions and answers their total.
func (s *Service) openSuggestionItems(
	ctx context.Context, depth int,
) ([]crmcontracts.AttentionItem, *int, *crmcontracts.WorklistSourceUnavailable) {
	if s.suggestions == nil {
		return nil, nil, nil
	}
	return readSuggestionLane(ctx, s, sourceDealSuggestion,
		func(ctx context.Context) ([]crmcontracts.DealSuggestion, error) {
			return s.suggestions.OpenSuggestions(ctx, depth)
		}, s.suggestions.CountOpen, suggestionItem)
}

// readSuggestionLane reads one suggestion producer's page and total.
//
// A reader who may not read suggestions is shown none and no total; the rest of
// the lane stays. A failed read is shown the same way and named as a failed
// source. A suggestion visibility clause is among the heaviest statements on the
// page, and a stall in it must cost the reader only their suggestions.
func readSuggestionLane[T any](
	ctx context.Context, s *Service, source string,
	list func(context.Context) ([]T, error), count func(context.Context) (int, error),
	render func(T) crmcontracts.AttentionItem,
) ([]crmcontracts.AttentionItem, *int, *crmcontracts.WorklistSourceUnavailable) {
	var page []T
	var open int
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		if page, err = list(ctx); err != nil {
			return err
		}
		open, err = count(ctx)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return nil, nil, nil
	case err != nil:
		slog.ErrorContext(ctx, "a suggestion read failed", "source", source, "error", err)
		return nil, nil, &crmcontracts.WorklistSourceUnavailable{
			Source: source, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	}
	items := make([]crmcontracts.AttentionItem, 0, len(page))
	for _, one := range page {
		items = append(items, render(one))
	}
	return items, &open, nil
}

// suggestionItem renders one suggestion. Its subject is the company, which is
// where `open` leads; `decide` opens the deal and `dismiss` records that it is
// not one. It carries no title: the client words the row from its source and
// the company's name, so the row repeats no text from the evidence.
func suggestionItem(suggestion crmcontracts.DealSuggestion) crmcontracts.AttentionItem {
	kind := string(suggestion.Kind)
	confidence := suggestion.Confidence
	occurred := suggestion.CreatedAt
	return crmcontracts.AttentionItem{
		Id:         suggestion.Id.String(),
		Source:     crmcontracts.AttentionItemSource(sourceDealSuggestion),
		Kind:       &kind,
		Confidence: &confidence,
		OccurredAt: &occurred,
		Subject:    &crmcontracts.AttentionSubject{Type: subjectCompany, Id: suggestion.CompanyId},
		Actions: []crmcontracts.AttentionItemActions{
			crmcontracts.AttentionItemActionsDecide, actionDismiss, crmcontracts.AttentionItemActionsOpen,
		},
	}
}

// interleave takes one item from each lane in turn, in the order given, up to
// limit, and keeps drawing from whichever lanes still have items once the
// others run dry.
//
// The alternation is what keeps a lane honest when one producer floods: a
// morning's import can raise a hundred duplicate pairs, and the reader still
// meets their staged decisions and suggestions on the first screen.
func interleave(limit int, lanes ...[]crmcontracts.AttentionItem) []crmcontracts.AttentionItem {
	out := make([]crmcontracts.AttentionItem, 0, limit)
	for i := 0; len(out) < limit; i++ {
		drew := false
		for _, lane := range lanes {
			if i < len(lane) && len(out) < limit {
				out = append(out, lane[i])
				drew = true
			}
		}
		if !drew {
			return out
		}
	}
	return out
}
