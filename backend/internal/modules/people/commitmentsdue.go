// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The promises WE made that are coming due (ADR-0097 D1), for the attention
// feed's own lane.
//
// WHY THIS READ RATHER THAN THE OPEN-TASK SWEEP. A commitment needs two halves
// on screen: the deadline, and where the promise was made. Only this table
// carries both — an open task has a due date and no provenance, and a
// `capture_label = 'commitment'` email has provenance and no due date. The
// attention feed's `planned` lane already reads open tasks, so a producer over
// those would print the same row twice under two headings.
//
// The claim is the evidence, so the sentence a reader gets is the promise in
// their own words with the message it was read from beside it, rather than a
// task subject somebody retyped.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The bounds one sweep of the lane obeys. A caller that asks for nothing sane
// gets the default rather than a refusal or, worse, an unbounded read: the
// limit is formatted into the statement, so an unchecked zero or negative would
// be a syntax error at the database and an unchecked large one a sweep nobody
// asked for. The lane's own cap is well under the ceiling; the ceiling is here
// so the NEXT caller cannot raise it by accident.
const (
	commitmentsDueDefaultLimit = 12
	commitmentsDueMaxLimit     = 200
)

// commitmentsDueLimit bounds one sweep, the way the open-task read bounds its
// own. Same shape deliberately: two spellings of one rule is how one of them
// ends up without a ceiling.
func commitmentsDueLimit(asked int) int {
	switch {
	case asked <= 0:
		return commitmentsDueDefaultLimit
	case asked > commitmentsDueMaxLimit:
		return commitmentsDueMaxLimit
	default:
		return asked
	}
}

// CommitmentDue is one promise this rep made, with the evidence behind it.
type CommitmentDue struct {
	ID ids.UUID
	// PersonID is who it was promised to, so the surface can route a reader to
	// the record the promise lives on.
	PersonID ids.PersonID
	// Body is the promise in the reader's language; SourceQuote is the
	// verbatim excerpt it was read from. Both travel because the claim
	// contract's whole rule is that a claim is checkable against what was
	// actually written.
	Body        string
	SourceQuote string
	PersonName  string
	// SourceLabel names the conversation in a chip — the thread subject or the
	// meeting title — and OccurredAt is when it was said. Together they are the
	// "on 18 August, in the call" half of the sentence.
	SourceLabel string
	OccurredAt  time.Time
	DueAt       time.Time
}

// OpenCommitmentsDue reads the acting rep's own promises falling due by the
// given instant, most overdue first.
//
// WHOSE PROMISES. A claim carries no assignee of its own, so ownership rides
// the PERSON it was made to: person.owner_id is the rep who holds that
// relationship, and a promise made in their own captured conversation is
// theirs. A claim on an unowned person therefore reaches nobody's lane, which
// is the honest answer — no rep is on the hook for it.
//
// UNDATED CLAIMS ARE EXCLUDED. "Coming due" is unanswerable without a date. An
// undated promise is real work and it is not TODAY's, which is the same call
// the planned lane already makes for an undated task.
//
// The two gates CommitmentsTheirsForProjects keeps, for its reasons. The
// activity join with auth.ActivityContentClause stops a claim outliving or
// outreaching the message it cites. The person row scope is filtered on rather
// than selected here — unlike that read, this one returns a LIST rather than
// one row per project, so an invisible person's claim is simply not this
// caller's to see and dropping it hides nothing they were owed.
//
// status = 'open' AND NOT needs_review, for the reason the sibling read states:
// a settled claim is done with, and a disputed one presented as a fact would
// state as true the very thing the extractor flagged as contested.
func (s *Store) OpenCommitmentsDue(ctx context.Context, owner ids.UserID, by time.Time, limit int) ([]CommitmentDue, error) {
	// The claim names a person and quotes a message, so both objects are
	// required before either row is read.
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []CommitmentDue
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = openCommitmentsDue(ctx, tx, owner, by, limit)
		return err
	})
	return out, err
}

// openCommitmentsDueFrom is the promises this rep owes and the day they are
// owed by, spelled ONCE.
//
// The page and the count that sits beside it share it deliberately: a count
// assembled from a second copy of these arms would answer a different question
// from the cards under it, one arm at a time, and nothing would fail to say so.
//
// %[1]s is the OWNER TEST — `pr.owner_id = $n` for one rep, `= ANY($n)` for a
// board counting several. A fragment rather than a fixed comparison because the
// board would otherwise need its own copy of every arm below to change that one
// term, and the count under a lead's table would drift from the cards a rep
// opens. %[2]d is the bound, %[3]s the activity-content scope and %[4]s the
// person scope.
//
// Held by: TestTheCommitmentCountAnswersTheSameQuestionAsThePage
// (backend/internal/compose/integration/commitmentlane_integration_test.go)
const openCommitmentsDueFrom = `
		  FROM conversation_claim c
		  JOIN activity a ON a.id = c.source_activity_id AND a.archived_at IS NULL
		  JOIN person pr ON pr.id = c.person_id AND pr.archived_at IS NULL
		 WHERE c.kind = 'commitment_ours' AND c.status = 'open' AND NOT c.needs_review
		   AND c.archived_at IS NULL
		   -- STRICTLY before, which is what the caller's bound means: it is the
		   -- END of the day, so an inclusive test put a promise due at exactly
		   -- tomorrow's midnight on today's list — reported late a day early,
		   -- and again tomorrow. The task lane beside this one reads it the
		   -- same way, and the two decide the same afternoon.
		   AND c.due_at IS NOT NULL AND c.due_at < $%[2]d
		   AND %[1]s
		   AND (%[3]s) AND (%[4]s)`

// CountOpenCommitmentsDue answers how many promises are due by an instant, with
// no cap on it — the badge beside the lane's bounded page.
//
// Same gates as the page, in the same order: the claim names a person and quotes
// a message, so both objects are required before either row is counted, and the
// row scopes narrow the count exactly as they narrow the list. A number that
// moved when a colleague captured a contact this reader may not see would
// disclose that contact.
func (s *Store) CountOpenCommitmentsDue(ctx context.Context, owner ids.UserID, by time.Time) (int, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return 0, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return 0, err
	}
	var total int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		ownerPos := arg(owner)
		byPos := arg(by)
		activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		if activityScope == "" {
			activityScope = sqlAlwaysVisible
		}
		personScope, err := auth.ScopeClauseFor(ctx, "person", "pr", arg)
		if err != nil {
			return err
		}
		if personScope == "" {
			personScope = sqlAlwaysVisible
		}
		return tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*)`+openCommitmentsDueFrom,
			fmt.Sprintf("pr.owner_id = $%d", ownerPos), byPos, activityScope, personScope),
			args...).Scan(&total)
	})
	if err != nil {
		return 0, fmt.Errorf("count the rep's commitments coming due: %w", err)
	}
	return total, nil
}

func openCommitmentsDue(
	ctx context.Context, tx pgx.Tx, owner ids.UserID, by time.Time, limit int,
) ([]CommitmentDue, error) {
	var out []CommitmentDue
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ownerPos := arg(owner)
	byPos := arg(by)
	activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if activityScope == "" {
		activityScope = sqlAlwaysVisible
	}
	personScope, err := auth.ScopeClauseFor(ctx, "person", "pr", arg)
	if err != nil {
		return nil, err
	}
	if personScope == "" {
		personScope = sqlAlwaysVisible
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT c.id, c.person_id, c.body, c.source_quote,
		       coalesce(pr.full_name, ''), coalesce(a.subject, ''),
		       a.occurred_at, c.due_at`+openCommitmentsDueFrom+`
		 ORDER BY c.due_at ASC, c.id
		 LIMIT %[5]d`,
		fmt.Sprintf("pr.owner_id = $%d", ownerPos), byPos, activityScope, personScope,
		commitmentsDueLimit(limit)), args...)
	if err != nil {
		return nil, fmt.Errorf("read the rep's commitments coming due: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var due CommitmentDue
		if err := rows.Scan(&due.ID, &due.PersonID, &due.Body, &due.SourceQuote,
			&due.PersonName, &due.SourceLabel, &due.OccurredAt, &due.DueAt); err != nil {
			return nil, fmt.Errorf("scan a commitment coming due: %w", err)
		}
		out = append(out, due)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the rep's commitments coming due: %w", err)
	}
	return out, nil
}

// CountOpenCommitmentsDueByOwner answers the same question as
// CountOpenCommitmentsDue for several owners at once, for the team board.
//
// ONE query rather than one per teammate. A board is capped at a hundred
// people, and a hundred sequential round trips on a surface a lead opens every
// morning is a slow page for no reason — worse, each ran in its own transaction,
// so a person's records changing hands mid-loop could have them counted twice or
// not at all. This reads one snapshot.
//
// It shares openCommitmentsDueFrom with the single-owner count and the page, so
// what a lead's column totals is what that rep's own cards show. Owners not
// named in the result owe nothing; the caller fills the zero.
func (s *Store) CountOpenCommitmentsDueByOwner(
	ctx context.Context, owners []ids.UserID, by time.Time,
) (map[ids.UUID]int, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	per := map[ids.UUID]int{}
	if len(owners) == 0 {
		return per, nil
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		ownersPos := arg(owners)
		byPos := arg(by)
		activityScope, err := auth.ActivityContentClause(ctx, "a", arg)
		if err != nil {
			return err
		}
		if activityScope == "" {
			activityScope = sqlAlwaysVisible
		}
		personScope, err := auth.ScopeClauseFor(ctx, "person", "pr", arg)
		if err != nil {
			return err
		}
		if personScope == "" {
			personScope = sqlAlwaysVisible
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT pr.owner_id, count(*)`+
			openCommitmentsDueFrom+` GROUP BY pr.owner_id`,
			fmt.Sprintf("pr.owner_id = ANY($%d)", ownersPos), byPos, activityScope, personScope),
			args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var owner ids.UUID
			var due int
			if err := rows.Scan(&owner, &due); err != nil {
				return err
			}
			per[owner] = due
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("count the team's commitments coming due: %w", err)
	}
	return per, nil
}
