// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The deal and tag suggestions below the ranked rows. The decision lane reads
// each to the census depth. The tail continues each store's own newest-first
// keyset from the last suggestion the lane kept, so scrolling reaches every one
// its store counts.

import (
	"context"
	"errors"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealSuggestionBacklog continues the deal suggestions past the census depth.
// OPTIONAL: unbound, scrolling ends at the ranked rows for them.
type DealSuggestionBacklog interface {
	// SuggestionsAfter reads up to limit open suggestions after a position,
	// under the lane's owners and visibility rule, each with the position
	// after it. more says suggestions lie past the batch.
	SuggestionsAfter(ctx context.Context, owners *storekit.OwnerSet, after string, limit int) (
		read []Positioned[crmcontracts.DealSuggestion], more bool, err error)
}

// TagSuggestionBacklog continues the tag suggestions past the census depth.
// OPTIONAL: unbound, scrolling ends at the ranked rows for them.
type TagSuggestionBacklog interface {
	// TagSuggestionsAfter reads up to limit open tag suggestions after a
	// position, under the lane's visibility rule, each with the position after
	// it. more says suggestions lie past the batch.
	TagSuggestionsAfter(ctx context.Context, after string, limit int) (
		read []Positioned[crmcontracts.TagSuggestion], more bool, err error)
}

// WithDealSuggestionBacklog binds the deal suggestion continuation, an option
// for the reason WithWaiting is one.
func (s *Service) WithDealSuggestionBacklog(b DealSuggestionBacklog) *Service {
	s.backlog.dealSuggestions = b
	return s
}

// WithTagSuggestionBacklog binds the tag suggestion continuation, an option
// for the reason WithWaiting is one.
func (s *Service) WithTagSuggestionBacklog(b TagSuggestionBacklog) *Service {
	s.backlog.tagSuggestions = b
	return s
}

// decisionTailStarts is where each decision source the tail continues resumes:
// after the last row of it the lane kept. interleave keeps a prefix of each
// producer, so that row is the producer's last in the lane.
func decisionTailStarts(
	lane []crmcontracts.AttentionItem, staged []crmcontracts.Approval,
) (map[crmcontracts.WorklistItemSource]string, error) {
	approvalsFrom, err := approvalTailStart(lane, staged)
	if err != nil {
		return nil, err
	}
	at := map[crmcontracts.WorklistItemSource]string{crmcontracts.WorklistItemSourceApproval: approvalsFrom}
	for source, position := range map[string]func(time.Time, ids.UUID) (string, error){
		sourceDealSuggestion: deals.SuggestionPositionAfter,
		sourceTagSuggestion:  collections.TagSuggestionPositionAfter,
	} {
		from, err := suggestionTailStart(lane, source, position)
		if err != nil {
			return nil, err
		}
		at[crmcontracts.WorklistItemSource(source)] = from
	}
	return at, nil
}

// suggestionTailStart is the store position after the source's last row in the
// lane, which its rendering keeps as the row's id and occurred time. Empty when
// the lane kept none.
func suggestionTailStart(
	lane []crmcontracts.AttentionItem, source string, position func(time.Time, ids.UUID) (string, error),
) (string, error) {
	var last *crmcontracts.AttentionItem
	for i := range lane {
		if string(lane[i].Source) == source {
			last = &lane[i]
		}
	}
	if last == nil {
		return "", nil
	}
	id, err := ids.Parse(last.Id)
	if err != nil {
		return "", fmt.Errorf("attention: a %s row carries no suggestion id: %w", source, err)
	}
	if last.OccurredAt == nil {
		return "", fmt.Errorf("attention: a %s row carries no creation time", source)
	}
	return position(*last.OccurredAt, id)
}

// moreDealSuggestions reads the next deal suggestions under the lane's owners.
// Its store orders them newest first, so they are undated in the tail's order
// and follow the dated sources. A reader who lost the suggestion grant mid-walk
// reaches the source's end, as the lane shows such a reader none.
func (s *Service) moreDealSuggestions(ctx context.Context, _ tailPage, walk tailWindow, after string, limit int) (tailBatch, error) {
	owners, err := s.suggestionScope().recordOwners(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return tailBatch{}, nil
	}
	if err != nil {
		return tailBatch{}, err
	}
	read, more, err := s.backlog.dealSuggestions.SuggestionsAfter(ctx, owners, after, limit)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return tailBatch{}, nil
	}
	if err != nil {
		return tailBatch{}, err
	}
	items := make([]crmcontracts.AttentionItem, len(read))
	positions := make([]string, len(read))
	for i, suggestion := range read {
		items[i], positions[i] = suggestionItem(suggestion.Row), suggestion.After
	}
	return s.suggestionBatch(ctx, walk, items, positions, more)
}

// moreTagSuggestions reads the next tag suggestions, in the scopes whose lane
// reads them, ordered and ended as moreDealSuggestions is.
func (s *Service) moreTagSuggestions(ctx context.Context, _ tailPage, walk tailWindow, after string, limit int) (tailBatch, error) {
	if !s.unassignedDecisions() {
		return tailBatch{}, nil
	}
	read, more, err := s.backlog.tagSuggestions.TagSuggestionsAfter(ctx, after, limit)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return tailBatch{}, nil
	}
	if err != nil {
		return tailBatch{}, err
	}
	items := make([]crmcontracts.AttentionItem, len(read))
	positions := make([]string, len(read))
	for i, suggestion := range read {
		items[i], positions[i] = tagSuggestionItem(suggestion.Row), suggestion.After
	}
	return s.suggestionBatch(ctx, walk, items, positions, more)
}

// suggestionBatch draws suggestion items as the decision lane draws them:
// named under the reader's grants and classified as decisions.
func (s *Service) suggestionBatch(
	ctx context.Context, walk tailWindow, items []crmcontracts.AttentionItem, positions []string, more bool,
) (tailBatch, error) {
	subjects := make([]*crmcontracts.AttentionSubject, len(items))
	for i := range items {
		subjects[i] = items[i].Subject
	}
	if err := s.labelSubjects(ctx, subjects); err != nil {
		return tailBatch{}, err
	}
	// appendLane draws one row per suggestion, in order, under the lane that
	// carries suggestions in the ranked rows.
	rows := appendLane(nil, crmcontracts.WorklistItemSourceApproval, &items, walk.AsOf, classifyDecision)
	batch := tailBatch{more: more, rows: make([]tailRow, len(rows))}
	for i := range rows {
		batch.rows[i] = tailRow{row: rows[i], after: positions[i]}
	}
	return batch, nil
}
