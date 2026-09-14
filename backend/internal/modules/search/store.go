// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type Store struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
	// carriedBy counts the records one tag is on, for THIS caller. It is the
	// collections store's own counter, injected by compose because a module
	// never imports a sibling. Calling that counter rather than writing a
	// second one here is what keeps the figure beside a search hit derived
	// from the same rule as the figures on the tag page — including the rule
	// that a caller counts only the records they may see.
	//
	// Nil where nothing supplied it (a worker's store, a test that does not
	// ask): a tag hit then carries no count, which reads as "not known" and
	// never as zero.
	carriedBy TagReachCounter
	// emailSummaries answers the email row behind an activity hit, for THIS
	// caller. It is the activities store's own reader, injected by compose
	// because a module never imports a sibling.
	//
	// Nil where nothing supplied it (a worker's store, a test that does not
	// ask): an email hit then carries no row, and the frontend renders it the
	// generic way rather than showing a blank canonical one.
	emailSummaries EmailSummaryReader
}

// NewStore opens this module's store on a handle already bound to the
// workspace it serves.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

// WithTagReach binds the counter behind a tag hit's `carried_by`.
func (s *Store) WithTagReach(count TagReachCounter) *Store {
	s.carriedBy = count
	return s
}

// WithEmailSummaries binds the reader behind an email hit's `email_summary`.
func (s *Store) WithEmailSummaries(read EmailSummaryReader) *Store {
	s.emailSummaries = read
	return s
}

// bounded is this store with a time ceiling on every statement it runs.
//
// The ceiling rides the HANDLE, so it reaches the lanes this store opens for
// itself: answering one query plan takes a ranking transaction and an exact
// one, and a ceiling armed at a single call site would leave the other lane as
// unbounded as it was before anybody thought about it.
func (s *Store) bounded(budget time.Duration) *Store {
	// Every field travels, not just the handle: this rebuilds the store, so a
	// field left out here is one the bounded lane silently does without.
	return &Store{db: s.db.Bounded(budget), carriedBy: s.carriedBy, emailSummaries: s.emailSummaries}
}

// forWorkspace is this store re-bound to one tenant of the fleet enumeration.
// Only the index-maintenance passes that walk every workspace use it; a
// request-path caller already holds the handle for the tenant it serves.
func (s *Store) forWorkspace(ws ids.WorkspaceID) *Store {
	// carriedBy is deliberately NOT carried across: these passes rebuild the
	// index and answer nobody, so there is no caller whose row scope a count
	// would be taken under. A lane that starts serving hits from here owes
	// itself the counter, and would otherwise report every tag as uncounted.
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
	// CarriedBy is set on a `tag` hit alone: how many records the caller may
	// see carry this word. Nil elsewhere, and nil when no counter is bound.
	CarriedBy *int
	// EmailSummary is set on an `activity` hit whose activity is an email the
	// caller may read. Nil on every other hit type, nil for a non-email
	// activity, and nil when no reader is bound.
	EmailSummary *crmcontracts.EmailSummary
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

// Search runs the ranked cross-object query (contract /search). Every
// branch carries archived_at IS NULL and the caller's row scope; ranked
// keyset pagination orders (score DESC, type, id) so the cursor is
// stable under concurrent writes.
func (s *Store) Search(ctx context.Context, in Input) (Page, error) {
	// normalizeQuery, not TrimSpace: a TRAILING separator is what says the
	// reader finished a word, and trimming it turned every completed search
	// into a prefix search.
	query := normalizeQuery(in.Query)
	if strings.TrimSpace(query) == "" {
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

		// The query split at the last separator: the words the reader FINISHED,
		// and the fragment they are still typing. The two are matched
		// differently — finished words whole, the fragment as a prefix.
		head, tail := splitTypedQuery(query)
		headPos := arg(head)
		// Bound only when there IS a fragment: a parameter no SQL references
		// cannot have its type inferred, and Postgres fails the whole statement.
		tailPos := 0
		if tail != "" {
			tailPos = arg(tail)
		}

		branches, err := admittedBranchSQL(ctx, types, headPos, tailPos, tail != "", arg)
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
			return rankingFault(ctx, err)
		}
		defer rows.Close()
		// Through the SAME judgement, because this is where the cancellation
		// usually lands. pgx returns from Query before the rows are read, so a
		// statement stopped mid-result reports through the iteration — the
		// ceiling would otherwise surface as a raw fault from the scan and the
		// arm above would only ever catch a statement killed before it started
		// returning.
		if page, err = scanRankedPage(rows, limit); err != nil {
			return rankingFault(ctx, err)
		}
		if err := s.countTagReach(ctx, tx, page.Hits); err != nil {
			return err
		}
		return s.attachEmailSummaries(ctx, tx, page.Hits)
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// admittedBranchSQL builds one ranked SELECT per requested-and-admitted
// entity type. A hit is a read twice over: object RBAC first (a role
// without contact.read gets no contact hits — search must not out-see the
// entity lists), then the row scope.
func admittedBranchSQL(ctx context.Context, types []string, headPos, tailPos int, hasFragment bool, arg func(any) int) ([]string, error) {
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
		// What this branch matches, and why it is shaped that way, lives with
		// the expression in typedquery.go — including the parse configurations
		// each entity uses and the rule that only the fragment widens.
		tsquery := matchExpression(branch.entity, headPos, tailPos, hasFragment)

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

// rankingFault tells a spent ceiling apart from every other reason the ranking
// statement stopped before answering.
//
// Postgres raises the same 57014 for a spent statement_timeout, an operator
// cancelling the backend, and the client going away, so the SQLSTATE alone
// cannot carry the judgement. The one that must not be misread is the CALLER:
// a cancelled request is not a too-broad query, and reporting their own
// vanished deadline as a property of their words would send them rewriting a
// query that was fine — with nobody left to read the answer anyway. The live
// context is what separates that case, which is why this takes one.
//
// An operator cancelling the backend under a live request reads as a spent
// ceiling, and is left that way: both mean the statement was stopped before it
// answered, and narrowing the search is the reader's move either way. The same
// call the agent query surface makes, for the same reason.
//
// Only a handle the caller BOUNDED can reach this — compose bounds the request
// path's (server.go) and leaves the background stores alone — so on a
// background reader the raw fault is the honest one and this returns it.
func rankingFault(ctx context.Context, err error) error {
	if storekit.IsQueryCanceled(err) && ctx.Err() == nil {
		return &QueryTooBroadError{Err: err}
	}
	return fmt.Errorf("search: query: %w", err)
}

// QueryTooBroadError is a search that could not be RANKED inside the ceiling
// its handle carries.
//
// A 422 naming `q`, not a 5xx: the server is not broken, and retrying the same
// words will not help. The corpus and the query together are what did not fit,
// and narrowing the query is the reader's move — which is what an actionable
// fault is supposed to say.
//
// Not a partial page either. This operation has no member to carry "these hits
// are what fitted" — unlike the agent query surface, whose Coverage says so —
// and returning the rows that happened to rank before the ceiling would present
// an arbitrary prefix of a ranking as the ranking.
// It WRAPS the database error rather than replacing it, and that is what keeps
// the other reader of this statement working. The agent query surface wants the
// same spent ceiling as a DEGRADED ANSWER — rows dropped, a note, a coverage
// verdict saying the plan was abandoned — and it recognises one by asking
// storekit.IsQueryCanceled. A fault that hid the SQLSTATE would turn that
// surface's honest partial answer into an error, which is the opposite of what
// #1847 built it for.
type QueryTooBroadError struct{ Err error }

func (e *QueryTooBroadError) Error() string {
	return "search: the query could not be ranked within this surface's budget: " + e.Err.Error()
}

// Unwrap is what lets both readers see the same stopped statement: this
// surface as an actionable 422, the query executor as a plan to abandon.
func (e *QueryTooBroadError) Unwrap() error { return e.Err }

// FieldFault names `q`, because `q` is what the reader can change.
func (e *QueryTooBroadError) FieldFault() (field, code, message string) {
	return "q", "query_too_broad", "this search matched too much of the workspace to rank in time — " +
		"add another word, or narrow it with types"
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
