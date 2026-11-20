// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

// The audit write census resolves, refuses or ignores each shape it has met.
// It reads a fixture tree here, not the real one.

package gates

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const fixtureStorekit = `package storekit

import (
	"context"
	"strings"
)

type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (any, error)
}

const TableAudit = "audit_log"

func insertAuditRows(ctx context.Context, tx Tx, action, entityType string) error {
	values := []string{"($1, $2)"}
	_, err := tx.Exec(ctx, "INSERT INTO "+TableAudit+" (action, entity_type) VALUES "+strings.Join(values, ", "), action, entityType)
	return err
}

func Audit(ctx context.Context, tx Tx, action, entityType string) error {
	return insertAuditRows(ctx, tx, action, entityType)
}
`

const censusFixtureImports = `import (
	"context"

	sk "github.com/margince/margince/backend/internal/platform/database/storekit"
)
`

// fixtureShapes are the entity argument shapes the census follows or refuses.
// gatekit:fixture Go source by the path it is written to
var fixtureShapes = map[string]string{
	"backend/internal/modules/shapes/kinds.go": "package shapes\n\nconst kindContact = \"contact\"\n",
	"backend/internal/modules/shapes/structs.go": "package shapes\n\n" + censusFixtureImports + `
type spec struct{ Kind string }
type scanned struct{ Kind string }
type generic struct{ Kind string }
type plain struct{ Kind string }

var keyed = []any{spec{Kind: "note"}, scanned{Kind: "note"}, generic{Kind: "note"}}

func aliasedImport(ctx context.Context)     { sk.Audit(ctx, nil, "create", "lead") }
func constantElsewhere(ctx context.Context) { sk.Audit(ctx, nil, "create", kindContact) }
func auditKind(ctx context.Context, kind string) { sk.Audit(ctx, nil, "create", kind) }
func viaHelper(ctx context.Context)         { auditKind(ctx, "deal") }

func keyedField(ctx context.Context) {
	s := plain{Kind: "note"}
	sk.Audit(ctx, nil, "create", s.Kind)
}

func positional(ctx context.Context) {
	s := spec{"task"}
	sk.Audit(ctx, nil, "create", s.Kind)
}

func wholeScan(ctx context.Context, row interface{ Scan(...any) error }) {
	var s scanned
	_ = row.Scan(&s)
	sk.Audit(ctx, nil, "create", s.Kind)
}

func instantiated(ctx context.Context) {
	var s generic
	s = sk.Collect[generic]()
	sk.Audit(ctx, nil, "create", s.Kind)
}

func asValue(ctx context.Context) {
	audit := sk.Audit
	_ = audit(ctx, nil, "create", "contact")
}

type Store struct{}
type Other struct{}

func (s *Store) write(ctx context.Context, kind string) { sk.Audit(ctx, nil, "create", kind) }
func (o *Other) write(ctx context.Context, kind string) {}

func methodExpression(ctx context.Context) { (*Store).write(nil, ctx, "company") }

func receiverTyped(ctx context.Context) {
	var other Other
	other.write(ctx, "not_a_kind")
	var store Store
	store.write(ctx, "project")
}
`,
	"backend/internal/modules/shapes/hidden_windows.go": "package shapes\n\n" + censusFixtureImports + `
func hiddenOnWindows(ctx context.Context) { sk.Audit(ctx, nil, "create", "windows_only") }
`,
	"backend/internal/modules/shapes/locals.go": "package shapes\n\n" + censusFixtureImports + `
const shadowedKind = "deal"

func auditPair(ctx context.Context, first, second string) {
	sk.Audit(ctx, nil, "create", first)
	sk.Audit(ctx, nil, "create", second)
}

func twoForwarded(ctx context.Context) { auditPair(ctx, "account", "invoice") }

func ranged(ctx context.Context) {
	for _, kind := range []string{"offer", "quote"} {
		sk.Audit(ctx, nil, "create", kind)
	}
}

func mutatedCollection(ctx context.Context) {
	kinds := map[string]bool{"contact": true}
	kinds["mutated_kind"] = true
	for kind := range kinds {
		sk.Audit(ctx, nil, "create", kind)
	}
}

func aliasedCollection(ctx context.Context) {
	kinds := map[string]bool{"contact": true}
	alias := kinds
	alias["aliased_kind"] = true
	for kind := range kinds {
		sk.Audit(ctx, nil, "create", kind)
	}
}

func grow(kinds map[string]bool) { kinds["helper_kind"] = true }

func helperCollection(ctx context.Context) {
	kinds := map[string]bool{"contact": true}
	grow(kinds)
	for kind := range kinds {
		sk.Audit(ctx, nil, "create", kind)
	}
}

func reassignedParam(ctx context.Context, kind string) {
	if kind == "" {
		kind = "lead"
	}
	sk.Audit(ctx, nil, "create", kind)
}

func callsReassigned(ctx context.Context) { reassignedParam(ctx, "contact") }

func shadowedLocal(ctx context.Context) {
	kind := "contact"
	{
		kind := "lead"
		_ = kind
	}
	sk.Audit(ctx, nil, "create", kind)
}

func shadowedConst(ctx context.Context) {
	{
		shadowedKind := "contact"
		_ = shadowedKind
	}
	sk.Audit(ctx, nil, "create", shadowedKind)
}
`,
}

// fixtureDirect are Go statements that do or do not insert audit rows of their
// own, one outcome per file.
// gatekit:fixture Go source by the path it is written to
var fixtureDirect = map[string]string{
	"backend/internal/modules/direct/runs.go": `package direct

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func runs(ctx context.Context, tx pgx.Tx, batch *pgx.Batch) {
	_, _ = tx.Exec(ctx, "INSERT INTO audit_log (id) VALUES ($1)", 1)
	statement := "INSERT INTO audit_log (id) SELECT id FROM t"
	_, _ = tx.Exec(ctx, statement)
	batch.Queue("INSERT INTO audit_log (id) VALUES ($1)", 1)
	_, _ = tx.Prepare(ctx, "audit", "INSERT INTO audit_log (id) VALUES ($1)")
	_, _ = tx.CopyFrom(ctx, pgx.Identifier{"audit_log"}, nil, nil)
}
`,
	"backend/internal/modules/direct/describes.go": `package direct

import (
	"errors"
	"fmt"
)

func describes(err error) error {
	if err != nil {
		return fmt.Errorf("INSERT INTO audit_log values invalid: %w", err)
	}
	return errors.New("INSERT INTO audit_log SELECT failed")
}
`,
	"backend/internal/modules/direct/bound.go": `package direct

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func bound(ctx context.Context, tx pgx.Tx) {
	_, _ = tx.Exec(ctx, "SELECT $1::text", "INSERT INTO audit_log VALUES invalid")
	_, _ = tx.Prepare(ctx, "INSERT INTO audit_log VALUES", "SELECT 1")
}
`,
	"backend/internal/modules/direct/sink.go": `package direct

func sink(record func(string)) {
	record("INSERT INTO audit_log (id) VALUES (1)")
}

func statement() string { return "INSERT INTO audit_log (id) SELECT 1" }
`,
}

// fixturePrivacy are privacy's scrub shapes. A verb is read through a helper,
// a wrapper carries its own, and one verb scrubs nothing. A row's Table sits
// inside and outside clearExtensionRows.
// gatekit:fixture Go source by the path it is written to
var fixturePrivacy = map[string]string{
	"backend/internal/modules/privacy/scrubs.go": "package privacy\n\n" + censusFixtureImports + `
const (
	actionErase     = "erase"
	actionAnonymize = "anonymize"
)

type clearedRow struct{ Table string }

func stamp(ctx context.Context, action, entity string) { sk.Audit(ctx, nil, action, entity) }
func eraseThroughHelper(ctx context.Context)          { stamp(ctx, actionErase, "contact") }
func anonymizeRow(ctx context.Context, entity string) { sk.Audit(ctx, nil, actionAnonymize, entity) }
func anonymizeLead(ctx context.Context)               { anonymizeRow(ctx, "lead") }
func archiveDeal(ctx context.Context)                 { sk.Audit(ctx, nil, "archive", "deal") }

func clearCoreRows(ctx context.Context, rows []clearedRow) {
	for _, row := range rows {
		sk.Audit(ctx, nil, actionAnonymize, row.Table)
	}
}
`,
	extensionClearPath: "package privacy\n\n" + censusFixtureImports + `
func clearExtensionRows(ctx context.Context, rows []clearedRow) {
	for _, row := range rows {
		sk.Audit(ctx, nil, actionAnonymize, row.Table)
	}
}
`,
}

// writeFixture writes each file, by its repository-relative path, under a new
// root, and answers the root.
func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for path, source := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// fixtureCensus reads a census of the given files beside the storekit seed.
func fixtureCensus(t *testing.T, files map[string]string) *auditCensus {
	t.Helper()
	tree := map[string]string{auditSeedDir + "/storekit.go": fixtureStorekit}
	maps.Copy(tree, files)
	c, failed := newAuditCensus(writeFixture(t, tree))
	if failed != "" {
		t.Fatal(failed)
	}
	return c
}

// fixtureSite is what a census says of the sites one function makes.
type fixtureSite struct {
	types           []string
	opaque, asValue bool
}

func sitesByFunc(c *auditCensus) map[string]fixtureSite {
	out := map[string]fixtureSite{}
	for _, site := range c.sites {
		got := out[site.fn]
		got.types = append(got.types, site.types...)
		got.opaque = got.opaque || (site.opaque != "" && !site.asValue)
		got.asValue = got.asValue || site.asValue
		out[site.fn] = got
	}
	for fn, got := range out {
		slices.Sort(got.types)
		out[fn] = got
	}
	return out
}

func TestTheAuditCensusResolvesOrRefusesEachEntityShape(t *testing.T) {
	t.Parallel()
	sites := sitesByFunc(fixtureCensus(t, fixtureShapes))
	cases := []struct {
		fn   string
		want fixtureSite
	}{
		{"aliasedImport", fixtureSite{types: []string{"lead"}}},
		{"constantElsewhere", fixtureSite{types: []string{"contact"}}},
		{"viaHelper", fixtureSite{types: []string{"deal"}}},
		{"keyedField", fixtureSite{types: []string{"note"}}},
		{"positional", fixtureSite{opaque: true}},
		{"wholeScan", fixtureSite{opaque: true}},
		{"instantiated", fixtureSite{opaque: true}},
		{"asValue", fixtureSite{asValue: true}},
		{"methodExpression", fixtureSite{types: []string{"company"}}},
		{"receiverTyped", fixtureSite{types: []string{"project"}}},
		{"twoForwarded", fixtureSite{types: []string{"account", "invoice"}}},
		{"ranged", fixtureSite{types: []string{"offer", "quote"}}},
		{"mutatedCollection", fixtureSite{opaque: true}},
		{"aliasedCollection", fixtureSite{opaque: true}},
		{"helperCollection", fixtureSite{opaque: true}},
		{"reassignedParam", fixtureSite{types: []string{"contact", "lead"}}},
		{"shadowedLocal", fixtureSite{opaque: true}},
		{"shadowedConst", fixtureSite{opaque: true}},
	}
	for _, tc := range cases {
		got, found := sites[tc.fn]
		if !found {
			t.Errorf("%s: the census found no audit write", tc.fn)
			continue
		}
		if !slices.Equal(got.types, tc.want.types) || got.opaque != tc.want.opaque || got.asValue != tc.want.asValue {
			t.Errorf("%s: the census read %+v, want %+v", tc.fn, got, tc.want)
		}
	}
	// A writer's own forwarding call, a build-excluded file and a same-named
	// method on another type make no site of their own.
	for _, fn := range []string{"auditKind", "auditPair", "write", "hiddenOnWindows"} {
		if got, found := sites[fn]; found {
			t.Errorf("%s: the census read a site %+v where it should follow the writer on", fn, got)
		}
	}
}

func TestTheAuditCensusFollowsAnInsertToTheCallThatRunsIt(t *testing.T) {
	t.Parallel()
	direct := fixtureCensus(t, fixtureDirect).directInserts()
	if !direct.seedSeen {
		t.Error("the census did not see the seed's own INSERT")
	}
	count := func(positions []string, file string) int {
		return len(slices.DeleteFunc(slices.Clone(positions), func(at string) bool {
			return !strings.HasPrefix(at, "backend/internal/modules/direct/"+file+":")
		}))
	}
	cases := []struct {
		file           string
		others, opaque int
	}{
		{"runs.go", 5, 0},
		{"describes.go", 0, 0},
		{"bound.go", 0, 0},
		{"sink.go", 0, 2},
	}
	for _, tc := range cases {
		if got := count(direct.others, tc.file); got != tc.others {
			t.Errorf("%s: the census read %d INSERTs run outside the seed, want %d (%v)", tc.file, got, tc.others, direct.others)
		}
		if got := count(direct.opaque, tc.file); got != tc.opaque {
			t.Errorf("%s: the census lost %d INSERTs, want %d (%v)", tc.file, got, tc.opaque, direct.opaque)
		}
	}
}

func TestTheScrubCensusReadsVerbsThroughHelpersAndRefusesAStrayRowTable(t *testing.T) {
	t.Parallel()
	found, opaque, unreadVerbs := fixtureCensus(t, fixturePrivacy).scrubbedTypes(t)
	want := []string{"contact", extensionScrubbedTypes, "lead"}
	if got := sortedKeys(found); !slices.Equal(got, want) {
		t.Errorf("the scrub census found %v, want %v", got, want)
	}
	if len(opaque) != 1 || !strings.HasPrefix(opaque[0], "backend/internal/modules/privacy/scrubs.go:") {
		t.Errorf("the scrub census refused %v, want only clearCoreRows's row.Table in scrubs.go", opaque)
	}
	if len(unreadVerbs) != 0 {
		t.Errorf("the scrub census could not read the verbs at %v", unreadVerbs)
	}
}

func TestTheMigrationCensusReadsOnlyLiteralEntityTypes(t *testing.T) {
	t.Parallel()
	statements := map[string]string{
		"union": "INSERT INTO audit_log (action, entity_type) SELECT 'create', 'contact' FROM t " +
			"UNION DISTINCT SELECT 'create', 'lead' FROM u;",
		"unreadable_arm": "INSERT INTO audit_log (action, entity_type) SELECT 'create', 'deal' FROM t " +
			"UNION ALL (SELECT 'create', 'task' FROM u);",
		"case_column": "INSERT INTO audit_log (action, entity_type) " +
			"SELECT 'create', CASE WHEN x THEN 'deal' ELSE kind END FROM t;",
		"case_literals": "INSERT INTO audit_log (action, entity_type) " +
			"SELECT 'create', CASE WHEN x THEN 'offer' ELSE 'quote' END FROM t;",
		"no_columns":  "INSERT INTO audit_log VALUES ('create', 'note');",
		"quoted_from": "INSERT INTO audit_log (action, entity_type) SELECT 'from', 'company' FROM t;",
	}
	files := map[string]string{}
	for name, sql := range statements {
		files["migrations/core/"+name+".up.sql"] = sql + "\n"
	}
	root := writeFixture(t, files)
	types, opaque := auditMigrationTypes(t, filepath.Join(root, "migrations"))
	wantTypes := map[string]string{
		"contact": "union.up.sql", "lead": "union.up.sql", "offer": "case_literals.up.sql",
		"quote": "case_literals.up.sql", "company": "quoted_from.up.sql",
	}
	for value, file := range wantTypes {
		if types[value] != file {
			t.Errorf("record type %q: the census read it from %q, want %q", value, types[value], file)
		}
	}
	if len(types) != len(wantTypes) {
		t.Errorf("the census read %v, want only %v", types, wantTypes)
	}
	var refused []string
	for _, path := range opaque {
		refused = append(refused, strings.TrimSuffix(filepath.Base(path), ".up.sql"))
	}
	slices.Sort(refused)
	if want := []string{"case_column", "no_columns", "unreadable_arm"}; !slices.Equal(refused, want) {
		t.Errorf("the census refused %v, want %v", refused, want)
	}
}

func TestTheWordMapReaderReadsEachKeyShape(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "words.ts")
	source := "export const WORDS = {\n" +
		"  contact,\n" +
		"  \"a:b\": \"colon\",\n" +
		"  'quoted': 'single',\n" +
		"  link: \"https://example.test\", // a comment, not a key\n" +
		"  plain: 'x',\n" +
		"};\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := tsObjectEntries(t, path, "export const WORDS")
	keys, spreads := tsKeys(entries)
	if want := []string{"a:b", "contact", "link", "plain", "quoted"}; !slices.Equal(sortedKeys(keys), want) || len(spreads) != 0 {
		t.Errorf("the reader read keys %v and spreads %v, want %v and none", sortedKeys(keys), spreads, want)
	}
	if entries["link"] != `"https://example.test"` {
		t.Errorf("the reader read link as %q, so a // inside a string was taken for a comment", entries["link"])
	}
	_, spreads = tsKeys(tsLiteralEntries("...SHARED, own: 'x'"))
	if !slices.Equal(spreads, []string{"SHARED"}) {
		t.Errorf("the reader read spreads %v, want SHARED, which a compared map must fail on", spreads)
	}
}
