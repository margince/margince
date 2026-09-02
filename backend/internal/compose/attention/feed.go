// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// batchScanDepth is how many staged decisions the RANKED queue reads.
//
// Deeper than needsYouPage on purpose, and for a reason that only applies to
// the queue: a batch row states how many decisions it stands for, and a count
// taken from a page of ten would say "10" over a pile of a hundred and fifty.
// The lane feed's page is a prefetch for a surface that answers one at a time;
// this is a census for a row that answers a group.
//
// Bounded by the approvals engine's own scan cap rather than by a number chosen
// here — past that the count reads "200+", which is the same message to a reader
// as the true figure.
const batchScanDepth = 200

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

// Zone answers the installation's timezone, which is what decides when "today"
// ends for this feed.
//
// A SEAM rather than a read of its own: this package binds no module and opens
// no transaction, and the installation's timezone is a fact the composition
// root already resolves for the morning brief through identity.TimezoneOf.
type Zone func(ctx context.Context) (*time.Location, error)

// Option adjusts a service after its seams are bound.
//
// Variadic rather than a nineteenth parameter: the constructor already takes
// every lane, and a reader counting arguments to find the one that decides the
// day is a reader who will bind it wrong.
type Option func(*Service)

// WithZone binds the installation timezone the day boundary is measured in.
//
// UNBOUND MEANS UTC, and that is the honest default rather than a hidden one: a
// composition with no installation to ask — every unit test in this package —
// has no local day, and UTC is the only answer that does not invent one. The
// shipped wiring binds it, which is what makes the difference visible.
func WithZone(z Zone) Option {
	return func(s *Service) { s.zone = z }
}

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
	// waiting is OPTIONAL like the lanes above it: an installation that does
	// not read the mail stream cannot say who is waiting, which is different
	// from saying nobody is.
	waiting Waiting
	// decay is OPTIONAL for the same reason, and says nothing about atRisk:
	// an installation can warn about deals without deriving relationships.
	decay    Decay
	meetings Meetings
	// zone resolves the installation timezone the day boundary is measured in;
	// nil is UTC, for the reason WithZone gives.
	zone   Zone
	failed FailedEffects
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
	// undelivered is OPTIONAL and per-user for the same reasons: mail that
	// never left, beside mail that arrived and was refused.
	undelivered Undelivered
	// automations is OPTIONAL and role-withheld exactly as dsrs is.
	automations AutomationHealth
	// notices is OPTIONAL and per-user exactly as captureHealth is.
	notices Notices
	// introductions is OPTIONAL and per-user exactly as notices is: an ask names
	// one colleague, so there is no wider tier for it to widen to.
	introductions Introductions
	// names is OPTIONAL like the lanes above it: nil means subjects travel
	// unnamed and the client resolves display names itself (labels.go).
	names Names
	// dealFacts is OPTIONAL in the same way: nil means a row whose producer
	// carried only a deal id travels without the deal's figures.
	dealFacts DealFacts
	// fx is OPTIONAL in the same way, and money is one read's answer from it,
	// written per read onto the request's own copy the way taskScope is.
	// basemoney.go states what each means and why the copy matters.
	fx    BaseMoney
	money dayMoney
	// machine answers whether an address is a sending system, for the group a
	// routine contact decision joins. Nil means every address reads as a
	// person's, which under-groups rather than hiding anything.
	machine MachineSender
	// teammates answers whether a team-scoped reader may open a named person's
	// queue. Unlike the lanes above it, nil does NOT mean "absent lane": it
	// means the question has no answer, and resolveOwner refuses rather than
	// admits. A lane whose absence widened a scope would be a security hole
	// wearing the shape of a missing feature.
	teammates Teammates
	// leads is the inbound leads still owed a first reply. Optional in the
	// ordinary way: nil is a feed that does not read leads at all, which the
	// queue reports as an absent source rather than as an empty one.
	leads LeadResponses
	// overdueLoad is the team board's COUNTING reader for tasks, beside the
	// bounded listing reader the ranked queue uses. Optional, and its absence
	// draws no column rather than a column of zeros.
	overdueLoad OverdueLoad
	// decisionDepth is how many staged decisions a read takes. The lane feed's
	// page is a prefetch for a surface that answers one at a time; the ranked
	// queue takes a census, because a batch row that says "10" over a pile of
	// a hundred and fifty is a wrong number rather than a bounded one.
	decisionDepth int
	now           Clock
	// taskScope narrows the task lane to whose work this read answers for. Set
	// per read (Assemble keeps the lane feed's behaviour; the ranked queue's
	// resolved scope sets it), because the same service answers both surfaces.
	//
	// A task carries an assignee and so has three answers available to it —
	// anybody's, the reader's, nobody's — where a mailbox or a notice is
	// per-reader by construction and has only one.
	taskScope TaskScope
	// taskOwner is whose queue TasksOwnedBy means. Zero for every other scope,
	// and never read by them.
	taskOwner ids.UUID
}

// forOwner returns a copy that reads one named person's queue. Same
// copy-per-read reason as forReader: a service is shared by every request, and
// an owner set on it would follow one manager's question onto another reader's
// page.
func (s *Service) forOwner(owner ids.UUID) *Service {
	narrowed := *s
	narrowed.taskScope = TasksOwnedBy
	narrowed.taskOwner = owner
	return &narrowed
}

// forReader returns a copy of this service that reads only the acting reader's
// own work. A copy rather than a field mutation: one Service is shared by every
// request, and a flag set on it would follow one reader's scope onto another
// reader's page.
func (s *Service) forReader() *Service {
	narrowed := *s
	narrowed.taskScope = TasksMine
	return &narrowed
}

// forUnowned returns a copy that reads the work nobody answers for. Same
// copy-per-read reason as forReader.
func (s *Service) forUnowned() *Service {
	narrowed := *s
	narrowed.taskScope = TasksUnassigned
	return &narrowed
}

// NewService binds the feed to its readers.
func NewService(
	a Approvals, d Duplicates, t Tasks, r Receipts, b Briefing,
	c Commitments, k AtRisk, q Decay, m Meetings, f FailedEffects, s DSRs, h SyncHealth, g CaptureHealth, w AIWork, o Bounces, u AutomationHealth, e Notices, n Names, now Clock,
	opts ...Option,
) *Service {
	svc := &Service{
		approvals: a, duplicates: d, tasks: t, receipts: r, briefing: b,
		commitments: c, atRisk: k, decay: q, meetings: m, failed: f, dsrs: s, syncHealth: h, captureHealth: g, aiWork: w, bounces: o, automations: u, notices: e, names: n, now: now,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// countingDecisions returns a copy of this service that reads decisions to
// census depth. A copy, because one Service serves every request and a depth
// set on it would follow one reader's page onto another's.
func (s *Service) countingDecisions() *Service {
	deeper := *s
	deeper.decisionDepth = batchScanDepth
	return &deeper
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

	needsYou, count, err := s.decisionsToDepth(ctx, s.decisionsDepth())
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

	planned, err := s.planned(ctx, asOf, s.taskScope)
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
// decisionsDepth answers how deep this read goes, defaulting to the lane
// feed's page.
func (s *Service) decisionsDepth() int {
	if s.decisionDepth > 0 {
		return s.decisionDepth
	}
	return needsYouPage
}

// decisionsToDepth is the same read at a depth the caller chooses: the lane feed
// takes a page, and the ranked queue takes a census so its batch rows can count.
func (s *Service) decisionsToDepth(ctx context.Context, depth int) ([]crmcontracts.AttentionItem, laneCount, error) {
	pairs, err := s.duplicates.OpenCandidates(ctx, depth)
	if err != nil {
		return nil, laneCount{}, err
	}
	openPairs, err := s.duplicates.CountOpen(ctx)
	if err != nil {
		return nil, laneCount{}, err
	}
	staged, err := s.approvals.ListWire(ctx, ApprovalQuery{Status: "pending", Limit: depth})
	if err != nil {
		return nil, laneCount{}, err
	}
	openStaged, err := s.approvals.CountPending(ctx)
	if err != nil {
		return nil, laneCount{}, err
	}

	named, err := s.namePairs(ctx, pairs)
	if err != nil {
		return nil, laneCount{}, err
	}
	duplicates := make([]crmcontracts.AttentionItem, 0, len(pairs))
	for _, pair := range pairs {
		duplicates = append(duplicates, s.duplicateItem(pair, named))
	}
	approvals := make([]crmcontracts.AttentionItem, 0, len(staged))
	for _, approval := range staged {
		approvals = append(approvals, approvalItem(approval, s.machine))
	}
	return interleave(duplicates, approvals, depth),
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

// endOfDay is the boundary every due-dated lane stops at, so a promise, a task
// and a meeting falling on the same afternoon are judged against one instant.
//
// THE INSTALLATION'S midnight, not UTC's. Truncating the clock to 24 hours
// answers UTC midnight wherever the installation is, which put a UTC+7 seat's
// "today" through 07:00 tomorrow and cut a UTC-4 seat's short at 20:00 — the
// same convention error the morning brief's local_day exists to avoid.
//
// AddDate over the local date, not Add(24h) over the instant: a day is 23 or 25
// hours where clocks change, and adding a fixed span lands an hour off on those
// two mornings a year — in the direction that drops work the reader is owed.
func (s *Service) endOfDay(ctx context.Context, asOf time.Time) (time.Time, error) {
	loc := time.UTC
	if s.zone != nil {
		resolved, err := s.zone(ctx)
		if err != nil {
			return time.Time{}, err
		}
		loc = resolved
	}
	local := asOf.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1), nil
}

// planned is today's agreed work, overdue first.
func (s *Service) planned(ctx context.Context, asOf time.Time, scope TaskScope) ([]crmcontracts.AttentionItem, error) {
	until, err := s.endOfDay(ctx, asOf)
	if err != nil {
		return nil, err
	}
	open, err := s.tasks.OpenForViewer(ctx, until, plannedCap, scope, s.taskOwner)
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
