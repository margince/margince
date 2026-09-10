// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The timeline read's query vocabulary: every dial a caller can narrow a list
// with, and what each one MEANS.
//
// Apart from the readers that execute it because it is the surface callers
// write against — a reader adding a filter needs this file and not the SQL, and
// the SQL's own file stays about how a page is fetched. What each dial renders
// to lives in orgscope.go, beside the other scope clauses.

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListActivitiesInput narrows a timeline read: the page, the kind, the
// transport, and the record the activities hang off.
type ListActivitiesInput struct {
	Cursor *string
	Limit  *int
	Kind   *string
	// ChannelProvider narrows to messages carried by ONE transport. Since the
	// kind stopped naming the transport (ADR-0107/A158) this is the only way to
	// ask the question `kind=telegram` used to answer, and it is a separate dial
	// from Kind rather than a second spelling of it: the two compose, so "every
	// message on telegram" and "every message" are both askable.
	ChannelProvider *string
	EntityType      *string
	// note: EntityType+EntityID is the polymorphic activity_link filter —
	// the target is ANY entity kind, so the id stays untyped (rule 6).
	EntityID *ids.UUID
	// Query is the contract's `q`: a substring match over the subject and
	// body a human would recognize the item by.
	Query *string
	// ThreadKey narrows to ONE provider conversation. The company timeline
	// groups client-side over the page it holds; a group the page cut off
	// completes itself through this rather than by widening the page for every
	// account that has no long thread.
	ThreadKey       *string
	IncludeArchived bool
	// AssigneeID is the work queue's narrowing: the OPEN tasks one person
	// holds, which is what the contract declares the parameter to mean and
	// what the partial index behind it is built on. Done-ness is part of
	// that question rather than a second dial — see openTaskAssigneeClause.
	AssigneeID *ids.UserID

	// OwnQueueOf narrows to the open work one person is answerable for.
	// Distinct from AssigneeID, which means exact assignment on any kind and is
	// what the task screen filters by; this one is the day's queue and carries
	// open-ness with it.
	OwnQueueOf *ids.UserID
	// UnassignedQueue narrows to the open tasks nobody answers for.
	//
	// A scope of its own rather than an arm of OwnQueueOf: unowned work is
	// somebody's to pick up, and a reader chooses to look at it. Folded into a
	// personal queue it arrives as though already theirs, which is how one
	// automation's follow-up came to sit on every colleague's page.
	UnassignedQueue bool

	// The three meeting narrowings the brief lanes ask with.
	//
	// They are worklist OWNERSHIP filters, not authorization. What a reader may
	// see is decided by the row-scope gate and the audience arm, which read
	// capture provenance, imports and participants — never host_user_id. These
	// only choose which of the rows a reader already passes the lane is about.
	//
	// OnMeetingOf is "mine": the meetings this person is genuinely on — their own
	// calendar hosted it, their seat imported it, or they are stamped as a
	// participant. Three sources because a meeting reaches a person three ways,
	// and asking about the host alone would drop every meeting a colleague was
	// invited to.
	OnMeetingOf *ids.UserID
	// MeetingHost is one NAMED person's own calendar — a manager opening the day
	// of the rep an exception named. Exact, unlike OnMeetingOf: asking for a
	// rep's day means the rep's calendar, not every meeting they were invited to.
	MeetingHost *ids.UserID
	// UnhostedMeetings is the meetings no calendar claims: booked in the app, or
	// captured before the host was recorded. Its own scope for the reason
	// UnassignedQueue is one — unowned work is picked up deliberately rather
	// than arriving unbidden in everybody's day.
	UnhostedMeetings bool

	// ReadableOnly drops the rows this caller may discover but not read, in SQL,
	// BEFORE the page is cut.
	//
	// For a timeline that is wrong: a withheld row is a real event, and showing
	// it with its content blanked is how a reader learns something happened
	// without learning what. For a WORKLIST it is the only correct answer. A lane
	// offers rows to act on, and a row whose subject the reader cannot read is
	// not one — worse, dropping it after the fetch spends the page on rows the
	// caller then discards, so a full page of a colleague's held meetings hides
	// the reader's own meeting sitting behind them.
	ReadableOnly bool
	// WithinProjectID narrows to one body of work, EXCLUDING what belongs to
	// another project and keeping what belongs to none.
	//
	// It is not a second spelling of EntityType="project"+EntityID, and the
	// difference is the whole point. That pair asks "what is filed under this
	// project"; this asks "what is on this account, minus the other
	// engagement" — the anchor stays the person or company, and the general
	// correspondence that carries no project at all stays with it. A reader
	// preparing for an ERP meeting still wants the relationship's history;
	// they do not want the datacentre migration.
	WithinProjectID *ids.ProjectID
	// OccurredAfter / OccurredBefore bound the timeline to a range: the
	// lower end inclusive, the upper end exclusive, so a calendar day is
	// [day 00:00, next day 00:00) with no double-counting at midnight.
	OccurredAfter  *time.Time
	OccurredBefore *time.Time
	// AwaitingOutcome narrows to meetings nobody has said the result of.
	//
	// A DIAL RATHER THAN A GO-SIDE FILTER, unlike the forward meetings lane
	// which removes non-booked rows after reading. That lane's window is the
	// rest of today, so what it discards is bounded by a set the database
	// already made small. This question looks BACKWARD, where almost every
	// meeting is settled: a newest-first page of the past is dominated by rows
	// the filter throws away, and the unreported meeting from three days ago
	// falls off the end. The lane then draws "nothing to report" over real
	// work — lossy in the one direction that hides itself.
	//
	// It admits a NULL status as well as `booked`. Capture writes calendar
	// events with no status at all, so a synced meeting that happened
	// yesterday carries NULL and is exactly the row this asks for; matching
	// only `booked` would empty the lane on every installation whose calendar
	// is connected. Same rule as meetingStillWorthPreparing, which is the
	// forward lane's spelling of the same fact.
	AwaitingOutcome bool
	// WaitingReplyAsOf narrows the list to the SAME thread walk WaitingReplies
	// answers for the Worklist: the newest inbound message per thread that
	// nobody has answered, as of this instant. Nil means the filter is off.
	//
	// An instant rather than a plain bool, for the reason OpenAndDueBy is one:
	// the caller resolves it (the store's own clock for an HTTP read, a fixed
	// moment for a test), so the whole read stays one snapshot rather than
	// this filter and the rest of the page judging against two different
	// clock reads.
	WaitingReplyAsOf *time.Time
	// ownDomains is the colleague-domain snapshot the waiting walk tests senders
	// against. Unexported and set by the store beside the transaction it reads
	// in, never by a caller: it is one read's snapshot, not a request parameter,
	// and a caller supplying it could widen or narrow who counts as a colleague.
	ownDomains []string
	// horizonDays is how far back a wait reaches and still counts, derived from
	// this installation's own response spread (waitinghorizon.go). Unexported
	// and set beside the transaction it was measured in, for ownDomains' exact
	// reason: it is one read's snapshot rather than a request parameter, and a
	// caller supplying it could widen or narrow what the queue calls work.
	//
	// Zero means unmeasured, which the clause reads as the compiled default —
	// so a caller with no seam to measure through gets today's behaviour rather
	// than a horizon of nothing.
	horizonDays int
	// OpenAndDueBy narrows to tasks still open and already due at an instant:
	// the day's work, asked as one question.
	//
	// It exists because the caller that wants this cannot express it any other
	// way. Reading a page by recency and dropping the finished rows afterwards
	// puts the bound on the WRONG set — a pile of completed tasks fills the
	// page, the one overdue promise never reaches the reader, and the day
	// renders clear while the work is still there. A limit is only honest over
	// the rows that qualify, which means the test belongs here.
	//
	// A task with no due date is excluded: it is agreed work, but it is not
	// work for a given instant, and a queue that promised today's list would be
	// lying if it carried the undated backlog too.
	OpenAndDueBy *time.Time
	// OpenAndDueAfter narrows the same read to work due LATER than an instant,
	// and is paired with OpenAndDueBy to ask for one window.
	//
	// It exists so "what is coming" can be a separate bounded read from "what
	// is due today", rather than one wider read split afterwards in Go. Split
	// afterwards, a full day's backlog fills the limit before a single upcoming
	// row is reached, and the reader who most needs to see next week's deadline
	// is exactly the reader who never does.
	OpenAndDueAfter *time.Time
}
