// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// The concurrency-guard obligation as a fitness function: every
// single-row-by-id UPDATE of a mutable entity carries SOME guard — the
// optimistic version (storekit.ApplyWithVersion / ApplyGuarded), a held
// row lock (LockRow / LockPair + ApplyLocked), an advisory lock, an
// in-statement FOR UPDATE, or a checked conditional write (the
// RowsAffected CAS shape). An unguarded by-id UPDATE is the
// last-writer-wins bug class this repo removed from storekit; this test
// keeps raw SQL from reintroducing it. Set-based writes (relinks,
// sweeps over a WHERE that is not the primary key) are out of scope by
// construction — they are not read-modify-write on one row.
//
// Exceptions are explicit, keyed by package path + function, each with
// the rationale that ratified it; a reasonless or stale waiver fails.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// byIDUpdate matches a single-row-by-primary-key UPDATE inside one SQL
// string literal — the shape that must carry a concurrency guard. The
// obligation applies to VERSIONED tables (the schema's own declaration
// that a row is a concurrently-edited entity), derived from the
// migrations rather than maintained as a list.
var byIDUpdate = regexp.MustCompile(`(?is)\bUPDATE\s+([a-z_]+)\s.*\bSET\b.*\bWHERE\s+(?:[a-z]\.)?id\s*=\s*\$`)

// versionPredicate is a version compared against a PARAMETER, which is a
// compare-and-set. `version = version + 1` in the SET clause is not one and does
// not match: the placeholder is what makes the statement fail on a row somebody
// else moved.
//
// Matched against the WHERE clause only, never the whole statement. `SET …,
// version = $5` assigns a client-supplied version and guards nothing, so
// crediting it would wave through the lost update this gate exists to refuse.
var versionPredicate = regexp.MustCompile(`(?i)\bversion\s*=\s*\$\d+`)

// whereClause is the tail of a statement from its last WHERE onward, which is
// the only part where a predicate can constrain which row is written.
var whereClause = regexp.MustCompile(`(?is)\bWHERE\b(.*)$`)

// byIDUpdateFloor is the smallest number of by-id-updating functions this
// census may find and still be believed. Set well below the real count, because
// its job is to catch a reader that has gone quiet, not to track the tree.
//
// It is the coarser of the two under-recognition alarms and covers the UNWAIVED
// remainder. The sharper one is unguardedByIDUpdates.AssertAllMatched: a waived
// function the reader stops seeing is named outright, because its ratification
// then matches nothing.
const byIDUpdateFloor = 90

var (
	// createTableLine opens a CREATE TABLE block; versionColumnLine marks
	// the block's table as optimistic-locking. Line-based on purpose:
	// column definitions nest parentheses (generated tsvector columns)
	// beyond what a block regex can pair.
	createTableLine   = regexp.MustCompile(`(?i)^\s*CREATE TABLE (?:IF NOT EXISTS )?([a-z_]+)\s*\(`)
	versionColumnLine = regexp.MustCompile(`(?i)^\s*version\s+bigint`)
)

// versionedTables derives the set of optimistic-locking tables from the
// migration sources: any CREATE TABLE whose columns include "version".
func versionedTables(t *testing.T) map[string]bool {
	t.Helper()
	tables := map[string]bool{}
	for _, root := range []string{"migrations/core", "migrations/custom"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".up.sql") {
				return err
			}
			raw, err := os.ReadFile(path) // #nosec G304 G122 -- path is a *.up.sql file from walking the trusted migrations tree
			if err != nil {
				return err
			}
			current := ""
			for _, line := range strings.Split(string(raw), "\n") {
				if m := createTableLine.FindStringSubmatch(line); m != nil {
					current = m[1]
					continue
				}
				if strings.HasPrefix(line, ");") {
					current = ""
					continue
				}
				if current != "" && versionColumnLine.MatchString(line) {
					tables[current] = true
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(tables) < 10 {
		t.Fatalf("derived only %d versioned tables from migrations — the derivation is broken, not the schema", len(tables))
	}
	return tables
}

// unguardedByIDUpdates are the ratified guard-free by-id updates, keyed
// by "package-dir:FuncName". Every entry carries its rationale inline;
// an entry without one is a finding, and one matching no function is
// stale and fails.
var unguardedByIDUpdates = gatekit.Waive(map[string]string{
	// This IS a compare-and-set write; what this gate cannot see is the shape it
	// checks the outcome in. It witnesses RowsAffected on a tx.Exec, and this
	// sends the same conditional UPDATE through QueryRow so the value the write
	// replaced comes back from the statement that replaced it — reading it
	// separately would name whatever the next writer had put there, and the
	// audit row would describe two transactions as one. A zero-row result
	// arrives as pgx.ErrNoRows, which the caller handles as the decline it is
	// rather than as a failure.
	"internal/modules/people:bindSiteReadLogo": "the bind is conditioned on logo_object_key IS NULL AND archived_at IS NULL, and ErrNoRows means the record already wears a mark or was archived, which releases the parked object instead",

	// The second of that shape, and the reason it is ratified rather than
	// taught to the witness: crediting a bare mention of pgx.ErrNoRows would
	// hand a free pass to any function that checks it on an unrelated read. A
	// third occurrence is the point at which that trade stops being worth it.
	"internal/modules/privacy:PinToFloor": "the same QueryRow compare-and-set: `WHERE a.id = $1 AND a.restricted_at IS NULL ... RETURNING a.restricted_until` matches nothing once any restriction stands, and the zero-row result arrives as pgx.ErrNoRows, which the caller turns into ErrConflict — a second controller pinning the same record is declined rather than overwriting the first one's window. A version guard would be wrong besides: nothing pins a version here, the administrator is acting on a record rather than on a value they read",

	// Archive is an absolute idempotent transition: the write sets
	// archived_at unconditionally (no state derived from a pre-read),
	// so concurrent archives converge on the same terminal row and the
	// in-transaction visibility read supplies the NotFound.
	//
	// That rationale answers "do two archives race each other", and it is
	// still true of every entry below. It does NOT answer "did this archive
	// land on the record the decider judged" — and for the six types
	// archive_record stages a human confirmation for (person, organization,
	// deal, project, relationship, activity) that is the question, because a
	// concurrent UPDATE in the window between a released approval and the
	// write changes the record without racing the archive at all. Those six
	// now carry the version the approval was granted against and are no longer
	// waived here. The entries that remain are types no approval ever pins.

	// Both geocode writes are LAST-WRITER-WINS on purpose, and a version guard
	// would make them worse rather than safer.
	//
	// RecordGeocode writes the answer for one address, identified by
	// geocode_input_hash. Two workers racing on the same address write the same
	// point; two racing on DIFFERENT addresses means the address changed
	// mid-flight, and the later write is the one that matches the row — which
	// is what last-writer-wins gives. A version guard would fail one of them and
	// leave the company holding coordinates for an address it no longer has.
	//
	// invalidateGeocodeInTx only ever moves a status TOWARD stale, never away,
	// and it runs inside the address writer's own transaction — the row is
	// already locked by the patch that changed the address.
	"internal/modules/people:recordGeocodeAfter":    "guarded by a re-read rather than a version: the transaction rebuilds the address hash from the live columns and writes nothing unless it still matches what was resolved (addressHashInTx). That is a stronger check than a version pin here — a version would refuse a write whose address is unchanged but whose row was touched for some unrelated reason, and accept one whose address moved without bumping it",
	"internal/modules/automation:Archive":           "absolute idempotent archive transition; concurrent archives converge, the visibility pre-read only feeds the audit before-image",
	"internal/modules/collections:ArchiveList":      "absolute idempotent archive transition; the RETURNING + archived_at IS NULL predicate makes a lost race read as already archived",
	"internal/modules/collections:ArchiveSavedView": "absolute idempotent archive transition; the RETURNING + archived_at IS NULL predicate makes a lost race read as already archived",
	"internal/modules/collections:ArchiveTag":       "absolute idempotent archive transition; the RETURNING + archived_at IS NULL predicate makes a lost race read as already archived",
	"internal/modules/deals:ArchiveProduct":         "absolute idempotent archive transition; concurrent archives converge, the visibility pre-read only feeds the response",
	"internal/modules/deals:ArchiveOfferTemplate":   "absolute idempotent archive transition; concurrent archives converge, the visibility pre-read only feeds the response",
	"internal/modules/quotas:ArchiveQuota":          "absolute idempotent archive transition; concurrent archives converge, the visibility pre-read only feeds the response",
	"internal/modules/webhooks:ArchiveSubscription": "absolute idempotent archive transition; the RETURNING + archived_at IS NULL predicate makes a lost race read as already archived (delivery stops at archive)",
	"internal/modules/signals:ArchiveSignal":        "absolute idempotent archive transition; concurrent archives converge, the visibility pre-read only feeds the response",

	// Writes that run UNDER a lock taken by their caller (or a lock the
	// function's own helper mints) — the guard exists, one frame up.
	"internal/modules/approvals:applyEditedPayload":             "runs only inside decideInTx, after its FOR UPDATE lock on the approval row",
	"internal/modules/customfields:Rename":                      "runs under the catalog row lock minted by lockField (FOR UPDATE before every decision read), with the If-Match version checked under that lock",
	"internal/modules/customfields:Retire":                      "runs under the catalog row lock minted by lockField (FOR UPDATE before every decision read); the flip is an absolute idempotent transition besides",
	"internal/modules/customfields:setOptionsInTx":              "runs under the catalog row lock minted by lockPicklistField (FOR UPDATE before every decision read), plus the per-table advisory lock serializeSchemaChange mints",
	"internal/modules/deals:ArchiveOffer":                       "runs under the offer row lock taken by visibleOfferLocked, and the write itself is an absolute archive transition",
	"internal/modules/deals:UpdateOfferLineItem":                "runs under the parent offer's row lock taken by visibleOfferLocked, which serializes every line edit",
	"internal/modules/deals:recomputeOfferTotals":               "every caller holds the offer row lock via visibleOfferLocked, except createOfferTx where the offer row was inserted in the same transaction",
	"internal/modules/people:absorbOrgReferences":               "runs under the merge pair lock (storekit.LockPair on both organization rows) taken by MergeOrganization",
	"internal/modules/signals:dropUnattributable":               "runs only inside resolveTx, under its signal row lock (storekit.LockRow before the terminal-state pre-read)",
	"internal/modules/signals:resolveToOrg":                     "runs only inside resolveTx, under its signal row lock (storekit.LockRow before the terminal-state pre-read)",
	"internal/modules/signals:flagAmbiguous":                    "runs only inside resolveTx, under its signal row lock (storekit.LockRow before the terminal-state pre-read)",
	"internal/modules/ai:SetBuildStage":                         "stage is display-only forward progress; the status=running predicate makes a raced write a harmless no-op",
	"internal/modules/ai:DeferBuild":                            "the status=running predicate is the CAS: a build already finished or re-claimed matches zero rows and the deferral is dropped",
	"internal/modules/ai:ClaimBuild":                            "the claim's own status predicate IS the CAS — queued, or deferred past its window, or running past reclaimAfter — so a build another worker already claimed matches zero rows and this claim is correctly dropped. It DOES hold a row lock, on voice_profile, and that lock is the documented profile-then-build ordering CompleteBuild takes rather than a guard on voice_build; the narrowed witness stopped crediting it for the wrong table, which is the narrowing working",
	"internal/modules/privacy:liftAndEraseHeldRecord":           "the `restricted_at IS NOT NULL` predicate plus the caller's own due-clause, with RETURNING, is the CAS: the expiry sweep and the controller's release both match nothing when the other got there first, so one erasure is audited once. A version guard would be wrong here — a held record has no optimistic-concurrency editor, and the deciding administrator is acting on a list, not on a version they read",
	"internal/modules/activities:StampCorrespondenceForProject": "the `retention_class IS NULL` predicate IS the CAS, and losing the race is the correct outcome rather than a conflict to report: a deal that qualified the same activity first has already written the same class, and the second writer's evidence row still lands beside it. A version guard would be actively wrong — this runs inside the transaction that files the link, so refusing it on a stale version would abort the filing and leave the correspondence unclassified, which is the one failure mode the stamp exists to prevent",
	"internal/modules/ai:finishBuildTx":                         "runs only under its callers' row lock — ClaimBuild's claim UPDATE or the FOR UPDATE pre-read in FailBuild/CompleteBuild, same transaction",
	"internal/modules/ai:persistBuildVersion":                   "runs only inside CompleteBuild's transaction, under its voice_profile row lock (storekit.LockRow before the pre-read)",

	// Writes that are race-free by their own shape.
	"internal/modules/capture:AdvanceChannelPollOffsetTx": "single-statement monotone cursor: the `poll_offset < $2` predicate IS the CAS, so a writer holding a lower offset than the row already carries matches zero rows and its advance is correctly dropped. A version guard would be wrong here — the poll cursor is not optimistically concurrent state an operator edits, and bumping version on it would fire the send path's binding fence on every inbound message (0151's trigger comment)",
	"internal/modules/privacy:anonymizeSubjectRows":       "terminal absolute write: the erasure overwrites the PII columns regardless of concurrent state, by design",
	"internal/modules/privacy:anonymizeLeadTwins":         "terminal absolute write: the same erasure statement as anonymizeSubjectRows, extracted for length — it overwrites the lead twin's PII columns regardless of concurrent state, by design",
	"internal/modules/privacy:archiveActivity":            "terminal absolute write: the retention sweep archives an over-age activity regardless of concurrent state, by design — a concurrent edit does not make the record younger",
	"internal/modules/privacy:archiveDeal":                "terminal absolute write: the retention sweep archives an over-age lost/won deal regardless of concurrent state, by design",
	"internal/modules/privacy:anonymizeLead":              "terminal absolute write: the retention sweep anonymizes an over-age lead regardless of concurrent state, by design, and its selector already excludes an already-anonymized row",
	"internal/modules/privacy:eraseActivityContent":       "terminal absolute write: the sweep's activity/erase action empties the body and stamps the tombstone subject regardless of concurrent state, by design",
	"internal/modules/privacy:anonymizePersonRecord":      "terminal absolute write: the sweep's person/anonymize action overwrites the PII columns regardless of concurrent state, by design",

	// A function the package-level folding attributes a statement to without its
	// also executing it. Ratified here rather than smoothed away in the reader,
	// because the shape recurs wherever a statement table is handed to an
	// executor by value instead of by name, and a reader taught to excuse it
	// would excuse the writers that DO need judging along with it.
	//
	// The lock this names is the ORGANIZATION row's, not the workspace row's.
	// SaveCompany also takes lockCompanyState — `SELECT id FROM workspace … FOR
	// UPDATE` — and that one is deliberately not what ratifies this: it names a
	// different table from the one being written, which is exactly the free
	// credit the table-scoped witness was narrowed to stop giving.
	"internal/modules/people:writeCompanyFields": "runs under the row lock its callers hold on the organization being written. Both — SaveCompany and applySiteReadConfirmation — reach it through resolveOrCreateAnchor, whose anchorOrganization(ctx, tx, true) is `SELECT id FROM organization WHERE is_anchor AND archived_at IS NULL FOR UPDATE`, held for the rest of the transaction, so two company saves serialize on the row rather than racing on it. The census cannot see that lock for two reasons at once: it is taken two frames up, and anchorOrganization assembles the statement with `query += \" FOR UPDATE\"`, which no reading here folds. This function is attributed a statement by NAME rather than by execution besides — it iterates companyFields and hands each one to setCompanyColumn",
})

// guardMarkers are the identifiers whose presence in the same function
// witnesses a concurrency guard: the storekit guarded-apply family and the
// RowsAffected conditional-write (CAS) check.
//
// LockRow and LockPair are NOT here, and that is the point: a lock witnesses a
// guard only when it names the table the UPDATE writes, which a bare identifier
// match cannot tell. They are matched by lockedTables below instead.
//
// RowsAffected stays function-scoped, and the reason is measured rather than
// assumed. 28 by-id updates in this tree are credited by it alone, and
// associating a command tag with the statement that produced it needs dataflow
// this AST walk does not do — a tag is assigned in one statement and tested in
// another, often through a named variable, sometimes inside a helper. Narrowing
// it would therefore mean 28 ratified waivers, which is a bigger change than
// this witness is worth on its own; it is recorded here so the next reader knows
// the looseness is a decision with a number behind it.
//
// That 28 was counted against a reader that saw only the literals typed in a
// body, so read it as a floor rather than as the figure. A marker now absolves
// every statement the function NAMES as well — one RowsAffected on an unrelated
// write credits a whole hoisted statement table the same function indexes. The
// looseness therefore grew with the reach, and narrowing it is its own change
// rather than a line to slip into this one.
var guardMarkers = map[string]bool{
	"ApplyWithVersion": true,
	"ApplyGuarded":     true,
	"ApplyLocked":      true,
	"RowsAffected":     true,
}

// lockMarkers are the row-lock mints, whose first string argument names the
// table they lock.
var lockMarkers = map[string]bool{"LockRow": true, "LockPair": true}

// lockingRead matches a statement that takes row locks, case-insensitively
// because Postgres accepts lowercase keywords and a witness that missed `for
// update` would report a guarded function as unguarded.
var lockingRead = regexp.MustCompile(`(?i)\bFOR\s+UPDATE\b`)

// advisoryLock names no table — it is a workspace-wide mutex — so it keeps an
// unattributed credit rather than being resolved to one.
var advisoryLock = regexp.MustCompile(`(?i)pg_advisory_xact_lock`)

// lockedReadTables pulls the tables a locking read could be locking: every FROM
// and JOIN target in the statement.
var lockedReadTables = regexp.MustCompile(`(?is)\b(?:FROM|JOIN)\s+([a-z_]+)`)

// forUpdateOf marks `FOR UPDATE OF a, b`, which locks only the aliases it
// names. Resolving an alias back to its table needs a parser, so a statement
// carrying one is credited with NOTHING and its call site takes a waiver — the
// safe direction, because the alternative is crediting a table the statement
// does not lock.
var forUpdateOf = regexp.MustCompile(`(?i)\bFOR\s+UPDATE\s+OF\b`)

// lockedByRead returns the tables a locking read witnesses a guard on.
//
// A statement that reads from exactly one table locks that table and nothing
// else. One that JOINs is ambiguous — `FOR UPDATE` locks every table in the
// join, but a test that credited them all would hand a free pass to whichever
// of them the caller then UPDATEs — so it credits nothing and the call site
// says why in a waiver. Under-crediting fails loudly; over-crediting is the
// failure this whole narrowing exists to remove, and it fails silently.
func lockedByRead(lit string) []string {
	if !lockingRead.MatchString(lit) || forUpdateOf.MatchString(lit) {
		return nil
	}
	seen := map[string]bool{}
	var tables []string
	for _, m := range lockedReadTables.FindAllStringSubmatch(lit, -1) {
		if table := strings.ToLower(m[1]); !seen[table] {
			seen[table] = true
			tables = append(tables, table)
		}
	}
	if len(tables) != 1 {
		return nil
	}
	return tables
}

// lockTableConsts is stringConsts (versionguard_test.go) memoised per package
// directory, so resolving a lock's table constant costs one parse of the
// package rather than one per function inspected.
//
// Both spellings are live in this tree — storekit.LockRow(ctx, tx, "stage", …)
// and storekit.LockRow(ctx, tx, entityPerson, …) — and a witness that only
// understood the literal form would credit every constant-form lock for free.
// That is the same over-recognition this narrowing exists to remove, arriving
// through the back door.
//
// PACKAGE-scoped, where writeauthority_test.go's packageStringConsts is
// deliberately FILE-scoped. The difference is not an oversight: that gate reads
// a const and the probes using it as one unit that sits together, and widening
// it would let an unrelated same-named const answer. Here the lock and the
// entity const are routinely in different files of one module — finance mints
// entityInvoice once and locks with it from both mirror.go and sync.go — so
// file scope would resolve nothing and hand back the free credit this narrowing
// exists to remove.
func lockTableConsts(t *testing.T, fset *token.FileSet, cache map[string]map[string]string, dir string) map[string]string {
	t.Helper()
	if consts, ok := cache[dir]; ok {
		return consts
	}
	consts := stringConsts(t, fset, parsePackageDir(t, fset, dir))
	cache[dir] = consts
	return consts
}

// statementsJudged is every SQL statement a function answers for: the string
// literals written in its own body, plus the package-level statements it names.
//
// A statement hoisted to a package-level table is executed by whoever names it,
// and the guard is that function's to carry. Reading only the body's literals
// left those statements outside the census entirely — not reported as a gap but
// silently absent, which is the one way a census must not fail. The sibling
// census in orgrenamerecheck_test.go already folds them; this one had stayed
// narrower, and eight organization writes were sitting in the difference.
//
// A name the function DECLARES is its own, whatever the package calls something
// of the same name — attributing by spelling alone hands a package-level
// statement to any function with a local of that name.
//
// Held by: TestTheGuardCensusJudgesEveryStatementAFunctionAnswersFor
// (backend/gates/updateguardcases_test.go)
func statementsJudged(fn *ast.FuncDecl, held map[string][]string, imported []map[string][]string) []string {
	walker := &lexicalStatements{held: held, imported: imported, locals: map[string]string{}}
	walker.open()
	walker.declareFields(fn.Type.Params, fn.Type.Results)
	if fn.Recv != nil {
		walker.declareFields(fn.Recv)
	}
	ast.Walk(walker, fn.Body)
	return walker.out
}

// lexicalStatements resolves each identifier against Go's own scoping while it
// gathers, which declaredNames deliberately does not: that helper treats a name
// declared anywhere in a function as shadowing the package value throughout, and
// says so, because the privacy census it serves would rather miss a statement on
// both sides than add one to the side that does not write it.
//
// This census wants the opposite conservatism. A local declared inside one
// branch would suppress a package-level statement named before it and outside
// it, and the whole point here is that a statement nobody judges produces no
// finding — only a smaller silence. So scope is tracked rather than approximated.
type lexicalStatements struct {
	held map[string][]string
	// imported is what the in-module packages this file imports hold, one map
	// each. A qualified name is looked up in ALL of them rather than in the one
	// its base resolves to: a package's declared name need not match its
	// directory, and getting that mapping subtly wrong would drop the statement
	// silently, where looking too widely at worst raises a finding somebody
	// answers.
	imported []map[string][]string
	scopes   []scope
	// locals is what a string-valued local has been built up to so far, so an
	// `x := "…"` / `x += "…"` pair is read as the one statement it becomes.
	// Keyed by name across scopes on purpose: reconstructing more statements
	// than a function really sends over-reports, and over-reporting is the
	// direction that produces a finding somebody can answer.
	locals map[string]string
	out    []string
}

// scope is one block's bindings: the names it declares, and what each of those
// displaced in locals so close can put it back.
type scope struct {
	declared  map[string]bool
	displaced map[string]*string
}

// foldAppend records what `target += addition` builds the target up to, and
// reads the accumulated whole. The parts are read separately anyway; this is
// the reading that carries a SET and its WHERE together.
func (l *lexicalStatements) foldAppend(target, addition ast.Expr) {
	name, named := target.(*ast.Ident)
	if !named {
		return
	}
	added, readable := gatekit.ConcatenatedString(addition)
	if !readable {
		return
	}
	l.locals[name.Name] += added
	l.out = append(l.out, l.locals[name.Name])
}

// seedLocal starts that accumulation from a declaration's own value.
func (l *lexicalStatements) seedLocal(target, value ast.Expr) {
	name, named := target.(*ast.Ident)
	if !named {
		return
	}
	if seed, readable := gatekit.ConcatenatedString(value); readable {
		l.locals[name.Name] = seed
	}
}

func (l *lexicalStatements) open() {
	l.scopes = append(l.scopes, scope{declared: map[string]bool{}, displaced: map[string]*string{}})
}

// close puts back what this scope's declarations displaced. An inner
// `statement := "SELECT 1"` must not survive its block: the outer name goes on
// being built up after it, and an accumulation that kept the inner value folds
// to a statement with no SET clause — one the census then skips.
func (l *lexicalStatements) close() {
	closing := l.scopes[len(l.scopes)-1]
	for name, previous := range closing.displaced {
		if previous == nil {
			delete(l.locals, name)
			continue
		}
		l.locals[name] = *previous
	}
	l.scopes = l.scopes[:len(l.scopes)-1]
}

func (l *lexicalStatements) declare(expr ast.Expr) {
	ident, isIdent := expr.(*ast.Ident)
	if !isIdent || ident.Name == "_" {
		return
	}
	current := l.scopes[len(l.scopes)-1]
	current.declared[ident.Name] = true
	if _, already := current.displaced[ident.Name]; already {
		return
	}
	if previous, held := l.locals[ident.Name]; held {
		current.displaced[ident.Name] = &previous
		return
	}
	current.displaced[ident.Name] = nil
}

func (l *lexicalStatements) declareFields(lists ...*ast.FieldList) {
	for _, list := range lists {
		if list == nil {
			continue
		}
		for _, field := range list.List {
			for _, name := range field.Names {
				l.declare(name)
			}
		}
	}
}

func (l *lexicalStatements) shadowed(name string) bool {
	for _, enclosing := range l.scopes {
		if enclosing.declared[name] {
			return true
		}
	}
	return false
}

// Visit gathers on the way down, so a name is read against the bindings that
// stood where it was written: `held` used before a later `held := …` in the same
// block is still the package's, which is what Go compiles it to.
func (l *lexicalStatements) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case nil:
		return nil
	case *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt,
		*ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt,
		*ast.CaseClause, *ast.CommClause:
		l.open()
		return scopeCloser{l}
	case *ast.FuncLit:
		l.open()
		l.declareFields(n.Type.Params, n.Type.Results)
		return scopeCloser{l}
	case *ast.RangeStmt:
		// Its own scope, and its own declaration: the loop variables are
		// declared by the clause rather than by an AssignStmt below it, so the
		// generic arm above would open the scope and then never put them in it.
		// The range expression is evaluated in the OUTER bindings.
		l.open()
		defer l.close()
		ast.Walk(l, n.X)
		if n.Tok == token.DEFINE {
			l.declare(n.Key)
			l.declare(n.Value)
		} else {
			for _, target := range []ast.Expr{n.Key, n.Value} {
				if target != nil {
					ast.Walk(l, target)
				}
			}
		}
		ast.Walk(l, n.Body)
		return nil
	case *ast.BinaryExpr:
		// The same fold packageLevelStatements does, for the same reason:
		// neither half of `UPDATE x SET a = $1 ` + `WHERE id = $2` carries the
		// statement, so a census matching a shape sees nothing in either. The
		// walk continues into the parts as well — adding a reading can only
		// widen what is judged.
		if n.Op == token.ADD {
			if folded, readable := gatekit.ConcatenatedString(n); readable {
				l.out = append(l.out, folded)
			}
		}
		return l
	case *ast.LabeledStmt:
		// A label shares the identifier namespace with nothing this reads —
		// neither where it is declared nor where a break or continue names it.
		ast.Walk(l, n.Stmt)
		return nil
	case *ast.BranchStmt:
		return nil
	case *ast.SelectorExpr:
		// `spec.update` names a FIELD, not this package's `update`, so the Sel
		// is not read as a local name. But `storekit.ProbeRenameOrg` is a
		// statement held one import away, and dropping the Sel outright left
		// that silence exactly where this change closed the same one inside a
		// package. So the Sel is looked up in what the file's in-module imports
		// hold — a field name matches nothing there, and a qualified statement
		// matches. The base is walked either way, because that half can be a
		// package value of this package's own.
		for _, elsewhere := range l.imported {
			l.out = append(l.out, elsewhere[n.Sel.Name]...)
		}
		ast.Walk(l, n.X)
		return nil
	case *ast.KeyValueExpr:
		// A struct literal's field name is not a variable reference either, and
		// the AST cannot tell one from a map key without types. An identifier
		// key is therefore left unread: a package-level SQL statement used as a
		// map KEY would be a statement nobody sends, so declining it costs no
		// coverage, while reading it credits every `{update: …}` in the tree.
		if _, named := n.Key.(*ast.Ident); !named {
			ast.Walk(l, n.Key)
		}
		ast.Walk(l, n.Value)
		return nil
	case *ast.AssignStmt:
		// `statement += " FOR UPDATE"` is the third spelling of the same
		// assembly, and the one anchorOrganization uses to add the row lock that
		// guards the company form. Each half is read on its own below; this
		// folds them so the whole is read too.
		if n.Tok == token.ADD_ASSIGN && len(n.Lhs) == 1 && len(n.Rhs) == 1 {
			l.foldAppend(n.Lhs[0], n.Rhs[0])
		}
		// The right-hand side is evaluated against the bindings that stand
		// BEFORE the declaration, so it is walked first and the names are
		// declared after: `held := held` reads the package's value.
		for _, rhs := range n.Rhs {
			ast.Walk(l, rhs)
		}
		if n.Tok == token.DEFINE {
			// declare BEFORE seedLocal: declaring is what records the value
			// this name displaces, and seeding is what displaces it. The other
			// order records the new value as the old one, so closing the scope
			// restores nothing.
			for _, lhs := range n.Lhs {
				l.declare(lhs)
			}
			if len(n.Lhs) == len(n.Rhs) {
				for i, lhs := range n.Lhs {
					l.seedLocal(lhs, n.Rhs[i])
				}
			}
		} else {
			for _, lhs := range n.Lhs {
				ast.Walk(l, lhs)
			}
		}
		return nil
	case *ast.ValueSpec:
		for _, value := range n.Values {
			ast.Walk(l, value)
		}
		for _, name := range n.Names {
			l.declare(name)
		}
		if len(n.Names) == len(n.Values) {
			for i, name := range n.Names {
				l.seedLocal(name, n.Values[i])
			}
		}
		return nil
	case *ast.Ident:
		if !l.shadowed(n.Name) {
			l.out = append(l.out, l.held[n.Name]...)
		}
		return nil
	case *ast.BasicLit:
		if n.Kind == token.STRING {
			if lit, err := strconv.Unquote(n.Value); err == nil {
				l.out = append(l.out, lit)
			}
		}
		return nil
	}
	return l
}

// scopeCloser pops the scope its node opened once ast.Walk has finished that
// node's children. Nested inside the walker rather than folded into it because
// ast.Walk signals "children done" by calling Visit(nil) on the visitor a node
// RETURNED, and a walker that popped on every such call would pop for every
// plain node too.
type scopeCloser struct{ walker *lexicalStatements }

func (c scopeCloser) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		c.walker.close()
		return nil
	}
	return c.walker.Visit(node)
}

// modulePrefix is the backend module's own import path. An import outside it is
// a dependency whose statements are not this tree's to judge.
const modulePrefix = "github.com/margince/margince/backend/"

// inModuleImportDirs are the directories of the backend packages a file imports,
// which is where a qualified statement name can have been declared.
func inModuleImportDirs(file *ast.File) []string {
	var dirs []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !strings.HasPrefix(path, modulePrefix) {
			continue
		}
		dirs = append(dirs, strings.TrimPrefix(path, modulePrefix))
	}
	return dirs
}

// heldStatements are the SQL statements a package holds in its package-level
// vars and consts, keyed by the name that holds them, read once per package.
//
// The reading is the one packageCallGraph uses, rather than a second answer to
// "what statement does this name hold" — a census whose reader is narrower than
// its sibling's reports a clean tree over what the sibling can see.
func heldStatements(t *testing.T, cache map[string]map[string][]string, dir string) map[string][]string {
	t.Helper()
	if held, ok := cache[dir]; ok {
		return held
	}
	held := packageLevelStatements(parsePackageFiles(t, dir))
	cache[dir] = held
	return held
}

// lockedTable resolves the table a LockRow/LockPair call names: a string
// literal, or an identifier declared as a package-level string constant. An
// argument it cannot resolve returns "", which the caller treats as an
// unattributable lock rather than as a guard for every table.
func lockedTable(call *ast.CallExpr, consts map[string]string) string {
	for _, a := range call.Args {
		switch arg := a.(type) {
		case *ast.BasicLit:
			if arg.Kind != token.STRING {
				continue
			}
			if v, err := strconv.Unquote(arg.Value); err == nil && v != "" && !strings.ContainsAny(v, " \t\n") {
				return v
			}
		case *ast.Ident:
			if v, ok := consts[arg.Name]; ok && v != "" && !strings.ContainsAny(v, " \t\n") {
				return v
			}
		}
	}
	return ""
}

func TestEveryByIDUpdateCarriesAConcurrencyGuard(t *testing.T) {
	t.Parallel()
	defer unguardedByIDUpdates.AssertAllMatched(t)
	versioned := versionedTables(t)
	fset := token.NewFileSet()
	constCache := map[string]map[string]string{}
	heldCache := map[string]map[string][]string{}
	judged := 0
	for _, root := range []string{"internal/modules", "internal/compose", "internal/platform"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			// Read once per FILE rather than per function: the imports are the
			// file's, and heldStatements caches per package anyway.
			var imported []map[string][]string
			for _, dir := range inModuleImportDirs(file) {
				imported = append(imported, heldStatements(t, heldCache, dir))
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				var guarded bool
				updated := map[string]bool{}
				locked := map[string]bool{}
				readStatement := func(lit string) {
					if m := byIDUpdate.FindStringSubmatch(lit); m != nil && versioned[m[1]] {
						updated[m[1]] = true
						// A version predicate in the statement's own WHERE
						// IS the compare-and-set this gate asks for, and the
						// most direct form of it — the UPDATE matches no row
						// unless the version is still the one that was read.
						// Crediting only the storekit helpers would push a
						// correct fix toward a waiver.
						if w := whereClause.FindStringSubmatch(lit); w != nil &&
							versionPredicate.MatchString(w[1]) {
							guarded = true
						}
					}
					// A locking read guards the table it READS. An advisory
					// lock names no table at all and is a workspace-wide
					// mutex, so it keeps its unattributed credit.
					for _, table := range lockedByRead(lit) {
						locked[table] = true
					}
					if advisoryLock.MatchString(lit) {
						guarded = true
					}
				}
				for _, lit := range statementsJudged(fn, heldStatements(t, heldCache, filepath.Dir(path)), imported) {
					readStatement(lit)
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					switch node := n.(type) {
					case *ast.CallExpr:
						sel, ok := node.Fun.(*ast.SelectorExpr)
						if !ok || !lockMarkers[sel.Sel.Name] {
							return true
						}
						if tbl := lockedTable(node, lockTableConsts(t, fset, constCache, filepath.Dir(path))); tbl != "" {
							locked[tbl] = true
						}
					case *ast.SelectorExpr:
						if guardMarkers[node.Sel.Name] {
							guarded = true
						}
					}
					return true
				})
				// The lock has to name the table being written. A function that
				// locks one row and updates another is the shape this witness
				// used to credit for free.
				for tbl := range updated {
					if locked[tbl] {
						guarded = true
					}
				}
				if len(updated) == 0 {
					continue
				}
				judged++
				if !guarded {
					key := filepath.ToSlash(filepath.Dir(path)) + ":" + fn.Name.Name
					if unguardedByIDUpdates.Waived(t, key) {
						continue
					}
					t.Errorf("%s: %s runs a by-id UPDATE with no concurrency guard — use storekit.ApplyGuarded/ApplyWithVersion, lock the row first (LockRow/LockPair/FOR UPDATE/advisory lock), or check RowsAffected as a CAS; a real exception is ratified in unguardedByIDUpdates",
						path, fn.Name.Name)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// A census that judged nothing certifies nothing, and this one has two ways
	// to go quiet: the walk can stop finding files, and the reader can stop
	// finding statements inside them. Neither produces a finding on its own —
	// they produce a smaller silence, which is what the floor is for. It sits
	// below the real count so it catches a broken reader rather than a tree
	// that has changed.
	if judged < byIDUpdateFloor {
		t.Fatalf("this census judged %d function(s) running a by-id UPDATE and expects at least %d — "+
			"the reader has stopped seeing statements rather than the tree having lost them",
			judged, byIDUpdateFloor)
	}
}
