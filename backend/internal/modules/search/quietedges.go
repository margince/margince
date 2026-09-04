// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// QuietEdgesForUser answers "which of MY relationships have gone silent",
// oldest silence first.
//
// It is the CANDIDATE half of the decay lane and deliberately not the verdict.
// The projection stores when a pair last spoke, so the reader's own quiet
// relationships are one indexed range over idx_graph_edge_user — but whether a
// silence is worth reporting is a §4 derivation over the contact's own
// interactions, and that stays where it already lives. Answering it here would
// be a second quiet rule, and the two would disagree in front of a rep.
//
// `quietBefore` is the caller's, derived from relstrength.QuietDays: the
// threshold belongs to the derivation, and this read applies it rather than
// holding an opinion about how long is too long. Inclusive on both sides,
// because the derivation admits at `days >= QuietDays`: a strict `<` here would
// drop a contact at exactly the threshold that their own page calls quiet, and
// the two surfaces would disagree for one day about one relationship.
//
// The pair's own silence is not enough to admit a candidate — a contact a
// colleague spoke to last week has been handed over, not lost, and the
// derivation would reject them anyway. The NOT EXISTS spells the derivation's
// person-wide ground here so those rows do not consume LIMIT slots ahead of
// genuine lapses.
//
// That suppression counts only LIVE colleagues, which is why it applies the
// live-member rule under its own alias, through laterMemberJoin. A handover to
// somebody who has since left is not a handover: nobody is holding that
// relationship now, and suppressing it would hide the exact lapse this lane
// exists to surface.
//
// The person row scope is applied AT SOURCE, not left to the caller: this read
// originates the candidate set, an edge row disclosing that a withheld contact
// exists is exactly what capture privacy forbids, and a LIMIT over unscoped
// rows would let unreadable contacts evict readable lapses from the budget.
//
// Established relationships only: a pair with no interaction on record has not
// gone quiet, they were never loud, and admitting them turns every dormant
// contact into an alert. What actually enforces that today is the derivation's
// own nil check on the last interaction — both writers of this projection count
// over rows that must exist, so `count_total > 0` refuses nothing they produce.
// It stays as the predicate a candidate read owes its own table rather than
// borrowing from the verdict, and it is the line to check first if a future
// writer ever inserts a pair before its evidence.
//
// The colleague filter is the live-member join every other read of this table
// carries — a departed colleague's silences are not the reader's work.
// ExcludeEdges renders a predicate that removes candidates before the LIMIT,
// over the alias `e` this projection gives the edge row.
//
// A HOLE rather than a join, because the rows that decide it belong to another
// module and this one never imports a sibling. The caller supplies the SQL and
// the arguments through the same `arg` counter, so the fragment is numbered
// against this statement's own list.
//
// It runs before the cut, which is the whole reason it exists here rather than
// over the returned rows. This read originates the candidate set and takes the
// oldest silences up to a bound; a contact filtered afterwards has already
// spent a slot, so a rep who set several aside would lose real lapses off the
// bottom of their lane and nothing would say so.
type ExcludeEdges func(arg func(any) int) (string, error)

func QuietEdgesForUser(
	ctx context.Context,
	tx pgx.Tx,
	quietBefore time.Time,
	limit int,
	exclude ExcludeEdges,
) ([]InteractionEdge, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return nil, err
	}
	// The reader is taken from the context, never from a parameter. This read
	// answers "which of MY relationships lapsed" and returns who a colleague
	// talks to and when they last did — so a user id off a call site would let
	// any future caller ask that question about somebody else, and the person
	// row scope below would not refuse it: their contacts can be perfectly
	// readable while their private relationship history is not the asker's.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	userPos := arg(actor.UserID)
	beforePos := arg(quietBefore)
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = "TRUE"
	}
	// Nothing excluded is the honest default: a caller with no rows to filter
	// on passes nil and gets every candidate.
	excluded := "TRUE"
	if exclude != nil {
		got, err := exclude(arg)
		if err != nil {
			return nil, err
		}
		if got != "" {
			excluded = got
		}
	}
	limitPos := arg(limit)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT e.user_id, e.person_id, e.last_at, e.last_inbound_at, e.last_outbound_at,
		       e.count_90d, e.in_count_90d, e.out_count_90d, e.count_total
		  FROM graph_interaction_edge e
		  %s
		  JOIN person p ON p.id = e.person_id AND p.archived_at IS NULL
		 WHERE e.user_id = $%d AND e.last_at <= $%d AND e.count_total > 0
		   AND NOT EXISTS (SELECT 1 FROM graph_interaction_edge later
		                     %s
		                    WHERE later.person_id = e.person_id AND later.last_at >= $%d)
		   AND (%s)
		   -- Before the cap, like every rule above it.
		   AND (%s)
		 ORDER BY e.last_at ASC, e.person_id
		 LIMIT $%d`, liveMemberJoin, userPos, beforePos,
		laterMemberJoin, beforePos, scope, excluded, limitPos), args...)
	if err != nil {
		return nil, fmt.Errorf("search: reading a rep's own quiet relationships: %w", err)
	}
	defer rows.Close()
	return scanEdges(rows)
}
