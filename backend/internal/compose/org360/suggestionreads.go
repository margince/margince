// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

// What the suggestion rules read, and why they do not read the nested
// sections above them.
//
// The 360's collections are TRUNCATED SUMMARIES capped at sectionLimit — the
// card shows the newest 25 and links to the dedicated endpoint for the rest.
// A rule derived from that page answers a different question than the one it
// claims to: an account whose newest 25 timeline entries are all notes would
// miss an overdue unanswered email underneath them, and its 26th open deal
// would never be reported as stalled. A rep would read that as "nothing to
// chase here", which is the one thing the surface must never say wrongly.
//
// So each rule reads exactly what it needs, under the SAME row-scope
// predicates the sections use — the caller cannot see more this way, only
// further back, and nothing here reads an assembled section at all.
//
// Every figure they state covers the whole visible set, and comes from ONE read
// of it. A count bounded by its own fetch is one a rep cannot tell from a real
// one, and two reads of the same pipeline can disagree — the 360's as_of
// promises one instant, so the rules take one snapshot.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lastMessage is the newest two-way exchange on an account, as the no-reply
// rule needs it: who spoke, when, and which activity to cite.
type lastMessage struct {
	ID        ids.UUID
	Direction string
	At        time.Time
}

// newestMessage reads the newest two-way exchange linked to the account, or
// reports that there is none.
//
// The kind set is every channel an answer can ARRIVE on, which is wider than
// the set we send on. A returned call or a meeting answers an email as
// completely as a reply does, so leaving them out would tell a rep to chase
// someone they spoke to yesterday. A note or a task is excluded for the
// opposite reason: it is something we wrote to ourselves, nobody owes a reply
// to it, and letting one count would silence the rule every time a rep left
// themselves a reminder.
//
// Reachability is activities.OrgLinkedActivityExists, the walk every reader of
// the account's timeline uses — one spelling, so they cannot drift. Every
// candidate still passes the activity row scope, so the reader can open it.
func newestMessage(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
) (lastMessage, bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	activityScope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return lastMessage{}, false, err
	}
	if activityScope == "" {
		activityScope = scopeAll
	}
	var found lastMessage
	var direction *string
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT a.id, a.direction, a.occurred_at
		FROM activity a
		WHERE a.kind IN ('email','message','call','meeting') AND a.archived_at IS NULL AND %[1]s
		  AND %[2]s
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT 1`, activityScope, activities.OrgLinkedActivityExists(orgPos)), args...).
		Scan(&found.ID, &direction, &found.At)
	if errors.Is(err, pgx.ErrNoRows) {
		return lastMessage{}, false, nil
	}
	if err != nil {
		return lastMessage{}, false, fmt.Errorf("read the account's newest message: %w", err)
	}
	if direction != nil {
		found.Direction = *direction
	}
	return found, true, nil
}

// hasOpenTask answers whether anything at all is scheduled on the account.
//
// The rule needs "is there one?", so that is what is asked. Reading the
// next-steps page instead would answer the same question correctly today —
// truncation only hides rows past the first 25 — while coupling the rules to a
// section, which is the coupling the whole file exists to remove.
//
// Reachability is activities.OrgLinkedActivityExists, the same walk
// nextStepsSection uses.
func hasOpenTask(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	activityScope, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return false, err
	}
	if activityScope == "" {
		activityScope = scopeAll
	}
	var scheduled bool
	err = tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS (
		  SELECT 1 FROM activity a
		  WHERE a.kind = 'task' AND NOT a.is_done AND a.archived_at IS NULL AND %[1]s
		    AND %[2]s)`,
		activityScope, activities.OrgLinkedActivityExists(orgPos)), args...).Scan(&scheduled)
	if err != nil {
		return false, fmt.Errorf("read whether anything is scheduled on the account: %w", err)
	}
	return scheduled, nil
}

// suggestionInputs is everything the rules read, gathered once.
//
// Both callers build from this same struct — the composite read that serves the
// card, and the dismissal that has to recognize what the card served. Two
// gatherers would let the two disagree about what a suggestion IS, and then a
// dismissal would silently match nothing.
type suggestionInputs struct {
	// timeline and pipeline are the two grants that decide which rules run at
	// all. They are the object grants, which is exactly what makes a 360 section
	// absent — row scope narrows a section, it does not withhold it.
	timeline bool
	pipeline bool
	// contracts is gated INDEPENDENTLY of pipeline: a role may read deals and
	// not the agreements behind them, and folding the two would answer a
	// question this reader has no standing to ask.
	contracts bool

	newest    lastMessage
	hasNewest bool
	// lifecycle is the stage the record claims. It comes from the organization
	// row the assembly already holds, not from a read of its own.
	lifecycle string
	// orgName is the account as its record names it, and it is only ever read
	// by the step a suggestion PREPARES: a task lands in a queue where this
	// page is not on screen, so "Agree the next step" alone would name nothing
	// its owner could place. It reaches the fingerprint through nothing.
	orgName string
	// contractEnded is whether the account's own correspondence says the
	// relationship is over. Filled from the shared signal read, so the
	// contradiction rule and the health section count one query between them.
	contractEnded bool
	open          pipeline
	contractStrip contractStrip
	scheduled     bool
}

// advisable reports whether this caller can be advised at all. Neither input
// means nothing to derive advice from, so the section is omitted and named
// rather than answering empty.
func (in suggestionInputs) advisable() bool { return in.timeline || in.pipeline }

// granted answers whether this caller may read one object, distinguishing a
// refusal from a broken context.
//
// Collapsing both into a bool would turn "no actor bound" — a programming error —
// into a quietly withheld section on a 200, while every other section in the same
// assembly surfaces it as a failure. The spelling here matches dealStageMoves in
// viewbaseline.go: the sentinel is a decision, anything else is a bug.
func granted(ctx context.Context, object string) (bool, error) {
	err := auth.Require(ctx, object, principal.ActionRead)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	return false, err
}

// gatherSuggestionInputs reads what the rules need, skipping whatever this
// caller has no grant for.
// facts and heading are passed in rather than read here, because the page
// already holds both and a 360 that re-read them would pay for the same rows
// twice.
//
// They are REQUIRED parameters, not optional fill-in: every input the
// contradiction rule reads must be supplied by every caller, or the page and
// the dismissal that answers it judge different suggestions and a dismissal
// stores nothing. Required, a caller that omits one does not compile.
func gatherSuggestionInputs(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID, now time.Time,
	facts signalFacts, heading organizationHeading, baseCurrency string,
) (suggestionInputs, error) {
	timeline, err := granted(ctx, "activity")
	if err != nil {
		return suggestionInputs{}, err
	}
	pipeline, err := granted(ctx, "deal")
	if err != nil {
		return suggestionInputs{}, err
	}
	contractsGranted, err := granted(ctx, "contract")
	if err != nil {
		return suggestionInputs{}, err
	}
	in := suggestionInputs{timeline: timeline, pipeline: pipeline, contracts: contractsGranted}
	if in.contracts {
		strip, err := readContractStrip(ctx, tx, orgID, now, baseCurrency)
		if err != nil {
			return suggestionInputs{}, err
		}
		in.contractStrip = strip
	}
	if in.timeline {
		newest, found, err := newestMessage(ctx, tx, orgID)
		if err != nil {
			return suggestionInputs{}, err
		}
		in.newest, in.hasNewest = newest, found
		scheduled, err := hasOpenTask(ctx, tx, orgID)
		if err != nil {
			return suggestionInputs{}, err
		}
		in.scheduled = scheduled
	}
	if in.pipeline {
		open, err := openPipeline(ctx, tx, orgID, now, baseCurrency)
		if err != nil {
			return suggestionInputs{}, err
		}
		in.open = open
	}
	in.contractEnded = facts.ContractEnded
	in.lifecycle = heading.lifecycle
	in.orgName = heading.name
	return in, nil
}

// organizationHeading is what the rules quote off the account's own row: the
// stage it claims, and the name it goes by. Together because both callers need
// both and a second read for the name would be a second instant.
type organizationHeading struct {
	lifecycle string
	name      string
}

// readOrganizationHeading takes it from the organization row, for the caller
// that has no assembled page to read it off — so the contradiction rule and the
// step a suggestion prepares come out the same way for the page and for the
// dismissal that answers it.
func readOrganizationHeading(
	ctx context.Context, tx pgx.Tx, orgID ids.OrganizationID,
) (organizationHeading, error) {
	var lifecycle *string
	var name string
	if err := tx.QueryRow(ctx,
		`SELECT lifecycle, display_name FROM organization WHERE id = $1`, orgID,
	).Scan(&lifecycle, &name); err != nil {
		return organizationHeading{}, fmt.Errorf("read the account's stage and name: %w", err)
	}
	out := organizationHeading{name: name}
	if lifecycle != nil {
		out.lifecycle = *lifecycle
	}
	return out, nil
}

// engagementState is PO-F-4: whose move is it, from the newest message that can
// answer that question.
//
// The order below IS the spec's evaluation order and the states are mutually
// exclusive — the first match wins, so a silent account reads as dormant rather
// than as whichever side happened to write last a year ago.
//
// waiting_on_them reuses noReplyDays rather than restating it, which is what
// makes the strip and the no_reply suggestion beneath it agree by construction
// instead of by coincidence.
func engagementState(in suggestionInputs, now time.Time) crmcontracts.Organization360StateStripEngagementState {
	if !in.hasNewest {
		return "never_contacted"
	}
	age := now.Sub(in.newest.At)
	switch {
	case age > engagementDormantDays*24*time.Hour:
		return "dormant"
	case in.newest.Direction == "outbound" && age >= noReplyDays*24*time.Hour:
		return "waiting_on_them"
	case in.newest.Direction == "inbound" && age >= noReplyDays*24*time.Hour:
		return "waiting_on_us"
	}
	return "active"
}

// engagementDormantDays (PO-PARAM-4) is where "whose move is it" stops being a
// useful question. Past it the distinction is not actionable, and a strip still
// saying "waiting on them" after a quarter would be advising a reply to a
// conversation nobody remembers.
const engagementDormantDays = 90
