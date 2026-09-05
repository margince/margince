// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// What each lane READS, and the shape it reads it into.
//
// Every one is an interface a compose seam binds to the owning module, which is
// what lets this package assemble a day without importing a single module. They
// sit apart from the assembly in feed.go because they are the surface another
// package implements: a reader adding a lane needs this file and not the rest.
//
// The optional lanes are nil when the installation binds no reader for them,
// and a nil lane is ABSENT from the feed rather than empty — "this feed does
// not do commitments" is a different fact from "you owe nobody anything".

// Approvals is the staged-proposal queue, read through its owning service.
//
// CountPending is separate from the page because the lane is bounded and the
// count is not: a reader with forty decisions must be told forty, then shown
// the nine worth one sitting.
type Approvals interface {
	ListWire(ctx context.Context, in ApprovalQuery) ([]crmcontracts.Approval, error)
	CountPending(ctx context.Context) (int, error)
}

// MachineSender answers whether an address belongs to a sending system rather
// than a person.
//
// Injected rather than imported: this package reaches no module, and the rule
// belongs to capture, which owns what a machine sender IS. A feed given none
// treats every address as a person's, which under-groups rather than hiding
// anything.
type MachineSender func(address string) bool

// StagedFacts are the few things about a staged proposal this queue needs that
// its summary sentence does not carry: enough to put a routine decision in the
// right group without reading the payload twice.
type StagedFacts struct {
	// MachineSender is set when the address the decision is about belongs to a
	// sending system rather than a person.
	MachineSender bool
	// KnownCompany is set when its domain already names a company here.
	KnownCompany bool
}

// ApprovalQuery is what this feed asks the approvals engine for. Narrower than
// the engine's own input on purpose: the feed reads one page of one status and
// takes no part in choosing scope, which stays the engine's to decide.
type ApprovalQuery struct {
	Status string
	Limit  int
}

// Duplicates is the dedupe queue, read through the people module. Every method
// carries that module's both-sides-visible rule; nothing here re-derives it.
//
// DescribeMany names records of ONE entity type. It is separate from
// OpenCandidates because naming a record is a READ of that record: the queue row
// proves a pair was detected, never that this reader may see what it points at.
//
// A SET at a time, not a record at a time. The lane renders up to ten pairs, so
// naming them one by one is twenty scoped reads on the surface a rep opens first
// every morning. An id the reader may not see is simply ABSENT from the answer,
// which is what its refusal meant.
//
// DecidableSubset asks the OTHER question about the same records: not whether
// this reader may see them, but whether they could change them. Settling a pair
// archives one record and rewrites the other, so a reader holding neither
// side's write authority has no verb to be offered — and the card is where they
// should learn that, rather than from a refusal after the press.
type Duplicates interface {
	OpenCandidates(ctx context.Context, limit int) ([]DuplicatePair, error)
	CountOpen(ctx context.Context) (int, error)
	DescribeMany(ctx context.Context, entityType string, rowIDs []ids.UUID) (map[ids.UUID]RecordFace, error)
	DecidableSubset(ctx context.Context, entityType string, rowIDs []ids.UUID) (map[ids.UUID]bool, error)
	// SettleablePairs asks the PAIR's own question, which authority cannot:
	// whether the merge would be accepted at all. Two companies each carrying
	// live work refuse to combine (PROJ-LIFE-4), so a steward holding both
	// sides' write authority was still offered a button that answered 409
	// every time. Per pair rather than per record, because "both carry
	// projects" is not a property either record has alone.
	SettleablePairs(ctx context.Context, pairs []DuplicatePair) (map[ids.UUID]bool, error)
}

// DuplicatePair is one open candidate: the pair, and what the detector saw.
type DuplicatePair struct {
	ID         ids.UUID
	EntityType string
	Confidence float64
	LeftID     ids.UUID
	RightID    ids.UUID
	Evidence   []FieldComparison
}

// FieldComparison is one row of the detection-time snapshot.
type FieldComparison struct {
	Field  string
	Left   *string
	Right  *string
	Signal string
}

// RecordFace is how much of a record a merge decision needs: enough to tell the
// two sides apart, and nothing more.
type RecordFace struct {
	Label        string
	Detail       string
	CreatedAt    *time.Time
	RelatedCount *int
}

// TaskScope says whose open tasks a read answers.
//
// Three answers, not a boolean, because "nobody's" is a real one: unowned work
// has to be reachable without arriving in every reader's own queue.
type TaskScope int

const (
	// TasksVisible adds no narrowing of its own: the store's row-scope gate
	// decides what comes back.
	TasksVisible TaskScope = iota
	// TasksMine is the open tasks assigned to the reader, and no others.
	TasksMine
	// TasksUnassigned is the open tasks assigned to nobody.
	TasksUnassigned
	// TasksOwnedBy is the open tasks assigned to one NAMED person — a manager
	// opening the queue of the rep an exception named. The name rides beside
	// the scope, because a scope value cannot carry one.
	TasksOwnedBy
)

// Tasks is the open-task read, through the activities module.
//
// The scope belongs in the QUERY and not in a filter afterwards: the store
// bounds what it returns, so narrowing later would cut a colleague's twelve
// rows out of a page of twelve and leave the reader's own overdue task
// unreachable behind them.
type Tasks interface {
	OpenForViewer(ctx context.Context, until time.Time, limit int, scope TaskScope, owner ids.UUID) ([]Task, error)
	// CountOpenForViewer answers how many there ARE under the same narrowing,
	// which is not the page length: the lane is capped at a dozen, so a badge
	// showing the cap tells a reader with thirteen that they have twelve, and
	// there is no second page to reach the thirteenth by.
	CountOpenForViewer(ctx context.Context, until time.Time, scope TaskScope, owner ids.UUID) (int, error)
}

// Task is one piece of agreed work.
type Task struct {
	ID      ids.UUID
	Subject string
	DueAt   *time.Time
	// The record this task was raised for, when it names one. A follow-up task
	// says "Follow up with the new lead" and nothing else, so without the link
	// the row is a sentence the reader cannot act on: the lane knows which lead
	// it means and used to keep that to itself.
	//
	// Empty LinkType means the task is filed under nothing this surface can
	// route to, which is a real state and not an error.
	LinkType string
	LinkID   ids.UUID
	// AssigneeID is who the task belongs to, nil when nobody has taken it.
	//
	// The lane serves three scopes and only one of them is the reader's own
	// queue: an unassigned sweep and a named colleague's queue both put tasks
	// on the page that are not theirs. Without this the rows read identically,
	// and the one nobody owns — the whole point of that scope — cannot say so.
	AssigneeID *ids.UUID
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
	// The record the decision was about, carried from the approval it came
	// from. A reader told the system sent something on their behalf wants the
	// account it went to, and the card offers `open` only when this names one.
	//
	// Empty TargetType means the decision named no record, which is a real
	// state: not every approval is about one.
	TargetType string
	TargetID   ids.UUID
}

// FailedEffects reads the decisions the acting rep approved whose released
// work then failed — bounded, newest-staged first (the approvals queue's own
// order; a failure on an old staging sorts beneath newer rows).
type FailedEffects interface {
	Failed(ctx context.Context, limit int) ([]FailedEffect, error)
}

// FailedEffect is one approved decision whose promised work never happened.
type FailedEffect struct {
	ID   ids.UUID
	Kind string
	// Sentence is the server's own line about what did not run, written for
	// the reader when the failure was recorded.
	Sentence string
	FailedAt time.Time
	// The record the decision was about; empty TargetType when it named none,
	// and the card offers `open` only when it did.
	TargetType string
	TargetID   ids.UUID
}

// DSRs reads the data-subject requests nobody has resolved, soonest legal
// deadline first, bounded. The read is refused for everyone but a DSR
// admin, and the lane renders that refusal as withheld.
type DSRs interface {
	OpenDueSoonest(ctx context.Context, limit int) ([]DSRCase, error)
}

// DSRCase is one request still owed an answer: what was asked, and by when
// the law expects it answered.
type DSRCase struct {
	ID    ids.UUID
	Kind  string
	DueAt time.Time
}

// Briefing is the overnight brief's queue for the acting rep, best-ranked
// first.
//
// No run for today is NOT a refusal: the night has simply not produced one, or
// there was nothing worth ranking, and both are honestly an empty lane. The
// implementation therefore answers an empty slice rather than an error for that
// case, and reserves apperrors.ErrPermissionDenied for a caller who may not
// read the queue at all.
type Briefing interface {
	// Queue answers the unanswered entries AND whether a run exists at all:
	// ran=false means the night produced nothing for this reader, ran=true
	// with no entries means every item is answered. The two render as the
	// same empty lane, and only the second has earned a tick — so the seam
	// states which rather than leaving the feed to infer it from emptiness.
	// Queue answers the unanswered entries, whether a run exists at all, and the
	// run's DATA CUTOFF — its as_of, which is the instant the night read the
	// records, not the later instant at which it finished writing them down.
	//
	// The cutoff is what "changed since the brief" means. A run generated at
	// 06:42 over data read at 06:00 has a 42-minute window in which a buyer can
	// reply, and judging freshness by generated_at would hide exactly the replies
	// a rep most wants to see. Zero when no run exists, which is why `ran` is a
	// separate answer: absent is not the same as false.
	Queue(ctx context.Context) (entries []BriefEntry, ran bool, asOf time.Time, err error)
}

// BriefEntry is one UNANSWERED queue entry: what it is about, and where it
// ranks.
//
// It carries the item id and the deal, not the factor vector or the evidence.
// The card that draws those reads the brief's own endpoint, which the screen
// already calls — copying the ranking payload through this feed would put the
// same numbers on two wires and let them disagree.
//
// There is no state field, because the seam has already dropped the answered
// entries. The brief owns what its states mean and what each spells; a copy of
// that vocabulary here would be a second place to keep it right.
type BriefEntry struct {
	ID     ids.UUID
	DealID ids.UUID
	Rank   int
	// Composite is the night's own score for this deal, between 0 and 1, and
	// what ranksteps.go's `opportunity` step breaks a tie by.
	Composite float64
	// Finding is what the overnight pass wrote about this deal, empty when no
	// pass annotated the run. Grounded and citation-checked when it was written
	// (briefs.AnnotateCurrentRun refuses one that cites outside the run), which
	// is what makes it usable as a standing line where no deal card is cached.
	Finding string
}

// Commitments is the rep's own outstanding promises, soonest-due first.
//
// Read from the claims extracted out of captured conversations rather than from
// the task list, because a commitment card has to show BOTH halves — when it is
// due, and where it was promised. An open task carries the date and no
// provenance; a message labelled as a commitment carries the provenance and no
// date. Only the claim carries both.
//
// An installation that reads no claims binds nothing here, and the lane is then
// absent rather than empty: nil means "this feed does not do commitments",
// which is a different fact from "you owe nobody anything today".
type Commitments interface {
	DueBy(ctx context.Context, by time.Time, limit int) ([]Commitment, error)
	// CountDueBy answers how many are due, for the reason Tasks gives.
	CountDueBy(ctx context.Context, by time.Time) (int, error)
}

// Commitment is one promise this rep made, with the evidence behind it.
//
// Body and Quote both travel: the claim contract's rule is that a claim is
// checkable against what was actually written, and a card that showed only the
// paraphrase would be asking the reader to trust the extractor.
type Commitment struct {
	ID          ids.UUID
	PersonID    ids.UUID
	Body        string
	Quote       string
	SourceLabel string
	OccurredAt  time.Time
	DueAt       time.Time
}

// DealFacts answers the figures behind deals a row names but does not carry.
//
// Most rows arrive with their deal's numbers already on them, because the lane
// that produced them read the deal. The overnight brief does not: it ranks deal
// ids and keeps its composite and factor vector behind its own endpoint, so a
// card that draws those reads them there. Without this seam its rows reach a
// rep naming a deal and saying nothing about it — no amount, no close date,
// nothing to act on.
//
// What this answers is the deal's OWN columns, which every other lane already
// carries onto its rows. The ranking arithmetic stays where it is.
//
// One call for every id on the page, like the label pass beside it, rather than
// one per row.
//
// A deal the caller may not read is simply absent from the answer, which is the
// same refusal shape Names uses: the row keeps its name and loses its figures,
// and the id still travels because naming the deal was the producer's claim.
type DealFacts interface {
	Figures(ctx context.Context, dealIDs []ids.UUID) (map[ids.UUID]DealFigures, error)
}

// DealFigures is what a card needs to state a deal's commercial case: what it
// is worth, when it was meant to land, and who answers for it.
type DealFigures struct {
	StageID           ids.UUID
	OwnerID           ids.UUID
	AmountMinor       *int64
	Currency          string
	ExpectedCloseDate *time.Time
	// CloseOverdue is the SAME calendar-date, workspace-zone verdict
	// deals.CloseIsOverdue gives the at-risk lane's identical deal. Meaningless
	// where ExpectedCloseDate is nil — a deal with no close date is not late by
	// one, it has none to be late by.
	CloseOverdue bool
}

// Meetings is today's booked meetings that have not happened yet.
//
// Optional as the other two are: nil means this feed does not read meetings,
// which is not the same as a day with none in it.
type Meetings interface {
	Today(ctx context.Context, from, until time.Time, limit int) ([]Meeting, error)
}

// Meeting is one appointment still ahead of the reader.
type Meeting struct {
	ID       ids.UUID
	Subject  string
	StartsAt time.Time

	// PersonID is whose page the brief is read on, and it is zero whenever the
	// meeting names nobody this reader may see.
	//
	// The brief is not a page of its own: it opens as `?prep=<activity>` on a
	// PERSON's record, so the activity id the row already carries names the
	// meeting and says nothing about where to read it. Without this the lane
	// could describe a meeting and offer no way to prepare for it, which is
	// the one thing a rep opens the row to do.
	//
	// An internal meeting legitimately has none, and so does one whose only
	// attendees are people the reader cannot read. Both stay zero and the row
	// offers no verb rather than a link to somebody's page picked at random.
	PersonID ids.UUID

	// NeedsPrep is true when nothing has been written down for a meeting that
	// is about to happen: no agenda or notes body, and nobody outside this
	// organization recorded on it.
	//
	// It is a THREE-state answer squeezed into a bool plus its guard below, and
	// the third state is why: a meeting whose content this reader may not read
	// arrives with an empty body for a reason that has nothing to do with
	// preparation. Calling that "needs prep" would tell a rep to prepare a
	// meeting they cannot see, so the lane leaves PrepKnown false instead and
	// the surface says nothing rather than something false.
	NeedsPrep bool

	// PrepKnown reports whether NeedsPrep was answerable at all. False when the
	// row's content is withheld from this reader.
	PrepKnown bool
}

// Notices is the acting person's own unread notices — the durable
// informational line a system flow needed them to see. Per-user like the
// health lanes: the read refuses a principal with no human behind it, and
// the lane renders that refusal as withheld.
type Notices interface {
	Unread(ctx context.Context, limit int) ([]UnreadNotice, error)
}

// UnreadNotice is one line still waiting to be seen.
type UnreadNotice struct {
	ID        ids.UUID
	Kind      string
	Subject   string
	Body      string
	CreatedAt time.Time
}

// Introductions is the asks waiting on THIS reader to answer — a colleague
// wanting them to open a door.
//
// Per-user like Notices, and for the same reason: the ask names one colleague,
// so there is no wider tier for it to widen to. The `team` and `all` scopes
// reach shared record-bearing work, not the question of whose favour somebody
// else asked for.
//
// Without this lane an ask reached its colleague only if they happened to open
// that contact's Network tab, so one nobody went looking for expired unanswered
// — which reads to the requester exactly like a refusal.
type Introductions interface {
	Pending(ctx context.Context, limit int) ([]PendingIntroduction, error)
}

// PendingIntroduction is one colleague's ask, as the queue draws it.
//
// It names the CONTACT the introduction would be to, which is what the
// colleague is deciding about. No display name travels: resolving one is a read
// of that person's record, and fillSubjectLabels already makes it under the
// reader's own grants (labels.go). A name carried here would be a second,
// ungated answer to the question that pass exists to ask.
type PendingIntroduction struct {
	ID       ids.UUID
	PersonID ids.UUID
	// Reason is the requester's own sentence for why the ask is worth making,
	// written by them at the time. Never composed here: this queue puts no
	// words in a colleague's mouth.
	Reason      string
	RequestedAt time.Time
	// DueAt is when the ask lapses. It is what this lane orders by, so the one
	// about to expire is the one a reader sees first.
	DueAt time.Time
}
