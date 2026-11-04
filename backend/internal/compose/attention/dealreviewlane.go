// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// One Home row per open customer-support review. The review is one job, so
// the task it was delivered as and the deal's quiet-only at-risk row fold into
// it. The review stays on Home while it is open, even once its task is done:
// completing the task does not answer it.

import (
	"context"
	"errors"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const sourceDealReview = "deal_review"

// DealReview is one open `deal_viability` review on a deal the reader may read.
type DealReview struct {
	ID       ids.UUID
	DealID   ids.UUID
	DealName string
	// Owner is the deal's owner, nil when nobody owns it.
	Owner *ids.UUID
	// Task is the task the review was delivered as, nil until it was.
	Task     *ids.UUID
	OpenedAt time.Time
}

// reviewRead is the review reader, OPTIONAL, and on a request copy the open
// reviews it answered.
type reviewRead struct {
	source DealReviews
	open   []DealReview
}

// DealReviews reads the open reviews this reader may see.
type DealReviews interface {
	OpenDealReviews(context.Context) ([]DealReview, error)
}

// WithDealReviews connects the open customer-support reviews to the worklist.
func (s *Service) WithDealReviews(reviews DealReviews) *Service {
	s.reviews.source = reviews
	return s
}

// readingReviews carries this read's open reviews on a copy of the service.
func (s *Service) readingReviews(ctx context.Context) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	scoped := *s
	if s.reviews.source == nil {
		return &scoped, nil
	}
	var reviews []DealReview
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		reviews, err = s.reviews.source.OpenDealReviews(ctx)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return &scoped, reviewsUnavailable(crmcontracts.WorklistSourceUnavailableReasonWithheld)
	case err != nil:
		return &scoped, reviewsUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
	}
	scoped.reviews.open = reviews
	return &scoped, nil
}

func reviewsUnavailable(reason crmcontracts.WorklistSourceUnavailableReason) *crmcontracts.WorklistSourceUnavailable {
	return &crmcontracts.WorklistSourceUnavailable{Source: sourceDealReview, Reason: reason}
}

// foldDealReviews makes each open review one row. The review's own task row and
// its deal's quiet-only at-risk row leave their lanes and become that row. The
// reader sees one job, not three. A review whose task is done, or not yet
// delivered, still gets its row.
func foldDealReviews(rows []ranked, reviews []DealReview) []ranked {
	if len(reviews) == 0 {
		return rows
	}
	byTask, byDeal := map[string]int{}, map[string]int{}
	for i, review := range reviews {
		byDeal[review.DealID.String()] = i
		if review.Task != nil {
			byTask[review.Task.String()] = i
		}
	}
	fromRisk, fromTask := map[int]ranked{}, map[int]ranked{}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if i, ok := byTask[row.item.Id]; ok && row.item.Source == sourceTask {
			fromTask[i] = row
			continue
		}
		if i, ok := byDeal[row.item.Id]; ok && row.item.Source == sourceAtRisk && quietOnly(row) {
			fromRisk[i] = row
			continue
		}
		kept = append(kept, row)
	}
	for i, review := range reviews {
		kept = append(kept, reviewRow(review, fromRisk, fromTask, i))
	}
	return kept
}

// quietOnly is an at-risk row admitted by silence alone, which is what the
// review already says. A deal past its close date keeps its own row.
func quietOnly(row ranked) bool {
	return row.item.Kind != nil && *row.item.Kind == "quiet"
}

// reviewRow is the review's row. It keeps what the folded rows knew: the
// at-risk row's deal facts, money and ranking, else the task's deadline and
// assignee. Its id is the review's, and it offers only `open`, because the
// folded task's own verbs write to the task and not to the review.
func reviewRow(review DealReview, fromRisk, fromTask map[int]ranked, i int) ranked {
	row, folded := fromRisk[i]
	task, hasTask := fromTask[i]
	switch {
	case folded:
		row.item.Because = append(row.item.Because, task.item.Because...)
	case hasTask:
		row = task
	default:
		row = ranked{item: crmcontracts.WorklistItem{
			Level: levelAgreed, Because: []crmcontracts.WorklistReason{},
		}, ownerRef: unassigned(), occurredAt: review.OpenedAt}
		if review.Owner != nil {
			row.ownerRef = ownedBy(*review.Owner)
		}
	}
	name := review.DealName
	row.item.Id = review.ID.String()
	row.item.Source = sourceDealReview
	row.item.Category = categoryOfSource(sourceDealReview)
	row.item.Consequence = "deal_drifts"
	row.item.Title = &name
	row.item.Subject = subjectOf(subjectDeal, review.DealID)
	row.item.Actions = []crmcontracts.WorklistItemActions{crmcontracts.WorklistItemActionsOpen}
	row.lane = sourceDealReview
	return row
}
