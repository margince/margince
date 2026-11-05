// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxNearMatches is how many names an empty search offers.
const maxNearMatches = 5

// NearMatch is a record whose name is spelt close to a query that matched
// nothing. It is a suggestion, never a hit, and carries no score or count.
type NearMatch struct {
	Type    string
	ID      ids.UUID
	Title   string
	Snippet string
}

// NearNamedBranches names the entities the near-match pass reads: the ones
// whose branch declares the indexed name expression it matches on.
func NearNamedBranches() []string {
	var entities []string
	for _, branch := range searchBranches {
		if branch.nearName != "" {
			entities = append(entities, branch.entity)
		}
	}
	return entities
}

// nearNeedle is the folded query the near-match pass compares names with.
func nearNeedle(queryPos int) string {
	return fmt.Sprintf("f_fold_apostrophes(lower($%d::text))", queryPos)
}

// NearPredicate returns the table and the `WHERE` predicate, for the given alias
// and query placeholder number, that an entity's near-match pass filters on.
// It is empty for an entity with no near-match branch.
func NearPredicate(entity, alias string, queryPos int) (table, predicate string) {
	for _, branch := range searchBranches {
		if branch.entity == entity && branch.nearName != "" {
			return branch.table, nearNeedle(queryPos) + " <% " + fmt.Sprintf(branch.nearName, alias)
		}
	}
	return "", ""
}

// wantsNearMatches says whether an answer is the empty first page of a plain
// name search. A bound, a later page or an operator query is never "found
// nothing", and a non-empty page is never offered a guess.
func wantsNearMatches(in Input, query string, page Page) bool {
	return in.NearMatches && len(page.Hits) == 0 && in.Cursor == "" && in.Within == nil &&
		!carriesOperators(query)
}

// nearMatches lists up to maxNearMatches records whose name is word-similar to
// the query. Each branch passes the same object gate and row scope as the
// lexical union. A suggestion is never a record the reader could not open.
// Its statement has its own argument list: a placeholder no SQL references
// fails the whole statement.
func nearMatches(ctx context.Context, tx pgx.Tx, types []string, query string) ([]NearMatch, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	queryPos := arg(query)
	needle := nearNeedle(queryPos)

	var branches []string
	for _, branch := range searchBranches {
		if branch.nearName == "" || !slices.Contains(types, branch.entity) {
			continue
		}
		scope, admitted, err := branchScope(ctx, branch, "t", arg)
		if err != nil {
			return nil, err
		}
		if !admitted {
			continue
		}
		snippet, err := branch.excerpt(ctx, arg)
		if err != nil {
			return nil, err
		}
		name := fmt.Sprintf(branch.nearName, "t")
		_, predicate := NearPredicate(branch.entity, "t", queryPos)
		where := conjunction("t.archived_at IS NULL", predicate, branch.narrowing("t"), scope)
		branches = append(branches, fmt.Sprintf(
			`SELECT '%s'::text AS rtype, t.id, %s AS title, %s AS snippet,
			        word_similarity(%s, %s) AS word_sim, similarity(%s, %s) AS sim
			 FROM %s t WHERE %s`,
			branch.entity, branch.title, snippet, needle, name, needle, name,
			branch.table, where))
	}
	if len(branches) == 0 {
		return nil, nil
	}
	statement := fmt.Sprintf(`SELECT rtype, id, title, snippet FROM (%s) near
		ORDER BY word_sim DESC, sim DESC, rtype, id LIMIT %d`,
		strings.Join(branches, " UNION ALL "), maxNearMatches)
	rows, err := tx.Query(ctx, statement, append([]any{pgx.QueryExecModeDescribeExec}, args...)...)
	if err != nil {
		return nil, rankingFault(ctx, err)
	}
	defer rows.Close()
	var found []NearMatch
	for rows.Next() {
		var m NearMatch
		var title, snip *string
		if err := rows.Scan(&m.Type, &m.ID, &title, &snip); err != nil {
			return nil, rankingFault(ctx, err)
		}
		if title != nil {
			m.Title = *title
		}
		if snip != nil {
			m.Snippet = strings.TrimSpace(*snip)
		}
		found = append(found, m)
	}
	if err := rows.Err(); err != nil {
		return nil, rankingFault(ctx, err)
	}
	return found, nil
}
