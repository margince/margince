// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"sort"
	"time"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// needsYouCap bounds the decision lane.
//
// A queue whose whole promise is "this is what needs you" has to be finishable,
// and review quality falls off a cliff long before a scrolling list does. The
// count beside the lane reports the true total, so a bounded lane hides nothing
// — it only refuses to present forty decisions as one sitting.
const needsYouCap = 9

// plannedCap bounds today's agreed work for the same reason. Higher than the
// decision lane because reading a task costs less than deciding one.
const plannedCap = 12

// doneCap bounds the receipts. They are the least urgent thing on the surface
// and the easiest to let run long, so this is deliberately the shortest window
// on the page rather than a scrollback.
const doneCap = 8

// Approvals is the staged-proposal queue, read through its owning service.
//
// CountPending is separate from the page because the lane is bounded and the
// count is not: a reader with forty decisions must be told forty, then shown
// the nine worth one sitting.
type Approvals interface {
	ListWire(ctx context.Context, in ApprovalQuery) ([]crmcontracts.Approval, error)
	CountPending(ctx context.Context) (int, error)
}

// ApprovalQuery is what this feed asks the approvals engine for. Narrower than
// the engine's own input on purpose: the feed reads one page of one status and
// takes no part in choosing scope, which stays the engine's to decide.
type ApprovalQuery struct {
	Status string
	Limit  int
}

// Duplicates is the dedupe queue, read through the people module. Both halves
// carry that module's both-sides-visible rule; nothing here re-derives it.
type Duplicates interface {
	OpenCandidates(ctx context.Context, limit int) ([]DuplicatePair, error)
	CountOpen(ctx context.Context) (int, error)
}

// DuplicatePair is one open candidate, already reduced to what a card shows.
type DuplicatePair struct {
	ID         ids.UUID
	EntityType string
	Confidence float64
}

// Tasks is the open-task read, through the activities module.
type Tasks interface {
	OpenForViewer(ctx context.Context, until time.Time, limit int) ([]Task, error)
}

// Task is one piece of agreed work.
type Task struct {
	ID      ids.UUID
	Subject string
	DueAt   *time.Time
}

// Receipts is what the system did on its own, most recent first.
type Receipts interface {
	Recent(ctx context.Context, since time.Time, limit int) ([]Receipt, error)
}

// Receipt is one completed autonomous act, reported rather than asked about.
type Receipt struct {
	ID         ids.UUID
	Kind       string
	Summary    string
	OccurredAt time.Time
}

// Clock is the read's instant, injected so the lane boundaries a test asserts
// are the ones it set.
type Clock func() time.Time

// Service assembles the feed. Every dependency is an interface a compose seam
// binds to the owning module, so this package imports no module directly.
type Service struct {
	approvals  Approvals
	duplicates Duplicates
	tasks      Tasks
	receipts   Receipts
	now        Clock
}

// NewService binds the feed to its readers.
func NewService(a Approvals, d Duplicates, t Tasks, r Receipts, now Clock) *Service {
	return &Service{approvals: a, duplicates: d, tasks: t, receipts: r, now: now}
}

// Assemble reads every lane and returns the day.
//
// A lane whose read is REFUSED is omitted and named rather than reported empty.
// Any other failure is returned: a lane that is broken rather than withheld
// must not read as a clear day.
func (s *Service) Assemble(ctx context.Context) (crmcontracts.Attention, error) {
	asOf := s.now().UTC()
	out := crmcontracts.Attention{AsOf: asOf}
	var omitted []crmcontracts.AttentionLanesOmitted

	needsYou, count, err := s.decisions(ctx)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		omitted = append(omitted, crmcontracts.AttentionLanesOmitted("needs_you"))
	case err != nil:
		return crmcontracts.Attention{}, err
	default:
		out.NeedsYou = needsYou
		out.Counts.NeedsYou = count.items
		if count.duplicates > 0 {
			open := count.duplicates
			out.Counts.DuplicatesOpen = &open
		}
	}

	planned, err := s.planned(ctx, asOf)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		omitted = append(omitted, crmcontracts.AttentionLanesOmitted("planned"))
	case err != nil:
		return crmcontracts.Attention{}, err
	default:
		out.Planned = planned
		out.Counts.Planned = len(planned)
	}

	done, err := s.done(ctx, asOf)
	switch {
	case errors.Is(err, apperrors.ErrPermissionDenied):
		omitted = append(omitted, crmcontracts.AttentionLanesOmitted("done_for_you"))
	case err != nil:
		return crmcontracts.Attention{}, err
	default:
		out.DoneForYou = done
	}

	if len(omitted) > 0 {
		out.LanesOmitted = &omitted
	}
	return out, nil
}

// laneCount carries the totals behind a lane the reader only sees a slice of.
type laneCount struct {
	items      int
	duplicates int
}

// decisions is the needs_you lane: staged approvals and open duplicate pairs,
// the two things on this surface a person alone may answer.
//
// Duplicates lead. A merge cannot be undone — the product has no unmerge — and
// putting the one irreversible verb on the page after a list of reversible ones
// would rank by producer instead of by stakes.
func (s *Service) decisions(ctx context.Context) ([]crmcontracts.AttentionItem, laneCount, error) {
	pairs, err := s.duplicates.OpenCandidates(ctx, needsYouCap)
	if err != nil {
		return nil, laneCount{}, err
	}
	openPairs, err := s.duplicates.CountOpen(ctx)
	if err != nil {
		return nil, laneCount{}, err
	}
	staged, err := s.approvals.ListWire(ctx, ApprovalQuery{Status: "pending", Limit: needsYouCap})
	if err != nil {
		return nil, laneCount{}, err
	}
	openStaged, err := s.approvals.CountPending(ctx)
	if err != nil {
		return nil, laneCount{}, err
	}

	items := make([]crmcontracts.AttentionItem, 0, len(pairs)+len(staged))
	for _, pair := range pairs {
		items = append(items, duplicateItem(pair))
	}
	for _, approval := range staged {
		items = append(items, approvalItem(approval))
	}
	total := openPairs + openStaged
	if len(items) > needsYouCap {
		items = items[:needsYouCap]
	}
	return items, laneCount{items: total, duplicates: openPairs}, nil
}

// planned is today's agreed work, overdue first.
func (s *Service) planned(ctx context.Context, asOf time.Time) ([]crmcontracts.AttentionItem, error) {
	endOfDay := asOf.Truncate(24 * time.Hour).Add(24 * time.Hour)
	open, err := s.tasks.OpenForViewer(ctx, endOfDay, plannedCap)
	if err != nil {
		return nil, err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(open))
	for _, task := range open {
		items = append(items, taskItem(task, asOf))
	}
	// Overdue first: a promise already broken outranks one merely due, and the
	// server resolves it so every surface agrees on where the line falls.
	sort.SliceStable(items, func(i, j int) bool {
		return overdue(items[i]) && !overdue(items[j])
	})
	return items, nil
}

// done is the receipt lane: what ran without asking, so a rep can see it and
// challenge it.
func (s *Service) done(ctx context.Context, asOf time.Time) ([]crmcontracts.AttentionItem, error) {
	since := asOf.Add(-24 * time.Hour)
	recent, err := s.receipts.Recent(ctx, since, doneCap)
	if err != nil {
		return nil, err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(recent))
	for _, receipt := range recent {
		items = append(items, receiptItem(receipt))
	}
	return items, nil
}

func overdue(item crmcontracts.AttentionItem) bool {
	return item.Overdue != nil && *item.Overdue
}
