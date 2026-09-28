// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

var _ retrieval.Classifier = (*Retriever)(nil)

// Classify judges each record in q.Within against q.Text with the match
// expression and the admission a search hit passes, so a record counted as
// matching here is one the search would have found.
func (r *Retriever) Classify(ctx context.Context, q retrieval.ClassifyQuery) (retrieval.Classification, error) {
	return r.store.classify(ctx, q.Text, string(q.EntityType), q.Within)
}

// verdict is one record's answer to the match expression.
type verdict struct {
	matched bool
	// judgeable is false for a record with no searchable text, which matches
	// nothing — and "matches nothing" read as "does not match" would count it
	// against a claim it says nothing about.
	judgeable bool
}

func (s *Store) classify(ctx context.Context, text, entity string, within []ids.UUID) (retrieval.Classification, error) {
	query := normalizeQuery(text)
	if strings.TrimSpace(query) == "" {
		return retrieval.Classification{}, &BadQueryError{Field: "q", Reason: "q is required"}
	}
	branch, ok := classifiableBranch(entity)
	if !ok {
		return retrieval.Classification{}, &BadQueryError{Field: "types", Reason: fmt.Sprintf("unknown type %q", entity)}
	}
	verdicts := map[ids.UUID]verdict{}
	if len(within) > 0 {
		err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			verdicts, err = judge(ctx, tx, branch, query, within)
			return err
		})
		if err != nil {
			return retrieval.Classification{}, err
		}
	}
	out := retrieval.Classification{}
	for _, id := range within {
		v, found := verdicts[id]
		switch {
		case !found || !v.judgeable:
			// Not found means the search's own admission left it out — archived,
			// or not readable to this caller — which is no verdict at all.
			out.Unjudged = append(out.Unjudged, id)
		case v.matched:
			out.Matched = append(out.Matched, id)
		default:
			out.Unmatched = append(out.Unmatched, id)
		}
	}
	return out, nil
}

// judge runs the branch's match expression over the bounded records, under the
// same object gate, row scope and discovery narrowing the ranked union applies.
func judge(ctx context.Context, tx pgx.Tx, branch searchBranch, query string, within []ids.UUID) (map[ids.UUID]verdict, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	headPos, tailPos, hasFragment := bindTypedQuery(query, arg)
	scope, admitted, err := branchScope(ctx, branch, "t", arg)
	if err != nil || !admitted {
		return map[ids.UUID]verdict{}, err
	}
	tsquery := matchExpression(branch.entity, headPos, tailPos, hasFragment)
	sql := fmt.Sprintf(
		`SELECT t.id, coalesce(t.search_tsv @@ %s, false), coalesce(length(t.search_tsv) > 0, false)
		 FROM %s t
		 WHERE t.id = ANY($%d) AND t.archived_at IS NULL`,
		tsquery, branch.table, arg(within))
	if narrowing := branch.narrowing("t"); narrowing != "" {
		sql += " AND " + narrowing
	}
	if scope != "" {
		sql += " AND " + scope
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, rankingFault(ctx, err)
	}
	defer rows.Close()
	verdicts := map[ids.UUID]verdict{}
	for rows.Next() {
		var id ids.UUID
		var v verdict
		if err := rows.Scan(&id, &v.matched, &v.judgeable); err != nil {
			return nil, rankingFault(ctx, err)
		}
		verdicts[id] = v
	}
	return verdicts, rows.Err()
}

// classifiableBranch is the branch for a record type a classification can
// judge. A word-only branch is not one: a tag has no record to judge.
func classifiableBranch(entity string) (searchBranch, bool) {
	for _, branch := range searchBranches {
		if branch.entity == entity && !branch.textOnly {
			return branch, true
		}
	}
	return searchBranch{}, false
}
