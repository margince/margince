// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// review_commitments: the open promises this workspace has made, earliest due
// date first, each with the person who owes it and the record it was made
// about.
//
// EARLIEST DUE, not oldest promise. The order is the due date ascending with
// undated last — a promise made a year ago and never dated sorts behind one
// made this morning for tomorrow, because what a reviewer is chasing is the
// date that has passed rather than the day the words were said.
//
// A promise here is a TASK ACTIVITY that nobody has ticked off. That is the
// whole definition, and it is the tool's honesty problem: a commitment made
// out loud in a meeting and never written down is not in this answer, and a
// model told nothing would report the list as "what we owe" rather than as
// "what we recorded that we owe". The description says so, and the tool
// refuses to imply more than the rows support.
//
// THE TOOL IS CLOCK-FREE. The seam stamps the instant it swept at, and
// everything below is a pure function of (due date, that instant) — which is
// what makes the state of every row reproducible in a test without a real
// clock, and what stops two rows in one answer being judged against two
// different "nows".

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/kernel/deadline"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// CommitmentAbout is one record an open promise was made about.
type CommitmentAbout struct {
	EntityType string   `json:"entity_type"`
	EntityID   ids.UUID `json:"entity_id"`
	// Name is what a human calls that record. Empty where the record has no
	// name of its own — a lead captured as an email address and nothing else
	// — which a reader shows as the id rather than as a blank.
	Name string `json:"name,omitempty"`
}

// OpenCommitment is one outstanding promise as the seam read it, from either
// of the two places a promise gets written down.
//
// EXACTLY ONE OF TaskID AND ClaimID. A promise somebody typed is a task row; a
// promise an extractor read out of a conversation is a claim row, and the two
// are unlinked — nothing writes conversation_claim.task_activity_id — so one
// promise recorded both ways arrives here as two. That is the honest answer
// until the link is written; guessing which pairs mean one promise would be
// this surface inventing a fact.
type OpenCommitment struct {
	// Source is `task` or `conversation`, and says which of the two ids below
	// is set.
	Source string
	// TaskID is the task activity, for a task-sourced promise.
	TaskID *ids.UUID
	// ClaimID is the extracted commitment, and SourceActivityID is the message
	// it was read from — which is what a reader opens to check it.
	ClaimID          *ids.UUID
	SourceActivityID *ids.UUID
	// Quote is the sentence the promise was made in. Claims only: a task
	// carries what somebody retyped and has nothing to quote.
	Quote        string
	Subject      string
	DueAt        *time.Time
	AssigneeID   *ids.UUID
	AssigneeName string
	About        []CommitmentAbout
}

// The two places a promise is recorded, as this surface names them.
const (
	// CommitmentFromTask is a promise somebody typed as a task.
	CommitmentFromTask = "task"
	// CommitmentFromConversation is a promise an extractor read out of a
	// captured conversation and nobody typed.
	CommitmentFromConversation = "conversation"
)

// CommitmentSweep is ONE reading of the open-promise set: the rows, the
// instant they are judged against, and whether the sweep stopped at its
// bound.
//
// AsOf travels with the rows rather than being taken by the tool, so the
// answer states the instant its own states were derived from. A reader
// handed states without the instant cannot tell a stale answer from a fresh
// one, and "overdue" is a claim that is only true relative to a moment.
type CommitmentSweep struct {
	AsOf        time.Time
	Commitments []OpenCommitment
	Truncated   bool
}

// CommitmentQuery narrows one sweep to what the caller asked for.
type CommitmentQuery struct {
	AssigneeID *ids.UUID
	// WithinProjectID keeps the promises filed under this project or under
	// none; ones filed under another project drop out.
	WithinProjectID *ids.UUID
	Limit           int
}

// CommitmentLister serves the row-scoped open-promise set. Compose
// implements it over the activities module's own gated read, so RBAC and the
// timeline's row scope apply exactly as they do on the HTTP surface.
type CommitmentLister func(ctx context.Context, in CommitmentQuery) (CommitmentSweep, error)

// RegisterCommitmentTool wires the open-promise review. No lister, no tool: a
// surface that cannot ground its answer does not pretend to.
func RegisterCommitmentTool(r *Registry, list CommitmentLister) {
	if list == nil {
		return
	}
	r.Register(reviewCommitments{list: list})
}

// The states one promise can be in, and the whole vocabulary of them.
//
// THERE IS NO "DUE TODAY". A calendar day is a claim that needs a timezone,
// and this build stores none — not on the workspace, not on the user — so a
// bucket named for one would be UTC's day wearing the reader's name. The
// three states below need no calendar: a promise either has no date, has
// passed its date, or has not. The exact due date rides alongside for a
// reader who has a timezone of their own.
const (
	commitmentUndated  = "undated"
	commitmentOverdue  = "overdue"
	commitmentUpcoming = "upcoming"
)

// commitmentsTruncatedMessage is the same rule the other bounded reads on
// this surface state: a set that stopped at its cap is not the whole set, and
// a model told nothing reports it as one. It matters more here than most,
// because the question this tool answers is "is anything being dropped".
const commitmentsTruncatedMessage = "More open commitments exist than are listed here. " +
	"Report these as the soonest-due promises found, not as everything outstanding."

// maxCommitments bounds one call. It is the schema's maximum and the
// server-side ceiling both, so a caller that omits the argument gets a
// bounded read rather than the whole table.
const maxCommitments = 50

type reviewCommitments struct{ list CommitmentLister }

func (t reviewCommitments) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "review_commitments", Title: "Review open commitments", Version: toolVersionV1,
		Description:   reviewCommitmentsCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "listActivities",
		InputSchema: schema(`{"type":"object","properties":{
			"assignee_id":{"type":"string","format":"uuid","description":"Narrow to one owner's promises; omit for everyone's"},
			"project_id":{"type":"string","format":"uuid","description":"Keep only promises filed under this project or under none"},
			"limit":{"type":"integer","minimum":1,"maximum":50,"description":"Cap the set; omit for 50, the server-side ceiling"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ReviewCommitmentsResult](),
		// The view renders the same answer as a dated queue. What it buys over
		// the text is the shape of the backlog at a glance — how far past due
		// the soonest-due promises are, and which of them nobody owns.
		UI: &mcp.ToolUI{ResourceURI: apps.CommitmentsURI},
	}
}

func (t reviewCommitments) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		AssigneeID *ids.UUID `json:"assignee_id"`
		ProjectID  *ids.UUID `json:"project_id"`
		Limit      int       `json:"limit"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := requireCommitmentLimit(args.Limit); err != nil {
		return nil, err
	}
	sweep, err := t.list(ctx, CommitmentQuery{AssigneeID: args.AssigneeID, WithinProjectID: args.ProjectID, Limit: args.Limit})
	if err != nil {
		return nil, err
	}
	// The items carry task subjects and the names of the records they are
	// about, read off rows this call does not hand over — so the answer is
	// tainted with their content.
	noteDerivedContent(ctx)
	items := make([]CommitmentItem, 0, len(sweep.Commitments))
	for _, c := range sweep.Commitments {
		item := c.wire(sweep.AsOf)
		noteCommitmentEvidence(ctx, item)
		items = append(items, item)
	}
	if sweep.Truncated {
		noteWarning(ctx, warningSweepTruncated, commitmentsTruncatedMessage)
	}
	return json.Marshal(ReviewCommitmentsResult{AsOf: sweep.AsOf, Commitments: items})
}

// requireCommitmentLimit refuses a limit the schema already forbids. The
// schema is what a well-behaved client reads; this is what holds for one that
// did not, and it names the bound rather than restating the shape.
func requireCommitmentLimit(limit int) error {
	if limit >= 0 && limit <= maxCommitments {
		return nil
	}
	return &BadArgsError{
		Cause:    fmt.Errorf("limit %d is outside the range this tool serves", limit),
		Guidance: fmt.Sprintf("omit it, or ask for 1..%d", maxCommitments),
	}
}

// wire turns one swept promise into the item a caller is served, with the
// state it is in as of the sweep's own instant.
func (c OpenCommitment) wire(asOf time.Time) CommitmentItem {
	item := CommitmentItem{
		TaskID: c.TaskID, ClaimID: c.ClaimID, Source: c.Source,
		SourceActivityID: c.SourceActivityID,
		Quote:            c.Quote, Subject: c.Subject, DueAt: c.DueAt,
		State: commitmentState(c.DueAt, asOf),
		// Never null: a model handed null reads it as "unknown" where an empty
		// array says "this promise names no record".
		About: c.About,
	}
	if item.About == nil {
		item.About = []CommitmentAbout{}
	}
	if c.AssigneeID != nil {
		item.AssigneeID = c.AssigneeID
		item.AssigneeName = c.AssigneeName
	}
	if days, overdue := daysOverdue(c.DueAt, asOf); overdue {
		item.DaysOverdue = &days
	}
	return item
}

// commitmentState judges one promise against the instant the set was swept at.
//
// The boundary is deadline.Passed's, so this surface and every list, card and
// figure the same person can open agree about the same promise.
func commitmentState(dueAt *time.Time, asOf time.Time) string {
	switch {
	case dueAt == nil:
		return commitmentUndated
	case deadline.Passed(dueAt, asOf):
		return commitmentOverdue
	default:
		return commitmentUpcoming
	}
}

// daysOverdue answers how many WHOLE days a promise has been past its date, and
// whether it is past it at all.
func daysOverdue(dueAt *time.Time, asOf time.Time) (int, bool) {
	return deadline.DaysPast(dueAt, asOf)
}

// noteCommitmentEvidence records the row a promise was read from, whichever
// kind it is.
//
// Both point at an activity: a task IS an activity row, and a claim quotes the
// message it was extracted from. So the caller's evidence trail lands on
// something they can open either way, which is what the trail is for.
func noteCommitmentEvidence(ctx context.Context, item CommitmentItem) {
	switch {
	case item.TaskID != nil:
		noteEvidence(ctx, datasource.EntityActivity, *item.TaskID)
	case item.SourceActivityID != nil:
		noteEvidence(ctx, datasource.EntityActivity, *item.SourceActivityID)
	}
}
