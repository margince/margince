// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// WHICH table a row-scope call bounds, and which column names a reference to
// one.
//
// The obligation used to be table-agnostic: a function that projected an
// `organization_id` satisfied it by reaching a probe over `deal`. That is not a
// weaker check, it is a different one — #1876's read named an organization and
// scoped a deal, and a gate counting probes rather than matching them would
// have gone green over it and then certified it.
//
// And the columns it looked for were named after their table. An FK named for
// its ROLE — `partner_org_id` was one of the three references #1876 fixed —
// escaped entirely: the extractor read the column's own words as a table name,
// found no such table, and dropped the site with no census entry and no waiver.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// anyTable is the scope a spelling applies when it cannot be attributed to one
// table. It satisfies a reference to any of them, which is the honest reading:
// ScopeClause and OwnerPredicate render over whatever alias the caller gives
// them, so the table is the caller's and this gate cannot see it.
const anyTable = "*"

// scopeSpellingTable says which table each row-scope spelling bounds.
//
// Three shapes, and the difference is in platform/auth's own signatures:
//
//   - a table NAMED BY ARGUMENT — EnsureVisible(ctx, tx, table, id) — where the
//     value is the string literal at that index, and a non-literal means a
//     caller allowlisting at its own seam, which this gate reads as any table;
//   - a table IMPLIED BY THE NAME — EnsureActivityVisible bounds activities and
//     nothing else, so the name is the answer;
//   - no table at all, which is anyTable above.
//
// TestEveryRowScopeSpellingSaysWhichTableItBounds holds this map against
// rowScopeSpellings in both directions, so a spelling added to one and not the
// other fails rather than defaulting to a silent wildcard.
var scopeSpellingTable = map[string]scopeSpelling{
	"ScopeClause":      {table: anyTable},
	"ScopeClauseFor":   {argument: 1},
	"OwnerPredicate":   {table: anyTable},
	"VisiblePredicate": {argument: 1},

	"EnsureVisible":                 {argument: 2},
	"EnsureVisibleLive":             {argument: 2},
	"EnsureVisibleForSubjectRights": {argument: 2},
	"EnsureLinkTarget":              {argument: 2},
	"VisibleTo":                     {argument: 2},
	// The link target's table is the caller's to name in its own allowlist;
	// this clause renders over the alias it is given.
	"LinkTargetVisibleClause": {table: anyTable},

	"ActivityDiscoverClause":           {table: "activity"},
	"ActivityContentClause":            {table: "activity"},
	"EnsureActivityVisible":            {table: "activity"},
	"EnsureActivityVisibleLive":        {table: "activity"},
	"EnsureActivityContentVisible":     {table: "activity"},
	"EnsureActivityContentVisibleLive": {table: "activity"},

	"SignalScopeClause":       {table: "signal"},
	"EnsureSignalVisible":     {table: "signal"},
	"EnsureSignalVisibleLive": {table: "signal"},

	// The edge conjunction bounds the relationship by its ENDPOINTS: each
	// endpoint column is tested with VisiblePredicate over its own table, so a
	// person or organization id projected from an edge that passed it is
	// bounded as surely as one from a direct scope clause. Reading it as
	// "relationship only" would report those projections unscoped and send the
	// next author to add a second clause over a column the conjunction covers.
	"RelationshipEndpointScope": {endpoints: true},
	"EnsureRelationshipVisible": {endpoints: true},
	"EdgeReadScope":             {endpoints: true},
}

// scopeSpelling is one spelling's table: fixed, read off an argument, or the
// edge conjunction's — which bounds the relationship AND every endpoint it
// names, so its tables are read out of platform/auth's own endpoint list.
type scopeSpelling struct {
	table string
	// argument is the 0-based index of the parameter naming the table, used
	// when table is empty and endpoints is false.
	argument int
	// endpoints marks the spellings whose clause is the endpoint conjunction:
	// every endpoint column is bounded by VisiblePredicate over its own table,
	// so a person id projected from an edge that passed it IS bounded.
	endpoints bool
}

func TestEveryRowScopeSpellingSaysWhichTableItBounds(t *testing.T) {
	t.Parallel()
	for spelling := range rowScopeSpellings {
		if _, classified := scopeSpellingTable[spelling]; !classified {
			t.Errorf("%s applies a row scope and this gate cannot say which table it bounds, so every "+
				"reference would count it — classify it in scopeSpellingTable, with anyTable only when "+
				"platform/auth's signature genuinely names no table", spelling)
		}
	}
	for spelling := range scopeSpellingTable {
		if !rowScopeSpellings[spelling] {
			t.Errorf("scopeSpellingTable classifies %s, which is not a row-scope spelling any more — "+
				"a classification outliving its subject is a standing answer to a question nobody asks", spelling)
		}
	}
}

// referenceColumn is a column name that holds a reference to a row-scoped
// record, mapped to the table it points at.
//
// DERIVED FROM THE SCHEMA's own FOREIGN KEY declarations rather than from the
// column's spelling, which is what makes a role-named FK visible: the catalog
// says `partner_org_id` references `organization`, where the name says
// `partner_org` and no such table exists.
func referenceColumns(t *testing.T, tables map[string]bool) map[string]string {
	t.Helper()
	catalog, err := os.ReadFile(filepath.Join(repoRoot, headCatalogPath))
	if err != nil {
		t.Fatalf("reading the schema catalog this census derives its columns from: %v", err)
	}
	columns := map[string]string{}
	for _, at := range foreignKeyDeclaration.FindAllStringSubmatch(string(catalog), -1) {
		column, table := at[1], at[2]
		// Composite keys carry a comma and reference a pair; no reference this
		// census is about is one, and half of a composite is not a reference.
		if strings.Contains(column, ",") || !tables[table] {
			continue
		}
		columns[column] = table
	}
	return columns
}

const headCatalogPath = "backend/migrations/testdata/head_catalog.txt"

// foreignKeyDeclaration reads one single-column FK out of the catalog line that
// declares it.
var foreignKeyDeclaration = regexp.MustCompile(`FOREIGN KEY \(([a-z_, ]+)\) REFERENCES ([a-z_]+)\(`)

// The extractor's floor, in the ticket's own words: a regex that silently
// matches nothing reads exactly like a tree with nothing to find.
func TestTheReferenceColumnMapIsDerivedAndReachesTheAliasedNames(t *testing.T) {
	t.Parallel()
	tables := rowScopedTables(t)
	columns := referenceColumns(t, tables)
	if len(columns) < wantMinimumReferenceColumns {
		t.Fatalf("the schema yielded %d reference columns, want at least %d — the catalog's shape moved "+
			"and this census is now reading almost none of it", len(columns), wantMinimumReferenceColumns)
	}
	// The half the old extractor could not see: a column whose own name is not
	// its table's. Without one of these the map is only an expensive spelling
	// of the `<table>_id` regex it replaces.
	aliased := 0
	for column, table := range columns {
		if column != table+"_id" {
			aliased++
		}
	}
	if aliased == 0 {
		t.Error("every reference column is named for its own table, so nothing here covers the case " +
			"this map exists for — an FK named for its ROLE, which the name-derived regex drops silently")
	}
}

// wantMinimumReferenceColumns is far below the real count: it catches a parser
// that has stopped reading the catalog, not a schema that has changed.
const wantMinimumReferenceColumns = 20

// scopedTable resolves which table one row-scope call bounds, from the spelling
// and the arguments it was given.
//
// A table argument that is not a string literal reads as anyTable, and that is
// deliberate: those callers forward a name they allowlist at their own seam
// (link entity types, grant record types, search anchors), and platform/auth
// refuses an unknown one itself. Refusing them here would fail correct code for
// being written the only way it can be.
func scopedTable(spelling string, args []ast.Expr) string {
	classified, known := scopeSpellingTable[spelling]
	if !known {
		// Unclassified is anyTable rather than nothing, so a spelling added to
		// rowScopeSpellings alone weakens this gate instead of breaking the
		// tree — TestEveryRowScopeSpellingSaysWhichTableItBounds is what makes
		// that state fail, and it names the omission where a reader can fix it.
		return anyTable
	}
	if classified.table != "" {
		return classified.table
	}
	if classified.endpoints {
		// One name for the whole conjunction; the caller expands it, because
		// the tables it covers are read from source rather than written here.
		return edgeEndpoints
	}
	if classified.argument >= len(args) {
		return anyTable
	}
	literal, isLiteral := args[classified.argument].(*ast.BasicLit)
	if !isLiteral {
		return anyTable
	}
	table, unquoted := stringConst(literal)
	if !unquoted {
		return anyTable
	}
	return table
}

// edgeEndpoints stands for "the relationship and every endpoint table it
// names". scopedTable returns it as one token and the census expands it, so the
// set stays derived from platform/auth rather than restated here.
const edgeEndpoints = "@edge-endpoints"

// relationshipEndpointTables reads the endpoint tables out of platform/auth's
// own relationshipEndpointColumns, so an endpoint added there widens this
// census without anyone remembering to.
func relationshipEndpointTables(t *testing.T) map[string]bool {
	t.Helper()
	consts := map[string]string{}
	var endpoints []string
	for _, src := range tierFiles(t, rowScopeVocabularyPkg) {
		for _, decl := range src.File.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				if gen.Tok == token.CONST {
					if text, isString := stringConst(value.Values[0]); isString {
						consts[value.Names[0].Name] = text
					}
					continue
				}
				if value.Names[0].Name == "relationshipEndpointColumns" {
					endpoints = endpointTableNames(value.Values[0])
				}
			}
		}
	}
	if len(endpoints) == 0 {
		t.Fatalf("%s declares no relationshipEndpointColumns — teach this gate where the edge's endpoint "+
			"list moved, because reading it as the empty set would report every edge projection unscoped",
			rowScopeVocabularyPkg)
	}
	tables := map[string]bool{"relationship": true}
	for _, name := range endpoints {
		if text, isConst := consts[name]; isConst {
			tables[text] = true
			continue
		}
		tables[name] = true
	}
	return tables
}

// endpointTableNames reads the table of each {column, table} pair as WRITTEN —
// a const by its identifier, a literal by its text.
func endpointTableNames(expr ast.Expr) []string {
	lit, isLit := expr.(*ast.CompositeLit)
	if !isLit {
		return nil
	}
	var names []string
	for _, element := range lit.Elts {
		pair, isPair := element.(*ast.CompositeLit)
		if !isPair || len(pair.Elts) != 2 {
			continue
		}
		switch table := pair.Elts[1].(type) {
		case *ast.Ident:
			names = append(names, table.Name)
		case *ast.BasicLit:
			if text, isString := stringConst(table); isString {
				names = append(names, text)
			}
		}
	}
	return names
}

// The modules tier, measured rather than guessed at.
//
// Extending the census over internal/modules is the change #1984 was filed to
// unblock, and its own header says that tier "holds several times as many
// statements of this shape". This is that number, standing where a reader can
// see it without breaking a gate to find it — and it may only FALL.
//
// It is not the widening. Ninety-five sites is not a waiver list anybody should
// write in one sitting, and a widening that shipped with one would be the green
// check over the same defect this ticket exists to refuse. What it does is stop
// the number growing quietly while the widening waits, and give whoever takes
// it a floor to work against.
func TestTheModulesTierIsMeasuredForTheWideningItIsOwed(t *testing.T) {
	t.Parallel()
	tables := rowScopedTables(t)
	vocab := referenceVocabulary{
		tables:  tables,
		columns: referenceColumns(t, tables),
		edge:    relationshipEndpointTables(t),
	}
	pkgs, sites := referenceSitesIn(t, modulesTier, vocab)
	if len(sites) < wantMinimumModuleSites {
		t.Fatalf("only %d record-reference reads found in %s, want at least %d — the extractor lost its "+
			"source, and a census that reads a smaller tree reports PASS with nothing to notice",
			len(sites), modulesTier, wantMinimumModuleSites)
	}
	unscoped := 0
	for _, site := range sites {
		if !reachesRowScope(pkgs[site.dir].visibleTo(site.recv), site.fn, site.table, map[string]bool{}) {
			unscoped++
		}
	}
	if unscoped > modulesTierUnscopedCeiling {
		t.Errorf("%d of %d record references in %s reach no row scope for the table they name, above the "+
			"recorded %d. A new one is a read handing back a reference it never bounded: bound it, or lower "+
			"nothing and explain here why this tier grew one",
			unscoped, len(sites), modulesTier, modulesTierUnscopedCeiling)
	}
}

const (
	modulesTier = "internal/modules"
	// The floor that catches an extractor which has stopped reading, far below
	// the real count.
	wantMinimumModuleSites = 60
	// What the tier holds today. A ratchet: it may only fall, and it has risen
	// three times. Every rise is a system-principal pass with no seat to narrow
	// to, or a read a write-authority probe bounds a line later, and each is
	// written out below rather than left as a number.
	//
	// THE FIRST RISE, and why it is not a widening anybody should copy.
	// people.RetractMisattributedSignatureFields takes back the profile fields
	// an earlier build wrote off a message the person never sent. It is a
	// repair pass, chosen by DATA — the same SenderPredicate that decides what
	// may be written decides what may still stand — and run by the enrichment
	// job's own system principal, which has no seat for a scope to narrow to.
	// Narrowing it to one caller's rows would leave every other contact wearing
	// another sender's title while the pass reported success, which is the
	// storage-limitation shape the privacy sweeps are exempted for.
	//
	// The scope clause was tried and is the WRONG tool: this path writes, and
	// auth.ScopeClauseFor answers about visibility, so a manual read-share
	// would have admitted a caller who may not edit the record —
	// TestEveryMutationOfAShareableRecordProbesForWriteAuthority says so and is
	// right. The authority it takes instead is auth.Require(person, update),
	// and its confinement is stated beside it in writesWithoutARowProbe.
	//
	// THE SECOND RISE, and the same shape as the first. deals.nextMembers reads
	// the close-date pass's own frozen membership — close_date_run_member rows
	// naming a deal_id — to walk the set that pass froze at its start. It runs
	// under the workspace's system principal, which has no seat for a scope to
	// narrow to, and narrowing it to one rep's deals would leave every other
	// rep's pipeline unassessed while the run reported it had covered
	// everything. That silent under-coverage is the exact defect the run table
	// exists to make visible, so bounding this read by seat would reintroduce it
	// through the gate meant to prevent it.
	//
	// What confines the read instead is the membership itself: the set was
	// decided once by startRun, every row it returns belongs to that set, and
	// the write it feeds re-locks the deal and re-verifies the deal is still
	// open before touching it. The pass's authority is ratified as a whole in
	// writeauthorityreach_test.go under SweepWorkspace and settleMember.
	//
	// THE THIRD RISE, +2, and the same pass again. deals.reversedCorrections
	// and lockReversibleCorrection read deal_correction rows naming a deal_id:
	// the first is the nightly sweep asking "has somebody taken this correction
	// back?", which runs under the same system principal with no seat to narrow
	// to, and narrowing it would let a correction one rep undid be re-applied to
	// another rep's deal.
	//
	// The second is the Undo path, and it IS bounded — by auth.EnsureWritable on
	// the deal, taken inside the same transaction immediately after. It reads
	// unscoped here because the correction row is looked up first, under FOR
	// UPDATE, so a repeated Undo serializes before either caller can act; the
	// visibility answer follows it and refuses everything a row scope would.
	modulesTierUnscopedCeiling = 99
)
