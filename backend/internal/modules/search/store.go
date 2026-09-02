// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type Store struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
}

// NewStore opens this module's store on a handle already bound to the
// workspace it serves.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

// bounded is this store with a time ceiling on every statement it runs.
//
// The ceiling rides the HANDLE, so it reaches the lanes this store opens for
// itself: answering one query plan takes a ranking transaction and an exact
// one, and a ceiling armed at a single call site would leave the other lane as
// unbounded as it was before anybody thought about it.
func (s *Store) bounded(budget time.Duration) *Store {
	return &Store{db: s.db.Bounded(budget)}
}

// forWorkspace is this store re-bound to one tenant of the fleet enumeration.
// Only the index-maintenance passes that walk every workspace use it; a
// request-path caller already holds the handle for the tenant it serves.
func (s *Store) forWorkspace(ws ids.WorkspaceID) *Store {
	return &Store{db: s.db.ForWorkspace(ws)}
}

// Hit is one ranked result. Score is ts_rank_cd over the entity's
// search_tsv — comparable across types because every column uses the
// same 'simple' configuration.
type Hit struct {
	Type    string
	ID      ids.UUID
	Title   string
	Snippet string
	Score   float64
}

type Page struct {
	Hits       []Hit
	NextCursor string
	HasMore    bool
}

type Input struct {
	Query  string
	Types  []string
	Limit  int
	Cursor string
}

// searchBranches declares one UNION branch per searchable entity: the
// scoped tables, their display title, and whether the caller's
// row-scope rides the owner predicate or the activity link walk. A new
// searchable entity is one row here — the query builder derives the
// rest.
type searchBranch struct {
	entity       string
	table        string
	title        string
	snippet      string
	activityWalk bool
	// workspaceWide marks a branch whose rows carry no owner: the tag
	// vocabulary is one word list the whole workspace shares, so object RBAC
	// is the only gate and there is no per-row predicate to render. Asking
	// ScopeClauseFor for one would be an error, not an empty clause.
	workspaceWide bool
	// extraWhere narrows DISCOVERY on this branch, beyond archived_at and the
	// row scope. The by-id graph anchor read (graph.go) deliberately does not
	// apply it: a record named by id is not being discovered, and the own
	// company stays readable everywhere it is asked for by name.
	// The organization branch uses it to keep the installation's own company
	// out of results: search is how people find accounts, and the company
	// running the CRM is not one to find (ADR-0082/A127). It stays reachable
	// by id, and the company page is where it is read.
	//
	// It carries a %s for the ALIAS rather than a fixed one, because a query
	// plan's traversal reads two record types in one statement and the
	// narrowing belongs to whichever of them is being discovered. A fixed
	// alias silently narrowed the wrong table.
	extraWhere string
	// snippetFor, when set, renders the excerpt for this caller in place of
	// `snippet` — for an excerpt that reads a SECOND record and so owes that
	// record's own gate. `snippet` stays the ungated floor it falls back to.
	snippetFor func(ctx context.Context, fallback string, arg func(any) int) (string, error)
}

// projectSnippet is `key · company`, with the company named only to a caller
// who may read that organization: naming the account behind a project is a
// read of the organization row, and a searcher with no organization grant,
// or one outside the row's scope — a capture-private company another rep
// owns — gets the key alone. coalesce keeps the key when the scoped subselect
// finds no row; concat_ws skips a NULL key rather than printing the dot.
func projectSnippet(ctx context.Context, fallback string, arg func(any) int) (string, error) {
	// A denied organization grant is the key-only excerpt, not a refusal:
	// the hit is the project's, which the caller may read.
	if denied := auth.Require(ctx, "organization", principal.ActionRead); denied != nil {
		if !errors.Is(denied, apperrors.ErrPermissionDenied) {
			return "", denied
		}
		return fallback, nil
	}
	scope, err := auth.ScopeClauseFor(ctx, "organization", "o", arg)
	if err != nil {
		return "", err
	}
	if scope != "" {
		scope = " AND " + scope
	}
	return fmt.Sprintf(`coalesce((SELECT concat_ws(' · ', t.key, o.display_name) FROM organization o
			WHERE o.id = t.organization_id AND o.archived_at IS NULL%s), %s)`, scope, fallback), nil
}

// excerpt renders the branch's snippet expression for this caller.
func (b searchBranch) excerpt(ctx context.Context, arg func(any) int) (string, error) {
	if b.snippetFor == nil {
		return b.snippet, nil
	}
	return b.snippetFor(ctx, b.snippet, arg)
}

// narrowing renders this branch's discovery narrowing for one alias, and the
// empty string when the branch has none.
func (b searchBranch) narrowing(alias string) string {
	if b.extraWhere == "" {
		return ""
	}
	return fmt.Sprintf(b.extraWhere, alias)
}

// branchScope is the ONE admission + row-scope resolution every union
// branch (lexical and vector alike) runs: object RBAC hides a denied
// type silently, then the branch carries the caller's scope clause.
//
// The alias is a PARAMETER because a query plan's traversal reads two
// record types in one statement — the target as `t`, the hop as `h`. A
// clause rendered against the wrong alias filters the wrong table, and
// deciding whether a deal is visible by asking whether the caller may
// see the deal, when the question was whether they may see the
// organization behind it, is a visibility rule answering about a
// different row.
func branchScope(ctx context.Context, branch searchBranch, alias string, arg func(any) int) (scope string, admitted bool, err error) {
	if auth.Require(ctx, branch.entity, principal.ActionRead) != nil {
		return "", false, nil
	}
	switch {
	case branch.workspaceWide:
		// No row predicate at all: every seat that may read the vocabulary
		// reads all of it.
		scope = ""
	case branch.activityWalk:
		scope, err = auth.ActivityContentClause(ctx, alias, arg)
	default:
		scope, err = auth.ScopeClauseFor(ctx, branch.entity, alias, arg)
	}
	return scope, true, err
}

var searchBranches = []searchBranch{
	{entity: "person", table: "person", title: "full_name", snippet: "NULL"},
	{entity: "organization", table: "organization", title: "display_name", snippet: "NULL", extraWhere: "NOT %s.is_anchor"},
	{entity: "deal", table: "deal", title: "name", snippet: "NULL"},
	{entity: "lead", table: "lead", title: "coalesce(full_name, company_name, email)", snippet: "NULL"},
	// A project's name alone does not say which account's work it is, and two
	// accounts can run a "Phase 2". The excerpt is the key and the company,
	// which is how a person tells the hits apart; see projectSnippet for the
	// gate the company name passes first.
	{entity: "project", table: "project", title: "name", snippet: "t.key", snippetFor: projectSnippet},
	// `entity` stays a literal and `table` takes the constant, which looks
	// inconsistent and is not: TestContextAnchorEnumMatchesTheSearchableEntities
	// AST-parses the `entity` values and can only read literals, while goconst
	// counts the repeats. Splitting them satisfies both without a waiver.
	//
	// The title folds the provider in ahead of the kind: since ADR-0107/A158 a
	// message's kind is the bare word "message", so a subject-less chat would
	// render identically for every transport. coalesce falls through to the kind
	// for everything that never travelled on one.
	// A tag is a word, not a record, and it is what a person types when they
	// mean "show me the accounts we called Key Account". Finding the word is
	// the step before finding the records, and without it a reader has to know
	// the vocabulary already.
	//
	// Archived words are excluded: a retired tag is not in the picker, so a hit
	// on one leads to a page that cannot be acted on.
	{entity: "tag", table: "tag", title: "name", snippet: "NULL",
		workspaceWide: true, extraWhere: "%s.archived_at IS NULL"},
	{entity: "activity", table: entityActivity, title: "coalesce(subject, channel_provider, kind)", snippet: "left(coalesce(body, ''), 200)", activityWalk: true},
}

// Search runs the ranked cross-object query (contract /search). Every
// branch carries archived_at IS NULL and the caller's row scope; ranked
// keyset pagination orders (score DESC, type, id) so the cursor is
// stable under concurrent writes.
func (s *Store) Search(ctx context.Context, in Input) (Page, error) {
	query := strings.TrimSpace(in.Query)
	if query == "" {
		return Page{}, &BadQueryError{Field: "q", Reason: "q is required"}
	}
	limit := clampLimit(in.Limit)
	types := in.Types
	if len(types) == 0 {
		for _, b := range searchBranches {
			types = append(types, b.entity)
		}
	}
	for _, t := range types {
		if !knownEntity(t) {
			return Page{}, &BadQueryError{Field: "types", Reason: fmt.Sprintf("unknown type %q", t)}
		}
	}

	var cursor *rankedCursor
	if in.Cursor != "" {
		decoded, err := decodeCursor(in.Cursor)
		if err != nil {
			return Page{}, err
		}
		cursor = &decoded
	}

	var page Page
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		qPos := arg(query)

		branches, err := admittedBranchSQL(ctx, types, qPos, arg)
		if err != nil {
			return err
		}
		if len(branches) == 0 {
			// Every requested type was denied by object RBAC: an empty
			// page, not an error — search discloses nothing the entity
			// lists would not.
			return nil
		}

		sql := "SELECT rtype, id, title, snippet, score FROM (" + strings.Join(branches, " UNION ALL ") + ") ranked"
		if cursor != nil {
			// Keyset over the ranked order: strictly worse score, or the
			// same score past the (type, id) tie-break.
			sql += fmt.Sprintf(
				` WHERE score < $%d OR (score = $%d AND (rtype, id) > ($%d, $%d))`,
				arg(cursor.Score), len(args), arg(cursor.Type), arg(cursor.ID))
		}
		sql += fmt.Sprintf(" ORDER BY score DESC, rtype, id LIMIT $%d", arg(limit+1))

		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("search: query: %w", err)
		}
		defer rows.Close()
		page, err = scanRankedPage(rows, limit)
		return err
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// admittedBranchSQL builds one ranked SELECT per requested-and-admitted
// entity type. A hit is a read twice over: object RBAC first (a role
// without person.read gets no person hits — search must not out-see the
// entity lists), then the row scope.
func admittedBranchSQL(ctx context.Context, types []string, qPos int, arg func(any) int) ([]string, error) {
	var branches []string
	for _, branch := range searchBranches {
		if !slices.Contains(types, branch.entity) {
			continue
		}
		scope, admitted, err := branchScope(ctx, branch, "t", arg)
		if err != nil {
			return nil, err
		}
		if !admitted {
			continue
		}
		// Name entities parse the query 'simple' (unaccented — Muller
		// finds Müller), OR-ed with the apostrophe-collapsed parse so
		// "o'reilly" also reaches a row stored as "OReilly" (the index
		// side carries the collapsed tokens, migration 0077); the
		// activity branch additionally ORs the German/English stemmed
		// parses so "Vertrag" reaches rows whose tsvector stemmed
		// "Verträge" under their captured language.
		tsquery := fmt.Sprintf(
			`(websearch_to_tsquery('simple', f_unaccent($%[1]d)) || websearch_to_tsquery('simple', f_fold_apostrophes($%[1]d)))`,
			qPos)
		if branch.entity == "activity" {
			tsquery = fmt.Sprintf(
				`(websearch_to_tsquery('simple', f_unaccent($%[1]d)) || websearch_to_tsquery('simple', f_fold_apostrophes($%[1]d)) || websearch_to_tsquery('german', f_unaccent($%[1]d)) || websearch_to_tsquery('english', f_unaccent($%[1]d)))`,
				qPos)
		}
		snippet, err := branch.excerpt(ctx, arg)
		if err != nil {
			return nil, err
		}
		sql := fmt.Sprintf(
			`SELECT '%s'::text AS rtype, t.id, %s AS title, %s AS snippet,
			        ts_rank_cd(t.search_tsv, %s)::float8 AS score
			 FROM %s t
			 WHERE t.search_tsv @@ %s
			   AND t.archived_at IS NULL`,
			branch.entity, branch.title, snippet, tsquery, branch.table, tsquery)
		if narrowing := branch.narrowing("t"); narrowing != "" {
			sql += " AND " + narrowing
		}
		if scope != "" {
			sql += " AND " + scope
		}
		branches = append(branches, sql)
	}
	return branches, nil
}

// scanRankedPage materializes the ranked rows and derives the keyset
// cursor from the limit+1 overfetch.
func scanRankedPage(rows pgx.Rows, limit int) (Page, error) {
	var page Page
	for rows.Next() {
		var h Hit
		var title, snippet *string
		if err := rows.Scan(&h.Type, &h.ID, &title, &snippet, &h.Score); err != nil {
			return Page{}, err
		}
		if title != nil {
			h.Title = *title
		}
		if snippet != nil {
			h.Snippet = strings.TrimSpace(*snippet)
		}
		page.Hits = append(page.Hits, h)
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Hits) > limit {
		page.Hits = page.Hits[:limit]
		page.HasMore = true
		last := page.Hits[limit-1]
		page.NextCursor = encodeCursor(rankedCursor{Score: last.Score, Type: last.Type, ID: last.ID})
	}
	return page, nil
}

// BadQueryError maps to a 422 at the transport. Field names WHICH query input
// was wrong — q or types. A malformed page token is not one of them: that answer
// is the same on every paginated endpoint, so it is storekit's to give.
type BadQueryError struct {
	Field  string
	Reason string
}

func (e *BadQueryError) Error() string { return "search: " + e.Reason }

// FieldFault names the query input that was actually wrong.
func (e *BadQueryError) FieldFault() (field, code, message string) {
	return e.Field, "invalid_query", e.Reason
}

// rankedCursor is the (score, type, id) keyset position. Encoding keeps
// full float64 precision (strconv 'g' -1) — a rounded score would skip
// or repeat rows on the boundary.
type rankedCursor struct {
	Score float64
	Type  string
	ID    ids.UUID
}

// encodeCursor renders the ranked position. Score, type and id cannot fail to
// marshal; an empty token would be refused on the way back in.
func encodeCursor(c rankedCursor) string {
	token, err := storekit.EncodeOpaque(c)
	if err != nil {
		return ""
	}
	return token
}

func decodeCursor(s string) (rankedCursor, error) {
	c, err := storekit.DecodeOpaque[rankedCursor](s)
	if err != nil {
		return rankedCursor{}, err
	}
	// The envelope proves the token is ours; this proves it names a row. `{}`
	// unmarshals cleanly and leaves a zero id, which would page from a position
	// nothing occupies rather than refuse.
	if c.ID.IsZero() {
		return rankedCursor{}, &storekit.MalformedCursorError{}
	}
	return c, nil
}

func knownEntity(t string) bool {
	for _, b := range searchBranches {
		if b.entity == t {
			return true
		}
	}
	return false
}

// clampLimit maps this module's zero-means-unset ints onto the shared
// CAP-PAGE bounds (default 50, max 200).
func clampLimit(v int) int {
	if v <= 0 {
		return storekit.ClampLimit(nil)
	}
	return storekit.ClampLimit(&v)
}
