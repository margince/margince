// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The tag scout's open suggestions sit beside Deal Scout's in the needs_you
// lane. Each proposes a suggestible tag on a contact or company and waits for
// somebody to accept or dismiss it.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

const sourceTagSuggestion = "tag_suggestion"

// TagSuggestions reads the open tag suggestions the reader may see, newest
// first, and how many there are, both under the collections visibility rule.
type TagSuggestions interface {
	OpenTagSuggestions(ctx context.Context, limit int) ([]crmcontracts.TagSuggestion, error)
	CountOpenTagSuggestions(ctx context.Context) (int, error)
}

// WithTagSuggestions binds the tag suggestion reader. Unbound, the lane carries
// none and no count.
func (s *Service) WithTagSuggestions(t TagSuggestions) *Service {
	s.tagSuggestions = t
	return s
}

func (s *Service) openTagSuggestionItems(
	ctx context.Context, depth int,
) ([]crmcontracts.AttentionItem, *int, *crmcontracts.WorklistSourceUnavailable) {
	if s.tagSuggestions == nil {
		return nil, nil, nil
	}
	return readSuggestionLane(ctx, s, sourceTagSuggestion,
		func(ctx context.Context) ([]crmcontracts.TagSuggestion, error) {
			return s.tagSuggestions.OpenTagSuggestions(ctx, depth)
		}, s.tagSuggestions.CountOpenTagSuggestions, tagSuggestionItem)
}

// tagSuggestionItem renders one suggestion. Its subject is the record the tag
// would go on, which is where `open` leads; `decide` accepts it. It carries no
// title: the client words the row, and its card reads the tag and evidence.
func tagSuggestionItem(suggestion crmcontracts.TagSuggestion) crmcontracts.AttentionItem {
	subject := crmcontracts.AttentionSubjectType(subjectContact)
	if suggestion.EntityType == crmcontracts.TagSuggestionEntityTypeCompany {
		subject = crmcontracts.AttentionSubjectType(subjectCompany)
	}
	occurred := suggestion.CreatedAt
	return crmcontracts.AttentionItem{
		Id:         suggestion.Id.String(),
		Source:     crmcontracts.AttentionItemSource(sourceTagSuggestion),
		OccurredAt: &occurred,
		Subject:    &crmcontracts.AttentionSubject{Type: subject, Id: suggestion.EntityId},
		Actions: []crmcontracts.AttentionItemActions{
			crmcontracts.AttentionItemActionsDecide, actionDismiss, crmcontracts.AttentionItemActionsOpen,
		},
	}
}
