// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which duplicate pairs a caller may SEE, and which of those they could
// actually SETTLE.
//
// Two different questions over the same two subject rows, and keeping them
// beside each other is the point: they share the both-ends shape, they differ
// only in which halves of authority each demands, and a reader comparing them
// can see that the second is the first plus the write arm. Split out of
// dedupequeue.go, which is the queue's reads and writes.

import (
	"context"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// dedupePairSides is the three entity types a pair can be, with the columns
// each one's two ends live in.
//
// ONE table, read by both clauses below. They walk the same three types over
// the same six columns and differ only in which halves of authority each
// demands, so a second copy of the column names is a chance for the filter and
// the flag to come to disagree about which rows a pair is even about.
type dedupePairSide struct{ entity, left, right string }

var dedupePairSides = []dedupePairSide{
	{entityContact, "left_contact_id", "right_contact_id"},
	{entityCompany, "left_company_id", "right_company_id"},
	{entityLead, "left_lead_id", "right_lead_id"},
}

// dedupePairSideFor answers which columns hold a pair's ends for one entity
// type, or false for a record type that has no dedupe queue at all.
func dedupePairSideFor(entityType string) (dedupePairSide, bool) {
	for _, side := range dedupePairSides {
		if side.entity == entityType {
			return side, true
		}
	}
	return dedupePairSide{}, false
}

// dedupePairArms renders one OR-arm per entity type, each asking `authority` of
// both ends under an alias prefixed to keep the two clauses' subqueries apart in
// one statement.
func dedupePairArms(
	prefix string, mustBeLive bool,
	authority func(entity, alias string) (string, error),
) (string, error) {
	arms := make([]string, 0, len(dedupePairSides))
	for i, side := range dedupePairSides {
		alias := fmt.Sprintf("%s%d", prefix, i)
		clause, err := authority(side.entity, alias)
		if err != nil {
			return "", err
		}
		arms = append(arms, fmt.Sprintf("(entity_type = '%s' AND %s)", side.entity,
			bothSidesServable(side.entity, alias, side.left, side.right, clause, mustBeLive)))
	}
	return "(" + strings.Join(arms, " OR ") + ")", nil
}

// dedupeVisibilityClause renders the queue's row-scope filter: a candidate
// surfaces only when BOTH sides of its pair are visible to the caller —
// the evidence snapshot reads both records, so listing a pair IS a read of
// them (H1). Empty for unbounded callers.
func dedupeVisibilityClause(ctx context.Context, arg func(any) int, mustBeLive bool) (string, error) {
	return dedupePairArms("v", mustBeLive, func(entity, alias string) (string, error) {
		return auth.ScopeClauseFor(ctx, entity, alias, arg)
	})
}

// bothSidesServable is the per-side predicate for one entity type: the subject
// row must EXIST, be LIVE, and pass whatever row scope the caller has.
//
// Emitted unconditionally, where the predicate this replaced was skipped whole
// when no record type narrowed the caller. That was not the bug — contact and
// company are capture-private, so even an all-scope human is bounded on
// them and the clause was built anyway — but it made the liveness term depend on
// a scope the reader might not have. A system principal is unbounded on all
// three (auth.Unbounded), so on the day one reads this queue the old shape would
// have served it every archived pair in the installation.
//
// The liveness term is not part of the scope clause and cannot be folded into
// it. auth.EnsureVisibleLive says why in its own words: erasure anonymizes a
// contact in place and stamps archived_at while LEAVING owner_id alone, so a
// scope predicate answers "yes, still yours" for a record every live read path
// refuses. The same is true of a plain archive.
//
// Without it a decision outlives both records it is about. The candidate carries
// its own archived_at and nothing sweeps it when a SUBJECT is archived, so the
// pair keeps the confidence it was filed with and holds its rank in a lane that
// serves ten by score. Archiving one of two duplicates is a reasonable way to
// resolve a pair — the most natural one for a company entered twice, since it
// needs no merge decision — so the more diligently a workspace resolves them
// that way, the faster its queue fills with its own finished work.
func bothSidesServable(entityType, alias, leftColumn, rightColumn, scopeClause string, mustBeLive bool) string {
	side := func(column string) string {
		terms := fmt.Sprintf("%[1]s.id = dedupe_candidate.%[2]s", alias, column)
		if mustBeLive {
			terms += fmt.Sprintf(" AND %s.archived_at IS NULL", alias)
		}
		if scopeClause != "" {
			terms += " AND " + scopeClause
		}
		return fmt.Sprintf("EXISTS (SELECT 1 FROM %s %s WHERE %s)", entityType, alias, terms)
	}
	return "(" + side(leftColumn) + " AND " + side(rightColumn) + ")"
}

// dedupeDecidableExpr renders the per-row "could this caller dispose of it"
// answer, in the same both-sides shape the visibility filter uses.
//
// BOTH halves of authority, and both ENDS. The visibility clause is not enough:
// a seat can read a record it may not change, and disposing of a pair changes
// both — dismissing suppresses them for the whole workspace and merging rewrites
// one into the other. So each side must pass the read scope AND the write arm,
// which is exactly what EnsureWritable checks one row at a time on the way in.
//
// It answers about the CALLER, so it is a column and not a filter. Narrowing the
// list to decidable pairs would hide a real duplicate from the contact best
// placed to notice it, and a pair nobody can decide would become invisible
// rather than merely stuck.
func dedupeDecidableExpr(ctx context.Context, arg func(any) int, mustBeLive bool) (string, error) {
	expr, err := dedupePairArms("d", mustBeLive, func(entity, alias string) (string, error) {
		scope, err := auth.ScopeClauseFor(ctx, entity, alias, arg)
		if err != nil {
			return "", err
		}
		write, err := auth.WriteAuthorityClauseFor(ctx, entity, alias, arg)
		if err != nil {
			return "", err
		}
		return andClauses(scope, write), nil
	})
	if err != nil {
		return "", err
	}
	return expr + " AS can_decide", nil
}

// andClauses joins the two authority halves, skipping an empty one — an
// unbounded caller narrows on neither, and "true AND true" in every row of
// every arm is noise in an EXPLAIN somebody will one day read.
func andClauses(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + " AND " + b
	}
}
