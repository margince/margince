// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// needsYouPage is how many decisions one read carries.
//
// The surface decides ONE item at a time and pulls the next as each is
// answered, so this is a prefetch depth rather than a ceiling on the day's work:
// the reader reaches every decision, a page at a time, and the count beside the
// lane always reports the true total.
//
// An earlier version capped the lane at nine with no way to continue while
// reporting ninety beside it. Nothing was reachable past the ninth, and the two
// numbers on screen contradicted each other.
const needsYouPage = 10

// plannedCap bounds today's agreed work for the same reason. Higher than the
// decision lane because reading a task costs less than deciding one.
const plannedCap = 12

// doneCap bounds the receipts. They are the least urgent thing on the surface
// and the easiest to let run long, so this is deliberately the shortest window
// on the page rather than a scrollback.
const doneCap = 8

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
	briefing   Briefing
	// commitments is OPTIONAL: nil means this feed serves no commitments lane,
	// and Assemble then leaves the field unset rather than sending an empty
	// array. The contract makes the lane optional for exactly that reason.
	commitments Commitments
	// atRisk is OPTIONAL for the reason commitments is: absent lane, not empty.
	atRisk AtRisk
	// decay is OPTIONAL for the same reason, and says nothing about atRisk:
	// an installation can warn about deals without deriving relationships.
	decay    Decay
	meetings Meetings
	failed   FailedEffects
	// dsrs is OPTIONAL like the lanes above it, and withheld-by-grant on top:
	// even where it is bound, only a DSR admin's read succeeds.
	dsrs DSRs
	// syncHealth is OPTIONAL like the lanes above it, and mode-gated on top:
	// even where it is bound, a workspace not in overlay mode answers
	// ErrModeNotOverlay and the lane stays absent (optionallanes.go).
	syncHealth SyncHealth
	// captureHealth is OPTIONAL like the lanes above it, and per-user on top:
	// the seam refuses a principal with no human behind it, and the lane
	// renders that as withheld.
	captureHealth CaptureHealth
	// aiWork is OPTIONAL and per-user exactly as captureHealth is.
	aiWork AIWork
	// bounces is OPTIONAL and per-user exactly as the two above it.
	bounces Bounces
	// automations is OPTIONAL and role-withheld exactly as dsrs is.
	automations AutomationHealth
	// notices is OPTIONAL and per-user exactly as captureHealth is.
	notices Notices
	// names is OPTIONAL like the lanes above it: nil means subjects travel
	// unnamed and the client resolves display names itself (labels.go).
	names Names
	now   Clock
}

// NewService binds the feed to its readers.
func NewService(
	a Approvals, d Duplicates, t Tasks, r Receipts, b Briefing,
	c Commitments, k AtRisk, q Decay, m Meetings, f FailedEffects, s DSRs, h SyncHealth, g CaptureHealth, w AIWork, o Bounces, u AutomationHealth, e Notices, n Names, now Clock,
) *Service {
	return &Service{
		approvals: a, duplicates: d, tasks: t, receipts: r, briefing: b,
		commitments: c, atRisk: k, decay: q, meetings: m, failed: f, dsrs: s, syncHealth: h, captureHealth: g, aiWork: w, bounces: o, automations: u, notices: e, names: n, now: now,
	}
}

// Assemble reads every lane and returns the day.
//
// A lane whose read is REFUSED is omitted and named rather than reported empty.
// Any other failure is returned: a lane that is broken rather than withheld
// must not read as a clear day.
func (s *Service) Assemble(ctx context.Context) (crmcontracts.Attention, error) {
	asOf := s.now().UTC()
	// Every lane starts as an empty slice, never nil. The contract declares
	// them as arrays, and a withheld lane leaves its field unset — which
	// serialises as `null` and breaks a generated client that iterates what the
	// schema promised was a list.
	out := crmcontracts.Attention{
		AsOf:        asOf,
		ThisMorning: []crmcontracts.AttentionItem{},
		NeedsYou:    []crmcontracts.AttentionItem{},
		Planned:     []crmcontracts.AttentionItem{},
		DoneForYou:  []crmcontracts.AttentionItem{},
	}
	var omitted []crmcontracts.AttentionLanesOmitted

	morning, morningState, err := s.thisMorning(ctx)
	omitted, err = fill(omitted, "this_morning", err, func() {
		out.ThisMorning = morning
		out.Counts.ThisMorning = len(morning)
		out.ThisMorningState = &morningState
	})
	if err != nil {
		return crmcontracts.Attention{}, err
	}

	needsYou, count, err := s.decisions(ctx)
	omitted, err = fill(omitted, "needs_you", err, func() {
		out.NeedsYou = needsYou
		out.Counts.NeedsYou = count.items
		if count.duplicates > 0 {
			open := count.duplicates
			out.Counts.DuplicatesOpen = &open
		}
	})
	if err != nil {
		return crmcontracts.Attention{}, err
	}

	planned, err := s.planned(ctx, asOf)
	omitted, err = fill(omitted, "planned", err, func() {
		out.Planned = planned
		out.Counts.Planned = len(planned)
	})
	if err != nil {
		return crmcontracts.Attention{}, err
	}

	// The three OPTIONAL lanes, each bound or absent. optionalLane holds the
	// shape they share; what differs is the reader and the drawing.
	for _, lane := range s.optionalLanes(ctx, asOf, &out) {
		omitted, err = lane.collect(omitted)
		if err != nil {
			return crmcontracts.Attention{}, err
		}
	}

	done, err := s.done(ctx, asOf)
	omitted, err = fill(omitted, "done_for_you", err, func() { out.DoneForYou = done })
	if err != nil {
		return crmcontracts.Attention{}, err
	}

	if len(omitted) > 0 {
		out.LanesOmitted = &omitted
	}
	// Last, over the assembled lanes: every card that names a record gets its
	// display name under this reader's own grants (labels.go).
	if err := s.fillSubjectLabels(ctx, &out); err != nil {
		return crmcontracts.Attention{}, err
	}
	return out, nil
}

// laneCount carries the totals behind a lane the reader only sees a slice of.
type laneCount struct {
	items      int
	duplicates int
}

// thisMorning is the briefing lane: the overnight run's unanswered queue, in
// its own rank order.
//
// No cap here. The brief is already honest-short — its own ranking bounds the
// queue and refuses to pad — so a second bound would hide items the engine had
// decided were worth the morning. The seam drops the answered ones, because
// this lane is a worklist that must be finishable and a row that cannot be
// removed is the opposite of finishing. Home still shows what was answered,
// which is where a rep looks to see what she did.
func (s *Service) thisMorning(ctx context.Context) ([]crmcontracts.AttentionItem, crmcontracts.AttentionThisMorningState, error) {
	queue, ran, err := s.briefing.Queue(ctx)
	if err != nil {
		return nil, "", err
	}
	items := make([]crmcontracts.AttentionItem, 0, len(queue))
	for _, entry := range queue {
		items = append(items, briefItem(entry))
	}
	// The state names WHY the lane holds what it holds. A run that ranked
	// nothing reads all_answered too: "nothing worth your first hour" and
	// "you answered everything" are the same message to a reader — nothing
	// to do here — while no_run_today is the one that must not wear a tick.
	state := crmcontracts.ItemsWaiting
	switch {
	case !ran:
		state = crmcontracts.NoRunToday
	case len(items) == 0:
		state = crmcontracts.AllAnswered
	}
	return items, state, nil
}

// decisions is the needs_you lane: staged approvals and open duplicate pairs,
// the two things on this surface a person alone may answer.
//
// Both producers are read to the full page depth and then INTERLEAVED, so one
// of them cannot bury the other. Reading each to depth and concatenating looks
// equivalent and is not: with eleven open pairs and a page of ten, every slot
// went to duplicates and seventy-nine staged approvals were unreachable from
// the surface that exists to reach them.
//
// Duplicates take the first slot of each round. A merge is the one verb here
// the product cannot undo, so it leads on stakes — but it leads by one place,
// not by the whole page.
func (s *Service) decisions(ctx context.Context) ([]crmcontracts.AttentionItem, laneCount, error) {
	pairs, err := s.duplicates.OpenCandidates(ctx, needsYouPage)
	if err != nil {
		return nil, laneCount{}, err
	}
	openPairs, err := s.duplicates.CountOpen(ctx)
	if err != nil {
		return nil, laneCount{}, err
	}
	staged, err := s.approvals.ListWire(ctx, ApprovalQuery{Status: "pending", Limit: needsYouPage})
	if err != nil {
		return nil, laneCount{}, err
	}
	openStaged, err := s.approvals.CountPending(ctx)
	if err != nil {
		return nil, laneCount{}, err
	}

	duplicates := make([]crmcontracts.AttentionItem, 0, len(pairs))
	for _, pair := range pairs {
		item, err := s.duplicateItem(ctx, pair)
		if err != nil {
			return nil, laneCount{}, err
		}
		duplicates = append(duplicates, item)
	}
	approvals := make([]crmcontracts.AttentionItem, 0, len(staged))
	for _, approval := range staged {
		approvals = append(approvals, approvalItem(approval))
	}
	return interleave(duplicates, approvals, needsYouPage),
		laneCount{items: openPairs + openStaged, duplicates: openPairs},
		nil
}

// interleave takes from `first` then `second` in turn, up to `limit`, and drains
// whichever still has items once the other runs dry.
//
// The alternation is what keeps a lane honest when one producer floods: a
// morning's import can raise a hundred duplicate pairs, and the reader still
// meets their staged decisions on the first screen.
func interleave(first, second []crmcontracts.AttentionItem, limit int) []crmcontracts.AttentionItem {
	out := make([]crmcontracts.AttentionItem, 0, limit)
	for i := 0; len(out) < limit && (i < len(first) || i < len(second)); i++ {
		if i < len(first) {
			out = append(out, first[i])
		}
		if len(out) < limit && i < len(second) {
			out = append(out, second[i])
		}
	}
	return out
}

// endOfDay is the boundary both due-dated lanes stop at, so a promise and a
// task falling on the same afternoon are judged against the same instant.
func endOfDay(asOf time.Time) time.Time {
	return asOf.Truncate(24 * time.Hour).Add(24 * time.Hour)
}

// planned is today's agreed work, overdue first.
func (s *Service) planned(ctx context.Context, asOf time.Time) ([]crmcontracts.AttentionItem, error) {
	open, err := s.tasks.OpenForViewer(ctx, endOfDay(asOf), plannedCap)
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
