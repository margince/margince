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
// Describe names the two records of a pair. It is separate from OpenCandidates
// because naming a record is a READ of that record: the queue row proves a pair
// was detected, never that this reader may see what it points at.
type Duplicates interface {
	OpenCandidates(ctx context.Context, limit int) ([]DuplicatePair, error)
	CountOpen(ctx context.Context) (int, error)
	Describe(ctx context.Context, entityType string, id ids.UUID) (RecordFace, error)
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
	Queue(ctx context.Context) (entries []BriefEntry, ran bool, err error)
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

// AtRisk is the open deals going quiet, or already past their expected close.
//
// The seam behind it reads the SAME candidate engine the whats_slipping tool
// reads, at a shorter idle window. A second at-risk rule living here would be
// two answers to one question, and the two would disagree in front of a rep.
//
// Optional exactly as Commitments is: nil means this feed does not do deal risk,
// which is a different fact from a pipeline with nothing wrong in it.
type AtRisk interface {
	Quiet(ctx context.Context) ([]RiskyDeal, error)
}

// RiskyDeal is one deal the pipeline should worry about, and the ground it is
// worried on.
//
// Both flags travel because they are different warnings: a deal nobody has
// touched is neglected, and a deal past its close date is late whether or not
// anyone touched it. A card that collapsed them would say "at risk" and leave
// the rep to guess which.
type RiskyDeal struct {
	DealID ids.UUID
	Name   string
	// The card's facts, carried so the client draws value, stage and
	// ownership without a second read per row. All optional: a deal can be
	// ownerless, unpriced, or an overlay mirror with no native stage.
	StageID     *ids.UUID
	OwnerID     *ids.UUID
	AmountMinor *int64
	Currency    *string
	// QuietDays is how long the deal has been idle, which is the number the
	// card says out loud. Zero for a deal admitted only by its close date.
	QuietDays int
	// CloseOverdue is set when the expected close date has already passed.
	CloseOverdue      bool
	ExpectedCloseDate *time.Time
}

// Decay is the reader's own relationships that have gone silent.
//
// A separate lane from AtRisk rather than more rows in it, because the two rest
// on different records and warn about different things: AtRisk is a DEAL nobody
// is moving, and this is a PERSON nobody is talking to. A contact carrying no
// open deal never reaches that lane at all, and those are exactly the
// relationships that lapse without anyone noticing.
//
// The seam behind it derives the silence through the same §4 change engine the
// contact's own page reads, so there is one quiet rule in the product rather
// than a second threshold spelled here.
//
// Optional exactly as the others are: nil means this feed does not derive
// relationship changes, which is a different fact from a rep whose
// relationships are all current.
type Decay interface {
	Lapsed(ctx context.Context) ([]QuietRelationship, error)
}

// QuietRelationship is one contact this reader has stopped talking to.
type QuietRelationship struct {
	PersonID ids.UUID
	Name     string
	// QuietDays is how long the silence has run, which is the number the card
	// says out loud. It comes from the derivation rather than from the
	// projection's own last_at, so the card and the contact's page agree.
	QuietDays int
	// LastAt is when they last spoke, so the card can date the silence rather
	// than only measure it.
	LastAt time.Time
}

// Waiting is who has written to this workspace and had no reply.
//
// Its own reader rather than a filter over AtRisk, because a fresh inbound
// makes a deal LESS quiet: deriving "waiting" from "quiet" loses the newest
// cases, which are the ones a rep most needs. It also reaches a person with no
// deal at all, whom the deal-shaped lanes never see.
type Waiting interface {
	Unanswered(ctx context.Context, asOf time.Time) ([]WaitingCustomer, error)
}

// WaitingCustomer is one message nobody has answered.
type WaitingCustomer struct {
	// ActivityID is the message itself — what a reply would be drafted to.
	ActivityID ids.UUID
	Subject    string
	// Since is when they wrote. The wait is measured from it, and it is what
	// the card says out loud.
	Since time.Time
	// The record the thread is filed under, most specific first. Any may be
	// zero: a message from a stranger names nobody.
	PersonID       ids.UUID
	OrganizationID ids.UUID
	DealID         ids.UUID
	// HasOpenDeal reports whether money this reader can see is still on this
	// thread. It is what keeps a long wait in execution instead of sending it
	// to review.
	HasOpenDeal bool
	// OwnerID is who owes this reply, resolved by the module from the record
	// the thread is filed under. Zero when nothing on it names an owner, which
	// is an unowned customer rather than a missing answer.
	OwnerID ids.UUID
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
