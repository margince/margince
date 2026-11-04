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
	// Owner answers for the review: its task's assignee, done or not, else
	// the deal's owner. Nil when nobody does.
	Owner *ids.UUID
	// Task is the task the review was delivered as, nil until it was, and
	// TaskOpen whether it is still to do.
	Task     *ids.UUID
	TaskOpen bool
	OpenedAt time.Time
}

// reviewRead is the review reader, OPTIONAL, and on a request copy the open
// reviews it answered.
type reviewRead struct {
	source DealReviews
	open   []DealReview
	// cut says the read stopped at its cap, so the source is bounded.
	cut bool
}

// foldsTask says a tail task row is a review's task, which the review row
// reports. The fold above the tail cannot see rows below a lane's cap.
func (r reviewRead) foldsTask(row ranked) bool {
	for _, review := range r.open {
		if review.Task != nil && row.item.Source == sourceTask && row.item.Id == review.Task.String() {
			return true
		}
	}
	return false
}

// holdsOpenTask says a review's task is still open, so a task lane's store
// count includes a task the fold reports elsewhere.
func (r reviewRead) holdsOpenTask() bool {
	for _, review := range r.open {
		if review.Task != nil && review.TaskOpen {
			return true
		}
	}
	return false
}

// foldsRisk says a tail at-risk row is a review deal's quiet-only row.
func (r reviewRead) foldsRisk(row ranked) bool {
	if row.item.Source != sourceAtRisk || !quietOnly(row) {
		return false
	}
	for _, review := range r.open {
		if row.item.Id == review.DealID.String() {
			return true
		}
	}
	return false
}

// ReviewScope narrows the read to whose reviews the page asks for, before its
// cap, so colleagues' older reviews cannot crowd out the reader's own. All
// reads every review the reader may see; else Owners, or Unowned alone.
type ReviewScope struct {
	All     bool
	Owners  []ids.UUID
	Unowned bool
}

// DealReviews reads the open reviews this reader may see under the scope. cut
// says the read stopped at its cap.
type DealReviews interface {
	OpenDealReviews(context.Context, ReviewScope) (reviews []DealReview, cut bool, err error)
}

// WithDealReviews connects the open customer-support reviews to the worklist.
func (s *Service) WithDealReviews(reviews DealReviews) *Service {
	s.reviews.source = reviews
	return s
}

// readingReviews carries this read's open reviews on a copy of the service.
func (s *Service) readingReviews(ctx context.Context, scope string) (*Service, *crmcontracts.WorklistSourceUnavailable) {
	defer timeStage(ctx, "deal_reviews")()
	scoped := *s
	if s.reviews.source == nil {
		return &scoped, nil
	}
	asked, refusal := s.reviewScope(ctx, scope)
	if refusal != nil {
		return &scoped, refusal
	}
	var reviews []DealReview
	var cut bool
	err := s.degradable(ctx, laneBudget, func(ctx context.Context) error {
		var err error
		reviews, cut, err = s.reviews.source.OpenDealReviews(ctx, asked)
		return err
	})
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return &scoped, reviewsUnavailable(crmcontracts.WorklistSourceUnavailableReasonWithheld)
	case err != nil:
		return &scoped, reviewsUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
	}
	scoped.reviews.open, scoped.reviews.cut = reviews, cut
	return &scoped, nil
}

// reviewScope is the page's scope as the review read asks it. It narrows the
// way narrowToScope narrows rows: a named owner, the reader, nobody, the
// reader's team, or everybody the reader may see.
func (s *Service) reviewScope(ctx context.Context, scope string) (ReviewScope, *crmcontracts.WorklistSourceUnavailable) {
	switch {
	case !s.taskOwner.IsZero():
		return ReviewScope{Owners: []ids.UUID{s.taskOwner}}, nil
	case mineOnly(scope):
		return ReviewScope{Owners: []ids.UUID{readerOf(ctx)}}, nil
	case scope == scopeUnassigned:
		return ReviewScope{Unowned: true}, nil
	case scope == scopeTeam:
		if s.teammates == nil {
			return ReviewScope{}, reviewsUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
		}
		roster, _, err := s.degradableRoster(ctx)
		if err != nil {
			return ReviewScope{}, reviewsUnavailable(crmcontracts.WorklistSourceUnavailableReasonFailed)
		}
		owners := make([]ids.UUID, 0, len(roster))
		for _, member := range roster {
			owners = append(owners, member.UserID)
		}
		return ReviewScope{Owners: owners}, nil
	default:
		return ReviewScope{All: true}, nil
	}
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
// at-risk row's deal facts, money and ranking, else the task's deadline. Its
// owner is the review's own. Its id is the review's, and it offers only `open`, because the
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
		}, occurredAt: review.OpenedAt}
	}
	// One owner per review, whichever rows this read happened to fold: the
	// task's assignee, else the deal's owner. Two readers cannot both own it.
	row.ownerRef = unassigned()
	if review.Owner != nil {
		row.ownerRef = ownedBy(*review.Owner)
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
