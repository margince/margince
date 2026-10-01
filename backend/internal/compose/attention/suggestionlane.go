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

// openSuggestionItems renders up to depth suggestions and answers their total.
//
// A reader who may not read suggestions is shown none and no total, rather than
// losing the whole lane: the approvals and pairs beside them are theirs to
// answer whatever they may read about deals. A read that FAILS is shown the
// same way and named as a failed source, because the suggestion visibility
// clause is the heaviest statement on the page and a stall in it must cost the
// reader their suggestions, not their day.
func (s *Service) openSuggestionItems(
	ctx context.Context, depth int,
) ([]crmcontracts.AttentionItem, *int, *crmcontracts.WorklistSourceUnavailable) {
	if s.suggestions == nil {
		return nil, nil, nil
	}
	var list []crmcontracts.DealSuggestion
	var open int
	err := s.degradable(ctx, func(ctx context.Context) error {
		var err error
		if list, err = s.suggestions.OpenSuggestions(ctx, depth); err != nil {
			return err
		}
		open, err = s.suggestions.CountOpen(ctx)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return nil, nil, nil
	case err != nil:
		slog.ErrorContext(ctx, "the deal-suggestion read failed", "error", err)
		return nil, nil, &crmcontracts.WorklistSourceUnavailable{
			Source: sourceDealSuggestion, Reason: crmcontracts.WorklistSourceUnavailableReasonFailed,
		}
	}
	items := make([]crmcontracts.AttentionItem, 0, len(list))
	for _, suggestion := range list {
		items = append(items, suggestionItem(suggestion))
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
