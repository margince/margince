// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A record held under a statutory retention obligation is unavailable in
// EVERY ordinary read path (A165/ADR-0114 §2): lists, timelines, search,
// exports, embeddings, agent grounding. This gate derives the readers of the
// activity table from the tree and asks each how it excludes a held row,
// because a reader that forgets is indistinguishable from one that never
// existed — until a supervisory authority asks why an erased subject's
// correspondence is on a sales rep's screen.
//
// A file satisfies the gate by ONE of three means, each a real exclusion:
//
//   - it carries the shared row scope (auth.ActivityDiscoverClause / ActivityContentClause or one of the
//     probes built on it), which always composes ActivityAvailableClause;
//   - it names restricted_at itself, or the privacy floor fragments that do;
//   - it filters `archived_at IS NULL`, which excludes held rows because the
//     schema makes restricted imply archived (activity_restricted_is_archived).
//
// A file that reads activity by none of those is waived here with the reason
// it may — and each waiver names the cost.

import (
	"go/ast"
	"path"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// activityReadLiteral matches a SQL string literal that reads the activity
// table by name. `activity_link`, `activity_participant` and the other
// activity_* tables are deliberately not matched: they carry no content.
//
// The pattern is gatekit's, shared with the sibling censuses: a matcher that
// stops seeing this tree's SQL judges nothing and reads exactly like a clean
// tree, so it has one place to be right and one place to be tested.
var activityReadLiteral = gatekit.TableReadPattern("activity")

// scopeMarkers are the shared gates that carry the availability test: a reader
// reaching activity through one of them cannot see a held row. They are Go
// calls rather than SQL, so they are matched on the names a reader reaches
// rather than inside its statements.
//
// The list is every activity gate auth exports plus the two privacy floor
// predicates, and it is stated rather than derived because each entry is a
// claim that THAT function composes ActivityAvailableClause. EnsureActivityWritable
// does so through EnsureActivityContentVisibleLive; it is here because a write
// path probes with it and nothing else, and a reader inside such a transaction
// is guarded by a call this gate could not otherwise see.
var scopeMarkers = []string{
	"ActivityDiscoverClause", "ActivityContentClause", "ActivityAvailableClause",
	"EnsureActivityVisible", "EnsureActivityVisibleLive",
	"EnsureActivityContentVisible", "EnsureActivityContentVisibleLive",
	"EnsureActivityWritable",
	"correspondenceFloorPredicate", "handelsbriefShielded",
}

// literalMarkers exclude a held row in the FUNCTION that carries them, not
// merely somewhere in the file. That is the granularity the tree actually
// supports: this SQL is assembled from concatenated fragments and shared
// constants, so the `FROM activity` and its `archived_at IS NULL` are
// routinely different string literals in one query, and matching per literal
// would report green code as red. Per function is still far tighter than per
// file — an `archived_at IS NULL` belonging to a different query in the same
// file no longer answers for the activity read beside it, which is the false
// negative both reviews of this gate found.
//
// `archived_at IS NULL` counts because the schema makes restricted imply
// archived (activity_restricted_is_archived), and the guard refuses a write
// that would un-archive a held row.
//
// The column may be aliased (`a.archived_at`) or bare, so the match is on the
// column and its test rather than on one spelling of the pair — a gate that
// only recognised the unaliased form would report green code as red, which
// costs its own credibility faster than a miss does.
var literalMarkers = []*regexp.Regexp{
	regexp.MustCompile(`(?i)restricted_at`),
	regexp.MustCompile(`(?i)\barchived_at\s+IS\s+NULL`),
}

// restrictedReadersAdmitted ratifies the readers that reach a held row and may.
//
// Keyed by FILE AND FUNCTION. It used to be keyed by file, which meant a
// waiver written for one reader silently covered every reader added to that
// file afterwards — the same looseness the file-scope exemption had, in the
// ratification list rather than in the walk. A key names one reader, and
// AssertAllMatched reports one whose reader has gone.
//
// Two entries that were here are gone rather than fixed: the replay lookup in
// activities/activity.go and the classify backlog in activities/capturelabel.go
// both stated in prose what the call graph now derives — the replay hands its
// id to readActivity, which carries the scope, and the backlog composes
// ClassifyBacklogPredicate, which filters archived rows. A reason a gate can
// work out is better computed than written down.
var restrictedReadersAdmitted = gatekit.Waive(map[string]string{
	"internal/compose/audiencerescope.go:AudienceRescopeGen.rescope": "the audience-change consumer reads the thread key (content by the activity policy) and the capture owner of the ONE activity whose audience just moved — deliberately, as a system principal, because both are exactly what narrowing the derived models needs, to NARROW what other readers may see — excluding a held row here would leave a legal-hold conversation's derived signals workspace-visible, the exact disclosure the consumer exists to remove. The cost is that a held activity's thread key and owner id reach this system principal",
	"internal/modules/privacy/auditlog.go:ListAuditLog":              "the compliance read joins activity to evaluate ONE predicate — the audience arm the row's author set — and projects a single boolean from it. No activity column reaches the caller: the join's whole output is content_readable, which can only ever WITHHOLD an audit image, never reveal an activity. A held activity is therefore no more readable through this join than without it. The cost is that the audit IMAGE of a held activity stays readable to the admin, which is a pre-existing property of audit_log rather than of this join — audit_log is append-only and the hold is on the activity — and is filed rather than settled here, because making the compliance trail skip held rows is a decision about A165 and not a fix to the audience gap this join closes",
	"internal/modules/capture/tracestore.go:TraceStore.readRungs":    "the capture trace ladder LEFT JOINs activity to reach one thing — the counterparty email a stored trace row was raised about — and uses it only inside the lateral's WHERE, to pick which disposition verdict applies. Every column it PROJECTS comes from capture_trace and from capture_pending_counterparty; no activity column is scanned, so a held activity is no more readable through this join than without it. Excluding held rows here would instead blank the disposition on a trace row whose message is under hold, which tells an operator the connector did nothing when it did. The cost is that a held activity's counterparty_email decides which verdict a trace row shows — a fact about the trace, never content of the activity",
})

// activityReaderScope is every non-test, non-generated file under internal/
// that reads the activity table by name.
var activityReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsActivityTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsActivityTable(path string, file *ast.File) bool {
	return gatekit.FileReadsTable(path, file, activityReadLiteral)
}

// unguardedActivityReaders names each reader in the file that reads the
// activity table and reaches no exclusion.
//
// A reader's exclusion is often not in the reader. This tree routinely splits
// one query across a function and the helper that builds its WHERE —
// ListActivitiesTx + listActivitiesFilter, listOpenTasks + openTasksFilter —
// so a function judged on its own body alone reports red while the scope it
// applies sits ten lines below, which teaches the next engineer to distrust
// the gate rather than to fix anything.
//
// That used to be answered by exempting the whole FILE the moment it mentioned
// one of the shared gates anywhere, and that is too much: a new unguarded
// reader added to such a file inherits an exemption earned by a different
// function. The exemption is call-graph-scoped now — a reader is guarded by
// what IT reaches, one level into its own package — which admits the split
// queries and admits nothing else.
//
// The graph is packageCallGraph, shared with the privacy and rename censuses,
// so what "reaches" means has one place to be right. Its own doc states the
// limit that matters here: an edge through an interface, a stored field or a
// closure is not followed, and an unfollowed edge is a route this gate cannot
// see rather than a route that carries nothing. A reader guarded only through
// such a call reports red and is ratified by name, which is the direction that
// asks a human instead of assuming one.
func unguardedActivityReaders(graph map[string]*graphFunc, file *ast.File) []string {
	var offenders []string
	for _, decl := range file.Decls {
		reads := gatekit.DeclReads(decl, activityReadLiteral)
		if len(reads) == 0 {
			continue
		}
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			// A package-level SQL fragment is half a query by construction, so
			// it is judged from the other end: at the functions that NAME it,
			// each of which owes the exclusion for the query it assembles.
			// Reporting the fragment itself would name a declaration nobody can
			// fix in place — the WHERE belongs to whoever composes it.
			//
			// Per BINDING, not per declaration. One `const (…)` block binds
			// several names, and only one of them may hold the activity SQL, so
			// a block judged whole would answer for a function that names its
			// unrelated sibling — reporting a reader that reads nothing, and
			// counting a sibling's namer as evidence that the fragment is
			// composed somewhere.
			for _, fragment := range activityFragmentsIn(decl) {
				for _, namer := range unguardedNamers(graph, fragment.names) {
					offenders = append(offenders, namer+": "+gatekit.FirstLineOf(fragment.sql))
				}
			}
			continue
		}
		key := scrubKey(receiverTypeName(fn), fn.Name.Name)
		if guardedInItsPackage(graph, key) {
			continue
		}
		offenders = append(offenders, key+": "+gatekit.FirstLineOf(reads[0].SQL))
	}
	return offenders
}

// guardedInItsPackage answers whether a reader is excluded from held rows by
// anything in its own package — what it reaches, or what reaches it.
//
// Both directions, because this tree splits a query BOTH ways. A reader calls
// the helper that writes its WHERE (ListActivitiesTx + listActivitiesFilter),
// and a reader is called by the transaction that probed and locked the row
// first (readAudienceImage under SetActivityAudience, which holds the row
// LiveOnly and passes EnsureActivityWritable before reading it). Judging only
// downwards reports the second kind red while its exclusion sits in the frame
// above.
//
// The caller arm demands EVERY caller, not any: a helper reached by one
// guarded transaction and one bare one is exactly the case worth a sentence,
// and it is the bare caller that gets reported by its own name as well. A
// function nothing calls is not guarded by vacuous truth — an unreferenced
// reader has no transaction to inherit from.
//
// What neither arm can check is whether the exclusion a neighbour carries
// actually constrains THIS query. That is the same assumption the downward arm
// has always made, and it is the price of a gate that does not report the
// composition this tree is built out of.
func guardedInItsPackage(graph map[string]*graphFunc, key string) bool {
	if reachesAnExclusion(graph, key, true) {
		return true
	}
	called := false
	for callerKey, entry := range graph {
		if callerKey == key || !entry.calls[key] {
			continue
		}
		called = true
		if !reachesAnExclusion(graph, callerKey, true) {
			return false
		}
	}
	return called
}

// reachesAnExclusion answers whether one function excludes a held row — in its
// own statements and identifiers, or through a function of its own package.
//
// followCalls is spent on the first hop and not renewed. One level is the
// shape the tree has: a reader and the helper that writes its WHERE. Following
// further would start admitting readers whose exclusion is three unrelated
// frames away, which is the file-scope looseness this replaced, spelled
// differently.
func reachesAnExclusion(graph map[string]*graphFunc, key string, followCalls bool) bool {
	entry, known := graph[key]
	if !known {
		return false
	}
	for _, statement := range entry.statements {
		if matchesAny(statement, literalMarkers) {
			return true
		}
	}
	// The shared gates are Go calls rather than SQL, so they are looked for
	// among the names the function reaches. `calls` alone is not enough: this
	// tree passes a clause builder by name as often as it calls it.
	for _, names := range []map[string]bool{entry.calls, entry.reads} {
		for name := range names {
			if matchesMarker(name, scopeMarkers) {
				return true
			}
		}
	}
	if !followCalls {
		return false
	}
	for callee := range entry.calls {
		if reachesAnExclusion(graph, callee, false) {
			return true
		}
	}
	return false
}

// fragmentBinding is ONE package-level binding whose value reads the activity
// table: the names it binds, and the statement that made it a subject.
type fragmentBinding struct {
	names []string
	sql   string
}

// activityFragmentsIn splits a package-level declaration into the individual
// bindings that read the activity table.
//
// A `const (…)` block is one declaration binding many names, and the activity
// SQL usually belongs to exactly one of them. Collecting every name in the
// block would let a function that names an unrelated sibling answer for the
// fragment — as evidence that it is composed somewhere, and, when that
// function reaches no exclusion, as a reported reader of SQL it never touches.
func activityFragmentsIn(decl ast.Decl) []fragmentBinding {
	gen, isGen := decl.(*ast.GenDecl)
	if !isGen {
		return nil
	}
	var bindings []fragmentBinding
	for _, spec := range gen.Specs {
		value, isValue := spec.(*ast.ValueSpec)
		if !isValue {
			continue
		}
		bindings = append(bindings, activityBindingsInSpec(value)...)
	}
	return bindings
}

// activityBindingsInSpec pairs each value in one spec with the name it is bound
// to.
//
// One spec can bind several at once — `const activitySQL, personSQL = "…", "…"`
// — so taking every name in the spec would let the person statement's name
// answer for the activity one, in both directions: as evidence the activity
// fragment is composed somewhere, and, when the function naming it reaches no
// exclusion, as a reported reader of SQL it never touches.
//
// Names and values pair by index while their counts agree, which is what a
// spec of literals always looks like. When they do not — `var a, b =
// twoResults()` binds two names to one expression — the value is attributed to
// every name in the spec, because there is no index to pair on and the
// alternative is attributing it to nobody. That direction over-reports, which
// is the safe one: it can turn a clean function into a finding somebody looks
// at, never an activity reader into a clean one.
func activityBindingsInSpec(spec *ast.ValueSpec) []fragmentBinding {
	names := make([]string, 0, len(spec.Names))
	for _, name := range spec.Names {
		if name.Name != "_" {
			names = append(names, name.Name)
		}
	}
	paired := len(spec.Names) == len(spec.Values)
	var bindings []fragmentBinding
	for i, value := range spec.Values {
		sql, reads := activitySQLIn(value)
		if !reads {
			continue
		}
		bound := names
		if paired {
			bound = nil
			if spec.Names[i].Name != "_" {
				bound = []string{spec.Names[i].Name}
			}
		}
		bindings = append(bindings, fragmentBinding{names: bound, sql: sql})
	}
	return bindings
}

// activitySQLIn is the first statement in this value that reads the activity
// table, and whether there is one.
func activitySQLIn(value ast.Expr) (string, bool) {
	found := ""
	ast.Inspect(value, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		expr, isExpr := node.(ast.Expr)
		if !isExpr {
			return true
		}
		text, isText := gatekit.LiteralText(expr)
		if isText && activityReadLiteral.MatchString(text) {
			found = text
			return false
		}
		return true
	})
	return found, found != ""
}

// unguardedNamers are the functions that assemble a query from this fragment
// and reach no exclusion, sorted so a failure reads the same on every run.
//
// A fragment nothing names answers with the fragment itself, because unused
// SQL that reads activity is either dead or reached by a spelling this gate
// cannot see, and both deserve a sentence. nil means every namer is guarded,
// which is the only way a fragment passes.
func unguardedNamers(graph map[string]*graphFunc, names []string) []string {
	var unguarded []string
	named := false
	for key, entry := range graph {
		mentions := false
		for _, name := range names {
			if entry.reads[name] || entry.calls[name] {
				mentions = true
			}
		}
		if !mentions {
			continue
		}
		named = true
		if !guardedInItsPackage(graph, key) {
			unguarded = append(unguarded, key)
		}
	}
	if !named {
		return []string{"a package-level SQL fragment no function names"}
	}
	sort.Strings(unguarded)
	return unguarded
}

// matchesMarker is CallsAny's substring test over a name already extracted,
// kept identical because this tree reaches a shared gate through a
// package-local wrapper as often as directly.
func matchesMarker(name string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func TestEveryReaderOfTheActivityTableExcludesRestrictedRows(t *testing.T) {
	t.Parallel()
	defer restrictedReadersAdmitted.AssertAllMatched(t)
	subjects := activityReaderScope.Files(t)
	graphs := map[string]map[string]*graphFunc{}
	for _, src := range subjects {
		dir := path.Dir(src.Path)
		if _, done := graphs[dir]; !done {
			// The whole package, not the reading file: a helper that builds the
			// WHERE need not read the activity table itself, so it often sits in
			// a file this gate's Scope never selects. Resolving against subjects
			// alone would follow the call, find no body, and report exactly like
			// a helper that excludes nothing.
			graphs[dir] = packageCallGraph(t, dir)
		}
		for _, offender := range unguardedActivityReaders(graphs[dir], src.File) {
			if restrictedReadersAdmitted.Waived(t, src.Path+":"+strings.SplitN(offender, ":", 2)[0]) {
				continue
			}
			t.Errorf("%s: %s reads the activity table and excludes no held row — compose auth.ActivityContentClause / ActivityDiscoverClause / ActivityAvailableClause, filter `restricted_at IS NULL` or `archived_at IS NULL`, or ratify the reader in restrictedReadersAdmitted with the cost stated (A165/ADR-0114 §2)", src.Path, offender)
		}
	}
}

func matchesAny(text string, markers []*regexp.Regexp) bool {
	for _, marker := range markers {
		if marker.MatchString(text) {
			return true
		}
	}
	return false
}
