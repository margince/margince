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
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
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
	// partnerMarks reads a company hit's live partner programme (PartnerMarker).
	// Nil where nothing supplied it (a worker's store, a test that does not
	// ask), and a company hit then carries no marker.
	partnerMarks PartnerMarker
	// companyLogos reads a company hit's logo URL (CompanyLogoReader). Nil
	// leaves every company hit without one, which the client draws as initials.
	companyLogos CompanyLogoReader
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

// WithPartnerMarks binds the reader behind a company hit's `is_partner`.
func (s *Store) WithPartnerMarks(mark PartnerMarker) *Store {
	s.partnerMarks = mark
	return s
}

// WithCompanyLogos binds the reader behind a company hit's `logo_url`.
func (s *Store) WithCompanyLogos(read CompanyLogoReader) *Store {
	s.companyLogos = read
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
	return &Store{
		db: s.db.Bounded(budget), carriedBy: s.carriedBy,
		emailSummaries: s.emailSummaries, partnerMarks: s.partnerMarks,
		companyLogos: s.companyLogos,
	}
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

// Hit carries an authorized lexical result. Identifier tiers precede text
// ranking; employer-only hits remain negative.
type Hit struct {
	Type            string
	ID              ids.UUID
	Title           string
	Snippet         string
	Score           float64
	IdentifierMatch *crmcontracts.SearchResultIdentifierMatch
	// CarriedBy is set on a `tag` hit alone: how many records the caller may
	// see carry this word. Nil elsewhere, and nil when no counter is bound.
	CarriedBy *int
	// EmailSummary is set on an `activity` hit whose activity is an email the
	// caller may read. Nil on every other hit type, nil for a non-email
	// activity, and nil when no reader is bound.
	EmailSummary *crmcontracts.EmailSummary
	// IsPartner is set on a `company` hit alone: whether the account carries a
	// live partner programme. Nil elsewhere, and nil when no marker was taken.
	IsPartner *bool
	// WorksAt is set on a `contact` hit the employer arm found: the matched
	// company it works at. Nil on every hit the query matched by its own text.
	WorksAt *Employer
	// LogoURL is set on a `company` hit wearing a logo, when a reader is bound.
	LogoURL *string
	// Standing is set on a contact, company, deal, lead or project hit the
	// caller may read: what a result list shows beside the name.
	Standing *crmcontracts.SearchHitStanding
}

type Page struct {
	Hits       []Hit
	NextCursor string
	HasMore    bool
	// TypesWithMore is non-nil on a grouped page alone: the types that matched
	// more hits than PerType let it carry, in searchBranches order.
	TypesWithMore []string
	// TypesTimedOut is non-nil on a grouped page alone: the types the time
	// budget stopped, which hold no hits here, in the order of TypesWithMore.
	TypesTimedOut []string
	// NearMatches is set only when NearMatches was asked for and a plain first
	// page found nothing: names spelt close to the query, never hits.
	NearMatches []NearMatch
}

type Input struct {
	Query  string
	Types  []string
	Limit  int
	Cursor string
	// Within bounds the search to these records; nil bounds nothing. An empty,
	// non-nil set finds nothing, because a bound set with no members is still
	// a bound — reading it as "no bound" would search the whole corpus.
	Within []ids.UUID
	// PerType, when set, asks for a grouped page — at most this many hits of
	// each type — instead of one ranked list. Nil is the ranked list.
	PerType *int
	// WithEmployees adds the employer arm (employerArmSQL): contacts found
	// through a company the query matches. GET /v1/search and search_records ask
	// for it; the retrieval and plan lanes keep matching by own text alone.
	WithEmployees bool
	// AddressCompanies admits visible recipient-domain associations for interactive discovery.
	AddressCompanies bool
	// NearMatches asks an empty first page to offer close names. Anchor
	// resolution and retrieval leave it unset: they must never resolve to a guess.
	NearMatches bool
	// WithExtensions adds every declared extension kind to the types a search
	// naming none covers. Only GET /v1/search asks: a kind named in Types is
	// searched either way, and the other lanes keep their core types.
	WithExtensions bool
	// Typeahead leaves messages out while the word being typed is under four
	// letters, unless the request names messages alone. Only a human's typeahead
	// box asks; the results page and search_records rank messages from the
	// first letter.
	Typeahead bool
	// Exclude drops these records, of any type, before ranking and counting.
	Exclude []ids.UUID
}

// maxQueryWords bounds the words one query may hold. The employer arm binds a
// parameter and a clause per word, so this caps the statement a request builds.
const maxQueryWords = 32

// Search runs the cross-object query (contract /search): one ranked list, or
// with PerType one page grouped by type. Every branch carries archived_at IS
// NULL and the caller's row scope; ranked keyset pagination orders (score
// DESC, type, id) so the cursor is stable under concurrent writes.
func (s *Store) Search(ctx context.Context, in Input) (Page, error) {
	// normalizeQuery, not TrimSpace: a TRAILING separator is what says the
	// reader finished a word, and trimming it turned every completed search
	// into a prefix search.
	query := normalizeQuery(in.Query)
	if strings.TrimSpace(query) == "" {
		return Page{}, &BadQueryError{Field: "q", Reason: "q is required"}
	}
	if words := len(strings.Fields(query)); words > maxQueryWords {
		return Page{}, &BadQueryError{Field: "q", Reason: fmt.Sprintf(
			"q holds %d words and a search takes at most %d; keep the words that name what you are looking for",
			words, maxQueryWords)}
	}
	types := requestedTypes(in)
	if in.Typeahead && !namesOnlyMail(in.Types) && fragmentTooShortForMail(query) {
		types = slices.DeleteFunc(slices.Clone(types), func(t string) bool { return t == entityActivity })
	}
	for _, t := range types {
		if !knownEntity(t) {
			return Page{}, &BadQueryError{Field: "types", Reason: fmt.Sprintf("unknown type %q", t)}
		}
	}
	var shape pageShape
	if in.PerType != nil {
		grouped, err := groupedShapeFor(*in.PerType, in)
		if err != nil {
			return Page{}, err
		}
		shape = grouped
	} else {
		ranked, err := rankedShapeFor(in)
		if err != nil {
			return Page{}, err
		}
		shape = ranked
	}

	var page Page
	err := s.db.Read(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if grouped, isGrouped := shape.(groupedShape); isGrouped {
			hits, timedOut, err := s.groupedHits(ctx, tx, in, query, types, grouped)
			if err != nil {
				return err
			}
			page = grouped.pageWith(hits, timedOut)
			return s.finishPage(ctx, tx, in, query, withoutTypes(types, timedOut), &page)
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }

		statement, err := rankedStatement(ctx, in, query, types, shape, arg)
		if err != nil {
			return err
		}
		if statement == "" {
			// Every requested type was denied by object RBAC: an empty
			// page, not an error — search discloses nothing the entity
			// lists would not. Still the shape asked for, so a grouped
			// request gets a grouped page.
			page = shape.page(nil)
			return nil
		}
		hits, err := rank(ctx, tx, statement, args)
		if err != nil {
			return err
		}
		page = shape.page(hits)
		return s.finishPage(ctx, tx, in, query, types, &page)
	})
	if err != nil {
		return Page{}, err
	}
	return page, nil
}

// rankedStatement renders the one statement a search ranks with, binding its
// values through arg. It is empty when object RBAC admits none of the types.
func rankedStatement(ctx context.Context, in Input, query string, types []string, shape pageShape, arg func(any) int) (string, error) {
	branches, err := searchUnion(ctx, in, query, types, arg)
	if err != nil || len(branches) == 0 {
		return "", err
	}
	bound := slices.DeleteFunc([]string{withinClause(in.Within, arg), excludeClause(in.Exclude, arg)},
		func(clause string) bool { return clause == "" })
	return shape.statement(branches, strings.Join(bound, " AND "), arg), nil
}

// finishPage adds what follows the ranking: near matches, the capture-withheld
// note and the hit decoration. answered is the types whose ranking finished; a
// type that ran out of time is not read again here, and the page reports
// nothing for it.
func (s *Store) finishPage(ctx context.Context, tx pgx.Tx, in Input, query string, answered []string, page *Page) error {
	if wantsNearMatches(in, query, *page) {
		var err error
		if page.NearMatches, err = nearMatches(ctx, tx, answered, query); err != nil {
			return err
		}
	}
	if err := noteCaptureWithheld(ctx, tx, answered); err != nil {
		return err
	}
	return s.decorate(ctx, tx, page.Hits)
}

func withoutTypes(types, drop []string) []string {
	return slices.DeleteFunc(slices.Clone(types), func(t string) bool { return slices.Contains(drop, t) })
}

func rank(ctx context.Context, tx pgx.Tx, statement string, args []any) ([]Hit, error) {
	// Unnamed, so Postgres plans it at each Bind with the words: a reused named
	// statement turns generic and rebuilds every tsquery for each row it scores.
	rows, err := tx.Query(ctx, statement, append([]any{pgx.QueryExecModeDescribeExec}, args...)...)
	if err != nil {
		return nil, rankingFault(ctx, err)
	}
	defer rows.Close()
	// Through the SAME judgement, because this is where the cancellation
	// usually lands. pgx returns from Query before the rows are read, so a
	// statement stopped mid-result reports through the iteration — the
	// ceiling would otherwise surface as a raw fault from the scan and the
	// arm above would only ever catch a statement killed before it started
	// returning.
	hits, err := scanHits(rows)
	if err != nil {
		return nil, rankingFault(ctx, err)
	}
	return hits, nil
}

// bindTypedQuery binds a query split at the last separator: the words the
// reader FINISHED, and the fragment they are still typing. The two are matched
// differently — finished words whole, the fragment as a prefix.
func bindTypedQuery(query string, arg func(any) int) (headPos, tailPos int, hasFragment bool) {
	head, tail := splitTypedQuery(query)
	headPos = arg(head)
	// Bound only when there IS a fragment: a parameter no SQL references
	// cannot have its type inferred, and Postgres fails the whole statement.
	if tail != "" {
		tailPos = arg(tail)
	}
	return headPos, tailPos, tail != ""
}

// withinClause renders a search's record bound over the union's output id, or
// "" when there is none. Against the output rather than any branch's table, so
// every branch, present and future, is bounded by the one predicate.
func withinClause(within []ids.UUID, arg func(any) int) string {
	if within == nil {
		return ""
	}
	return fmt.Sprintf("id = ANY($%d)", arg(within))
}

// BadQueryError maps to a 422 at the transport. Field names WHICH query input
// was wrong — q, types or per_type. A malformed page token is not one of them: that answer
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
// Not a partial list either: a ranking cut short would present an arbitrary
// prefix as the ranking. A grouped page names each stopped type in
// types_timed_out instead.
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
		"add another word, or narrow it by record type"
}

func knownEntity(t string) bool {
	if _, declared := recordkinds.Lookup(t); declared {
		return true
	}
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
