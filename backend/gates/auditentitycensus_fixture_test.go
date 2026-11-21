// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The audit census reads, refuses or reports each Go shape it has met, on
// planted packages type-checked the way the module is.

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/types"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	fixtureSeedPkg = modulePath + "/fixture/sk"
	fixtureSeed    = fixtureSeedPkg + ".insertRows"
	fixtureAppPkg  = modulePath + "/fixture/app"
)

// fixtureSeedSource is a storekit in miniature: one INSERT, a door that takes
// no image and one that takes only an after-image.
const fixtureSeedSource = `package sk

import "context"

type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) error
}

const Table = "audit_log"

func insertRows(ctx context.Context, tx Tx, action, entityType string) error {
	return tx.Exec(ctx, "INSERT INTO "+Table+" (action, entity_type) VALUES ($1, $2)", action, entityType)
}

func Audit(ctx context.Context, tx Tx, action, entityType string) error {
	return insertRows(ctx, tx, action, entityType)
}

func AuditEvent(ctx context.Context, tx Tx, action, entityType string, id int, after any) error {
	return insertRows(ctx, tx, action, entityType)
}
`

// fixtureCensus type-checks the seed package and app, a package of the given
// files, and reads them from seed.
func fixtureCensus(t *testing.T, seed string, app map[string]string) *auditEntityCensus {
	t.Helper()
	planted := []struct {
		path  string
		files map[string]string
	}{
		{fixtureSeedPkg, map[string]string{"sk.go": fixtureSeedSource}},
		{modulePath + "/fixture/other", map[string]string{"other.go": "package other\n\nconst Kind = \"beta\"\n"}},
		{fixtureAppPkg, app},
	}
	checked := map[string]*types.Package{}
	std := importer.Default()
	imports := importerFunc(func(path string) (*types.Package, error) {
		if pkg, ok := checked[path]; ok {
			return pkg, nil
		}
		return std.Import(path)
	})
	var pkgs []typedPackage
	for _, pkg := range planted {
		var files []*ast.File
		for _, name := range slices.Sorted(mapKeysOf(pkg.files)) {
			dir := strings.TrimPrefix(pkg.path, modulePath+"/")
			file, err := parser.ParseFile(gatekit.SourceFileSet(), dir+"/"+name, pkg.files[name], parser.ParseComments)
			if err != nil {
				t.Fatalf("parsing the planted %s: %v", name, err)
			}
			files = append(files, file)
		}
		info := newTypedInfo()
		checkedPkg, err := (&types.Config{Importer: imports}).Check(pkg.path, gatekit.SourceFileSet(), files, info)
		if err != nil {
			t.Fatalf("type-checking the planted %s: %v", pkg.path, err)
		}
		checked[pkg.path] = checkedPkg
		pkgs = append(pkgs, typedPackage{path: pkg.path, files: files, info: info, types: checkedPkg})
	}
	return readAuditEntityCensus(pkgs, seed)
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

func mapKeysOf(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for key := range m {
			if !yield(key) {
				return
			}
		}
	}
}

// fixtureRead is what a census read from the planted app, in a form a case
// can compare.
type fixtureRead struct {
	values, opaque                 []string
	asValue, directInserts, unread int
}

func readFixture(census *auditEntityCensus) fixtureRead {
	var read fixtureRead
	for _, site := range census.sites {
		if !strings.Contains(site.pos, "fixture/app/") {
			continue
		}
		if site.opaque != "" {
			read.opaque = append(read.opaque, site.opaque)
		} else if !slices.Contains(read.values, site.value) {
			read.values = append(read.values, site.value)
		}
	}
	sort.Strings(read.values)
	sort.Strings(read.opaque)
	read.asValue, read.directInserts = len(census.asValue), len(census.directInserts)
	read.unread = len(census.unreadInterfaces)
	return read
}

const fixtureHeader = "package app\n\nimport (\n\t\"context\"\n\t\"fmt\"\n\t\"strings\"\n\n\tk \"" + fixtureSeedPkg + "\"\n\t\"" + modulePath + "/fixture/other\"\n)\n\nvar ctx = context.Background()\nvar tx k.Tx\nvar _, _, _ = fmt.Sprint, strings.ToLower, other.Kind\n\n"

func TestTheAuditCensusReadsEachShape(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		want fixtureRead
	}{
		{
			name: "a constant from another file, an aliased import and an imported constant are read",
			body: "func a() { _ = k.Audit(ctx, tx, \"update\", kind); _ = k.Audit(ctx, tx, \"update\", other.Kind); _ = k.Audit(ctx, tx, \"update\", kind+\"_x\") }\n",
			want: fixtureRead{values: []string{"alpha", "alpha_x", "beta"}},
		},
		{
			name: "a wrapper's callers are read, through a closure, and a shadowing local is its own value",
			body: "func wrap(t string) error { f := func() error { return k.Audit(ctx, tx, \"u\", t) }; return f() }\n" +
				"func shadow(t string) error { { t := \"zeta\"; return k.Audit(ctx, tx, \"u\", t) } }\n" +
				"func b() { _ = wrap(\"gamma\"); _ = shadow(\"never\") }\n",
			want: fixtureRead{values: []string{"gamma", "zeta"}},
		},
		{
			name: "a parameter the wrapper changes is not read",
			body: "func w(t string) error { t = strings.ToLower(t); return k.Audit(ctx, tx, \"u\", t) }\nfunc c() { _ = w(\"x\") }\n",
			want: fixtureRead{opaque: []string{"t"}},
		},
		{
			name: "a local set on each branch and a function's results are read",
			body: "func kindOf(b bool) (string, int) { if b { return \"eta\", 1 }; return \"theta\", 2 }\n" +
				"func d(b bool) { var e string; switch { case b: e = \"c\"; default: e = \"d\" }; _ = k.Audit(ctx, tx, \"u\", e); f, _ := kindOf(b); _ = k.Audit(ctx, tx, \"u\", f) }\n",
			want: fixtureRead{values: []string{"c", "d", "eta", "theta"}},
		},
		{
			name: "a writer handed on as a value is reported",
			body: "func e() { audit := k.Audit; _ = audit(ctx, tx, \"u\", \"never\") }\n",
			want: fixtureRead{asValue: 1},
		},
		{
			name: "a call through an interface reaches its implementation",
			body: "type auditor interface{ Write(t string) error }\ntype store struct{}\n" +
				"func (store) Write(t string) error { return k.Audit(ctx, tx, \"u\", t) }\n" +
				"func f(a auditor) { _ = a.Write(\"iota\") }\n",
			want: fixtureRead{values: []string{"iota"}},
		},
		{
			name: "a result read through an interface is read from each implementation",
			body: "type namer interface{ Name() string }\ntype fixed struct{}\n" +
				"func (fixed) Name() string { return \"omicron\" }\n" +
				"func m(x namer) { _ = k.Audit(ctx, tx, \"u\", x.Name()) }\n",
			want: fixtureRead{values: []string{"omicron"}},
		},
		{
			name: "a type switch variable is not read",
			body: "func o(v any) { switch t := v.(type) { case string: _ = k.Audit(ctx, tx, \"u\", t) } }\n",
			want: fixtureRead{opaque: []string{"t"}},
		},
		{
			name: "a field is read by its writes across the module, unless a decoder or a scanner can fill it",
			body: "type sub struct{ kind string }\ntype Pub struct{ Kind string }\ntype Decoded struct{ Kind string }\ntype Scanned struct{ Kind string }\ntype Inst struct{ Kind string }\n" +
				"func decode[T any]() T { var zero T; return zero }\n" +
				"func fill(d *Decoded) { _, _ = fmt.Sscan(\"x\", d) }\nfunc scan(s *Scanned) { _, _ = fmt.Sscan(\"x\", &s.Kind) }\n" +
				"func g(p Pub, d Decoded, sc Scanned) { s := sub{kind: \"kappa\"}; _, _, _, _ = Pub{Kind: \"rho\"}, Decoded{Kind: \"no\"}, Scanned{Kind: \"no\"}, Inst{Kind: \"no\"}\n" +
				"in := decode[Inst]()\n" +
				"_ = k.Audit(ctx, tx, \"u\", s.kind); _ = k.Audit(ctx, tx, \"u\", p.Kind); _ = k.Audit(ctx, tx, \"u\", d.Kind); _ = k.Audit(ctx, tx, \"u\", sc.Kind); _ = k.Audit(ctx, tx, \"u\", in.Kind) }\n",
			want: fixtureRead{values: []string{"kappa", "rho"}, opaque: []string{"d.Kind", "in.Kind", "sc.Kind"}},
		},
		{
			name: "a writer reached through a promoted method of an interface is read",
			body: "type base struct{}\nfunc (base) Write(t string) error { return k.Audit(ctx, tx, \"u\", t) }\n" +
				"type store struct{ base }\nfunc (store) Other() {}\ntype w interface{ Write(t string) error; Other() }\nfunc f(a w) { _ = a.Write(\"pi\") }\n",
			want: fixtureRead{values: []string{"pi"}},
		},
		{
			name: "a result read through a promoted method of an interface is read from each implementation",
			body: "type nb struct{}\nfunc (nb) Name() string { return \"sigma\" }\ntype outer struct{ nb }\nfunc (outer) Other() {}\n" +
				"type full struct{}\nfunc (full) Name() string { return \"tau\" }\nfunc (full) Other() {}\n" +
				"type namer interface{ Name() string; Other() }\nfunc m(x namer) { _ = k.Audit(ctx, tx, \"u\", x.Name()) }\n",
			want: fixtureRead{values: []string{"sigma", "tau"}},
		},
		{
			name: "an interface whose signature spells an alias reaches its implementation",
			body: "type Str = string\ntype w interface{ Write(t Str) error }\ntype st struct{}\n" +
				"func (st) Write(t string) error { return k.Audit(ctx, tx, \"u\", t) }\nfunc f(a w) { _ = a.Write(\"upsilon\") }\n",
			want: fixtureRead{values: []string{"upsilon"}},
		},
		{
			name: "an interface sharing a writer's name and signature that nothing implements is reported",
			body: "type st struct{}\nfunc (st) Write(t string) error { return k.Audit(ctx, tx, \"u\", t) }\nfunc (st) Extra() {}\n" +
				"type lone interface{ Write(t string) error; Missing() }\nfunc f(a lone) { _ = a.Write(\"never\") }\nfunc g() { _ = st{}.Write(\"phi\") }\n",
			want: fixtureRead{values: []string{"phi"}, unread: 1},
		},
		{
			name: "a method expression's receiver is not read as its first argument",
			body: "type s struct{}\nfunc (s) Write(a, t string) error { return k.Audit(ctx, tx, a, t) }\nfunc f() { _ = s.Write(s{}, \"action\", \"chi\") }\n",
			want: fixtureRead{values: []string{"chi"}},
		},
		{
			name: "a closure's own parameter is not read",
			body: "var h = func(t string) error { return k.Audit(ctx, tx, \"u\", t) }\n",
			want: fixtureRead{opaque: []string{"t"}},
		},
		{
			name: "a range over a package list is read, and one that is exported or used any other way is not",
			body: "var fixed = []string{\"lambda\", \"mu\"}\nvar grown = []string{\"nu\"}\nvar copied = []string{\"psi\"}\nvar Shared = []string{\"omega\"}\n" +
				"func init() { grown = append(grown, strings.ToLower(\"XI\")); copy(copied, []string{\"y\"}) }\n" +
				"func i() { _ = fixed[0]; for _, t := range fixed { _ = k.Audit(ctx, tx, \"u\", t) }; for _, t := range grown { _ = k.Audit(ctx, tx, \"u\", t) }\n" +
				"for _, t := range copied { _ = k.Audit(ctx, tx, \"u\", t) }; for _, t := range Shared { _ = k.Audit(ctx, tx, \"u\", t) } }\n",
			want: fixtureRead{values: []string{"lambda", "mu"}, opaque: []string{"t", "t", "t"}},
		},
		{
			name: "an element of a package map is read by its literal, and one the package changes is not",
			body: "var verbs = map[string]string{\"a\": \"delta\", \"b\": \"epsilon\"}\nvar moved = map[string]string{\"a\": \"no\"}\n" +
				"func init() { moved[\"b\"] = \"no\" }\nfunc q(key string) { _ = k.Audit(ctx, tx, \"u\", verbs[key]); _ = k.Audit(ctx, tx, \"u\", moved[key]) }\n",
			want: fixtureRead{values: []string{"delta", "epsilon"}, opaque: []string{"moved[key]"}},
		},
		{
			name: "a write to audit_log built by fmt.Sprintf or strings.Join from constants is reported",
			body: "func j() { _ = tx.Exec(ctx, fmt.Sprintf(\"INSERT INTO %s_log (a) VALUES (1)\", \"audit\")); _ = tx.Exec(ctx, strings.Join([]string{\"COPY\", \"audit_log\", \"FROM STDIN\"}, \" \")) }\n",
			want: fixtureRead{directInserts: 2},
		},
		{
			name: "an UPDATE of audit_log is reported when it sets entity_type, and a MERGE always",
			body: "const fix = \"UPDATE audit_log SET entity_type = 'x' WHERE id = $1\"\nconst mask = \"UPDATE audit_log SET before = $2 WHERE entity_type = 'x'\"\n" +
				"const merge = \"MERGE INTO audit_log USING src ON false WHEN NOT MATCHED THEN INSERT VALUES (1)\"\n",
			want: fixtureRead{directInserts: 2},
		},
		{
			name: "an INSERT into audit_log outside the seed is reported once each",
			body: "const q = \"INSERT INTO audit_log (action) VALUES ('x')\"\n" +
				"func j(table string) { _ = tx.Exec(ctx, q); _ = tx.Exec(ctx, fmt.Sprintf(\"INSERT INTO %s (action) VALUES ('x')\", \"audit_log\")); name := \"audit_log\"; _ = tx.Exec(ctx, \"INSERT INTO \"+name+\" (action) VALUES ('x')\"); _ = tx.Exec(ctx, \"INSERT INTO \"+table) }\n",
			want: fixtureRead{directInserts: 3},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			census := fixtureCensus(t, fixtureSeed, map[string]string{
				"a.go": fixtureHeader + tc.body,
				"b.go": "package app\n\nconst kind = \"alpha\"\n",
			})
			if !census.seedInserts {
				t.Fatal("the planted seed was not read as the INSERT")
			}
			got := readFixture(census)
			if !slices.Equal(got.values, tc.want.values) || !slices.Equal(got.opaque, tc.want.opaque) ||
				got.asValue != tc.want.asValue || got.directInserts != tc.want.directInserts || got.unread != tc.want.unread {
				t.Errorf("read %+v, want %+v (direct inserts at %v)", got, tc.want, census.directInserts)
			}
		})
	}
}

func TestTheAuditCensusRefusesAMissingSeed(t *testing.T) {
	t.Parallel()
	census := fixtureCensus(t, fixtureSeedPkg+".renamedRows", map[string]string{"a.go": fixtureHeader})
	if census.seedInserts || len(census.writers) != 0 {
		t.Errorf("a seed that is not declared still read %d writers", len(census.writers))
	}
}
