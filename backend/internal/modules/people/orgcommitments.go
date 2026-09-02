// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The promises WE made to an account's people, for the company page's own
// moment card.
//
// WHY AN ACCOUNT READ AND NOT THE PERSON ONE REPEATED. A claim names a PERSON,
// never a company, so an account's promises are its contacts' promises
// gathered up. Asking the per-person read once per contact would be one query
// per row on a page whose cost may not grow with the size of the account, and
// it would give each contact its own bound — an account with forty people
// could hold forty capped lists and still not have the promise that matters.
//
// The employment edge is what makes a person this account's: the same edge the
// company page already draws its contact list from, so the promises on the
// card belong to the people listed beneath it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// orgCommitmentsCap bounds one account's sweep. A card names one promise and
// the rung ranks over the set, so the cap has to be wide enough that the
// ranking is not decided by where the read stopped.
const orgCommitmentsCap = 200

// OrgCommitment is one promise this workspace made to somebody at an account.
type OrgCommitment struct {
	// ID is the claim's own identity, which is what tells two promises read out
	// of ONE message apart. They share a source row and a moment, so anything
	// keyed on the evidence alone collides.
	ID ids.UUID
	// PersonID and PersonName are who it was promised to, so the card can name
	// them and route a reader to the record it lives on.
	PersonID   ids.PersonID
	PersonName string
	// Body is the promise; SourceQuote is the sentence it was read from, and
	// ActivityID is the message a reader opens to check it.
	Body        string
	SourceQuote string
	ActivityID  ids.UUID
	// DueAt is nil where the promise carries no date. Undated work is real and
	// is not yet late, which is the ranking's business rather than this read's.
	DueAt *time.Time
	// OccurredAt is when it was said, which breaks ties between two promises
	// sharing a due date.
	OccurredAt time.Time
}

// OpenCommitmentsForOrganization reads the open promises made to the people
// currently employed at one account.
//
// TWO GRANTS, because the row carries two things. The claim quotes a captured
// message and names a person, and a caller holding neither object may have
// neither half. Row scope alone would not stop that: auth.ScopeClauseFor
// narrows a set an object grant has already opened and returns no predicate at
// all for an unbounded actor.
//
// COMPLETENESS IS REPORTED. A promise dropped because its person sits outside
// the caller's row scope would leave the card silent, and a silent card reads
// as "this account owes nothing" — the one thing it must never say by
// accident. The caller is told instead, the way the sibling project read tells
// it.
func (s *Store) OpenCommitmentsForOrganization(
	ctx context.Context, tx pgx.Tx, orgID ids.UUID, limit int,
) ([]OrgCommitment, bool, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, false, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, false, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	orgPos := arg(orgID)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, false, err
	}
	if activityScope == "" {
		activityScope = sqlAlwaysVisible
	}
	personScope, err := auth.ScopeClauseFor(ctx, "person", "pr", arg)
	if err != nil {
		return nil, false, err
	}
	if personScope == "" {
		personScope = sqlAlwaysVisible
	}
	// The employment edge decides which promises are this account's, and the
	// row it yields NAMES the person it was promised to. That is an endpoint
	// pair — this person works here — which relationship.read governs on its
	// own; the person and activity grants above do not cover it.
	edgeScope, err := auth.EdgeReadScope(ctx, "r", arg)
	if err != nil {
		return nil, false, err
	}
	if edgeScope == "" {
		edgeScope = sqlAlwaysVisible
	}
	// Person scope FILTERS here rather than being projected beside each row.
	// The edge gate above already excludes a promise whose person this caller
	// may not see — auth.RelationshipEndpointScope requires the endpoint be
	// visible — so a projected "visible" column would be true on every row it
	// ever saw, and would read like a safety net that has never fired.
	// Filtering says the same thing honestly; the count below is what tells the
	// caller something was withheld.
	//
	// THE ORDER IS THE RANKING'S, NOT A DISPLAY'S, because the LIMIT below
	// decides what the ranking can see. Overdue first, latest slip at the top
	// — which is the promise the card names — then the rest by nearest
	// deadline. Ordered the other way, an account owing more than one page of
	// late promises would truncate away the one that slipped yesterday and
	// keep the ancient ones, so the card would name the least recoverable
	// promise on the record and the read would look like it worked.
	//
	// `now()` rather than a bound instant: this clause only SEPARATES late
	// from not-late for the sort, and the caller re-judges every row against
	// one instant of its own. A row landing on the wrong side of the split at
	// the boundary changes its position in an over-long list, never the
	// verdict shown.
	//
	// DISTINCT ON (c.id) is not needed — a claim is one row — but a person
	// with two live employment edges at one account would duplicate it, so the
	// edge is existence-tested rather than joined.
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.person_id, coalesce(pr.full_name, ''), c.body, c.source_quote,
		       c.source_activity_id, c.due_at, a.occurred_at,
		       count(*) OVER () AS admitted
		  FROM conversation_claim c
		  JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		  JOIN person pr ON pr.id = c.person_id AND pr.archived_at IS NULL
		 WHERE c.kind = 'commitment_ours' AND c.status = 'open' AND NOT c.needs_review
		   AND c.archived_at IS NULL
		   AND EXISTS (
		         SELECT 1 FROM relationship r
		          WHERE r.person_id = pr.id AND r.kind = 'employment'
		            AND r.organization_id = $%[1]d
		            AND `+EmploymentIsCurrentSQL("r.ended_at")+` AND r.archived_at IS NULL
		            AND (%[4]s))
		   AND (%[3]s) AND (%[2]s)
		 ORDER BY (c.due_at IS NOT NULL AND c.due_at < now()) DESC,
		          CASE WHEN c.due_at < now() THEN c.due_at END DESC,
		          c.due_at ASC NULLS LAST, a.occurred_at ASC, c.id
		 LIMIT %[5]d`,
		orgPos, personScope, activityScope, edgeScope, orgCommitmentsLimit(limit)), args...)
	if err != nil {
		return nil, false, fmt.Errorf("read the account's open commitments: %w", err)
	}
	defer rows.Close()

	var out []OrgCommitment
	admitted := 0
	for rows.Next() {
		var row OrgCommitment
		if err := rows.Scan(&row.ID, &row.PersonID, &row.PersonName, &row.Body, &row.SourceQuote,
			&row.ActivityID, &row.DueAt, &row.OccurredAt, &admitted); err != nil {
			return nil, false, fmt.Errorf("scan an account commitment: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("read the account's open commitments: %w", err)
	}
	// COMPLETENESS IS A SECOND COUNT, not a flag on a row. The scope clauses
	// above sit inside the query's own EXISTS and WHERE, so a promise this
	// caller may not see never becomes a row here — there is nothing to mark.
	// Projecting a "visible" column beside each row would compile, return true
	// on every row it ever saw, and read like a safety net that has never once
	// fired.
	//
	// So the read asks the database a second question instead: how many of this
	// account's open promises exist at all. Fewer admitted than exist means
	// something was withheld, and the card can say it is speaking about less
	// than the account rather than reporting silence as "nothing outstanding".
	total, err := countOrgCommitments(ctx, tx, orgID)
	if err != nil {
		return nil, false, err
	}
	return out, admitted >= total, nil
}

// countOrgCommitments is how many open promises this account carries, asked
// WITHOUT the caller's row scope.
//
// Deliberately unscoped, and it returns a NUMBER and nothing else — no name, no
// body, no quote, no id. Comparing it with what the scoped read admitted is the
// only way to tell "this account owes nothing" from "you may not see what it
// owes", and those two must never render the same. The cost is that the number
// weakly reflects that promises exist the reader cannot open, which is exactly
// what the card has to admit to avoid claiming an account is clear.
func countOrgCommitments(ctx context.Context, tx pgx.Tx, orgID ids.UUID) (int, error) {
	var total int
	err := tx.QueryRow(ctx, `
		SELECT count(*)
		  FROM conversation_claim c
		  JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		  JOIN person pr ON pr.id = c.person_id AND pr.archived_at IS NULL
		 WHERE c.kind = 'commitment_ours' AND c.status = 'open' AND NOT c.needs_review
		   AND c.archived_at IS NULL
		   AND EXISTS (
		         SELECT 1 FROM relationship r
		          WHERE r.person_id = pr.id AND r.kind = 'employment'
		            AND r.organization_id = $1
		            AND `+EmploymentIsCurrentSQL("r.ended_at")+` AND r.archived_at IS NULL)`,
		orgID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("count the account's open commitments: %w", err)
	}
	return total, nil
}

// orgCommitmentsLimit bounds one sweep, the way every sibling read bounds its
// own. A caller asking for nothing sane gets the default rather than an
// unbounded read: the limit is formatted into the statement, so an unchecked
// zero would be a syntax error at the database.
func orgCommitmentsLimit(asked int) int {
	if asked <= 0 || asked > orgCommitmentsCap {
		return orgCommitmentsCap
	}
	return asked
}
