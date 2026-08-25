// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// The gather half of the coverage read: the facts the risk rules decide on.
//
// Split from the fold so the thresholds can be tested against hand-built
// numbers with no database, and so every fact the rules see is read at ONE
// instant inside ONE transaction — a deal whose status came from a later
// snapshot than its last touch could report a won deal as going cold.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	peoplemod "github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/platform/auth"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/idlebase"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// DealStatusOpen is the status the pipeline rules apply to. Named rather than
// inline because several callers ask for it — including the at-risk sweep in
// compose — and a literal in each place is a second definition waiting to
// drift.
const DealStatusOpen = "open"

// dealStatusOpen is this package's own spelling of the same constant, so the
// rules below read at their own level rather than reaching for the exported
// name they define.
const dealStatusOpen = DealStatusOpen

// scopeAll is the clause an UNBOUNDED caller gets. auth answers "" for a caller
// bounded by nothing, and an empty string interpolated into a WHERE reads as a
// syntax error rather than as "no restriction".
const scopeAll = "true"

// dealFacts is the deal's own row, as the risk rules need it.
type dealFacts struct {
	status         string
	organizationID ids.UUID
	lastTouchAt    time.Time
	// everTouched says an activity has actually been captured against the
	// deal, which lastTouchAt cannot answer: it coalesces to the creation
	// date, so a deal nobody has contacted and one contacted the day it was
	// written down carry the same instant.
	everTouched bool
}

// readDealFacts loads the deal row the rules decide on.
//
// The last touch is idlebase.SQL — the same fallback the stalled rule
// measures from, not a second coalesce that agrees with it by inspection.
func readDealFacts(ctx context.Context, tx pgx.Tx, dealID ids.DealID) (dealFacts, error) {
	var out dealFacts
	var org *ids.UUID
	err := tx.QueryRow(ctx, fmt.Sprintf(`
		SELECT status, organization_id, %s,
		       last_activity_at IS NOT NULL
		  FROM deal WHERE id = $1`, idlebase.SQL("")), dealID).Scan(&out.status, &org, &out.lastTouchAt, &out.everTouched)
	if err != nil {
		return out, fmt.Errorf("network: reading the deal a coverage view describes: %w", err)
	}
	if org != nil {
		out.organizationID = *org
	}
	return out, nil
}

// readDeparted answers which of the deal's stakeholders have LEFT the account.
//
// The test demands evidence of a departure, not merely absence of an
// employment. Most stakeholders have no employment row on file at all, and
// treating "we never recorded where they work" as "they left" would put a
// departure flag on nearly every deal in a young workspace — a warning that is
// always on is a warning nobody reads.
//
// So a person qualifies only when BOTH halves hold: an employment at this
// account with an end date that has PASSED, and no live employment there now. A
// contract renewed after a gap, or a role change recorded as end-then-start,
// leaves a live row and correctly raises nothing.
//
// An ARCHIVED employment is not evidence of leaving, and reading it as such was
// wrong. Archiving retracts a statement — somebody recorded the job by mistake
// — while an end date records a fact about the world. Announcing a resignation
// because a colleague fixed a data-entry error is exactly the false alarm that
// teaches a rep to ignore the flag.
//
// "Still employed there" is people.EmploymentIsCurrentSQL, and both halves of
// this statement are written from it: the live half asserts it, the departed
// half is its NEGATION. Spelled that way rather than as an equivalent
// hand-written `ended_at IS NOT NULL AND ended_at <= today`, because the
// comment used to promise the two questions "must not disagree about the same
// row" and nothing held the promise — this file, introseams.go and
// linkedinmatch.go each carried their own copy, and each one was a chance for
// them to drift apart.
//
// It also removed a clock. The departed half compared against a Go time.Time
// while the live half compared against Postgres' current_date, so one statement
// asked its two questions on two different days whenever the server and the
// database disagreed about the date — which is precisely what
// EmploymentIsCurrentSQL's own comment says the predicate exists to prevent.
//
// No person visibility probe: the caller passes the stakeholder ids it already
// read under its own person row scope, so a seat this caller cannot see never
// reaches here. Re-probing would be a second enforcement of the same rule with
// its own way of being wrong.
//
// The EDGE gate is a different rule and is taken here. A departure IS an
// employment edge — "this person no longer works at Acme" is a fact about the
// pair — and CoverageFor only reaches this after the seat read passed the same
// gate, so in practice it admits. It is taken anyway because nothing structural
// stops a future caller arriving another way, and a read whose safety rests on
// the order its package happens to call things in is one refactor from
// disclosing.
func readDeparted(ctx context.Context, tx pgx.Tx, orgID ids.UUID, people []ids.UUID) ([]ids.UUID, error) {
	if orgID == ids.Nil || len(people) == 0 {
		return nil, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos, peoplePos := arg(orgID), arg(people)
	edgeBound, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, err
	}
	if edgeBound == "" {
		edgeBound = scopeAll
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT r.person_id
		  FROM relationship r
		 WHERE r.kind = 'employment'
		   AND r.organization_id = $%[1]d
		   AND r.person_id = ANY($%[2]d)
		   AND r.archived_at IS NULL
		   AND NOT `+peoplemod.EmploymentIsCurrentSQL("r.ended_at")+`
		   AND (%[3]s)
		   AND NOT EXISTS (
		       SELECT 1 FROM relationship live
		        WHERE live.kind = 'employment'
		          AND live.organization_id = r.organization_id
		          AND live.person_id = r.person_id
		          AND live.archived_at IS NULL
		          AND `+peoplemod.EmploymentIsCurrentSQL("live.ended_at")+`)
		 ORDER BY r.person_id`, orgPos, peoplePos, edgeBound), args...)
	if err != nil {
		return nil, fmt.Errorf("network: reading which stakeholders have left the account: %w", err)
	}
	defer rows.Close()
	var out []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
