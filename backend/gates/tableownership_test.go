// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// Table ownership as a fitness function: the import DAG is enforced three
// ways, but nothing in the import graph stops a package from writing SQL
// against a table it does not own. This test closes that gap — it walks the
// hand-written Go under internal/modules and internal/compose, extracts every
// INSERT/UPDATE/DELETE target from SQL string literals (plus the storekit
// applier and row-lock table arguments), and asserts each module only writes its own
// tables. Cross-store writes exist by design (merge relinks, GDPR erasure,
// ingest materialization); each one is ratified in crossStoreWrites
// (tableownershipwaivers_test.go) with a self-contained
// rationale — an entry without a rationale is a finding, not a pass, and a
// waiver that matches no remaining write is stale and fails too. SELECTs are
// out of scope: reads are governed by each statement's own workspace predicate
// and the platform/auth row-scope clauses, not by ownership.
//
// That last sentence names two of the three halves of a read's admission and
// stops. The OBJECT half — may this caller read this KIND of record at all — is
// enforced at module store entry points by rbacgate_test.go and, in the compose
// tier, by no gate at all for any core object except `relationship`
// (backend/gates/edgereaders_test.go). Nine compose reads of one table drifted inside
// that gap while a correct implementation sat one directory away, so it is
// written here rather than left to be re-derived: a sentence that lists the
// guards a read has is read as the list of guards a read needs.

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// storekitOwned marks the tables written ONLY through
// platform/database/storekit (Audit/Emit) or the migration runner — no
// walked package owns them, so any direct module write needs a waiver.
const storekitOwned = "internal/platform/database/storekit"

// extSecretsStoreDir is the extension tier's secret namespace, which owns
// extension_secret. The constant exists so tableOwners can point at it in one
// spelling; WHICH platform packages this gate walks is answered by
// platformStoreDirs, not by a name here.
const extSecretsStoreDir = "internal/platform/extsecrets"

// keyVaultStoreDir is the local key-vault provider, which owns vault_secret.
// Until this gate walked it the table had no owner entry at all.
//
// What that left open, stated narrowly because the wide version is not true: a
// second writer in internal/modules or internal/compose was already caught, by
// the no-declared-owner arm below. The unguarded case was a second writer
// inside a platform package the gate did not walk — and the hand-kept list of
// those was short by two, which is what the derivation replaced.
const keyVaultStoreDir = "internal/platform/keyvault"

// jobsStoreDir runs the River fleet, and is where river_job is written from —
// discovered by platformStoreDirs rather than named here; the constant exists
// so tableOwners can point at it in one spelling.
const jobsStoreDir = "internal/platform/jobs"

// isIntegrationTagged reports whether the file builds only under the
// integration tag — the test lane's scaffolding (harnesses, fixtures).
// The ownership and write-shape obligations bind PRODUCTION writes; an
// integration-tagged file can never reach a shipped binary, and its
// seeding writes are the suites' own fixtures.
func isIntegrationTagged(path string) bool {
	f, err := os.Open(path) // #nosec G304 -- path is a *.go file from walking the trusted source tree
	if err != nil {
		return false
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			panic(cerr) // a leaked fd in a test helper is a bug, not a condition
		}
	}()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "//go:build integration" {
			return true
		}
		if line != "" && !strings.HasPrefix(line, "//") {
			return false // past the header — build constraints must precede it
		}
	}
	return false
}

// owningDir normalizes a package dir to its ownership unit: the module root
// under internal/modules (subpackages share their module's ownership), or
// internal/compose.
func owningDir(pkgDir string) string {
	if strings.HasPrefix(pkgDir, "internal/modules/") {
		parts := strings.SplitN(pkgDir, "/", 4)
		return strings.Join(parts[:3], "/")
	}
	return pkgDir
}

// storekitTableArg names the storekit calls that carry their table as the third
// argument. The four Patch appliers issue the UPDATE themselves; LockRow and
// LockPair do not write, but they are where ApplyLocked's table comes from —
// the lock carries it in an unexported field, so the lock site is the only
// place the table is legible, and without it every ApplyLocked write is
// invisible here.
var storekitTableArg = map[string]bool{
	"ApplyWithVersion": true,
	"ApplyGuarded":     true,
	"ApplyGuardedIn":   true,
	"LockRow":          true,
	"LockPair":         true,
}

// stringConstsByPackage maps each walked directory to the string constants its
// own files declare. A table name spelled as a package constant is the tree's
// normal style — `entityLead`, `projectObject` — and a walker that reads only
// literals attributes none of those writes to anybody.
func stringConstsByPackage(t *testing.T, fset *token.FileSet, roots []string) map[string]map[string]string {
	t.Helper()
	consts := map[string]map[string]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			file, err := parser.ParseFile(fset, filepath.ToSlash(path), nil, 0)
			if err != nil {
				return err
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.CONST {
					continue
				}
				for _, spec := range gen.Specs {
					value, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for i, name := range value.Names {
						if i >= len(value.Values) {
							continue
						}
						lit, ok := value.Values[i].(*ast.BasicLit)
						if !ok || lit.Kind != token.STRING {
							continue
						}
						text, err := strconv.Unquote(lit.Value)
						if err != nil {
							continue
						}
						if consts[dir] == nil {
							consts[dir] = map[string]string{}
						}
						consts[dir][name.Name] = text
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return consts
}

type tableWrite struct {
	pos   string // file:line for the finding
	table string
	// verb is which write this is — "insert", "delete" or "update".
	//
	// cols carries the columns the statement names, where they can be read.
	//
	// Ownership does not care: writing a table you do not own is the same
	// finding whichever verb does it. It is recorded because a SECOND gate
	// asks a question ownership cannot — whether a column classified as
	// having no writer yet has gained one — and "a row was created here" is
	// not the same claim as "privacy deletes from this table", which it
	// already does for both communication tables.
	verb string
	cols []string
	// site is the write's INSTANCE — "path/to/file.go:enclosingFunc" — and it
	// is what a waiver ratifies. The enclosing FUNCTION rather than the line:
	// a line moves whenever anything above it does, so a line-keyed waiver
	// goes stale on edits that never touched the write, and a map that is
	// re-typed on unrelated diffs stops being read.
	site string
}

// indirectTableArg ratifies the storekit call sites whose table arrives through
// a struct field rather than a name this walker can read, each with the tables
// the field can actually hold. Ratified, not discovered: the reason must name
// them, so the exception is re-checkable against the construction sites.
var indirectTableArg = gatekit.Waive(map[string]string{
	"internal/modules/people:w.table": "the evidence writer is one shape over two sidecars; the field is set at four struct literals in this package, to the organization_fact constant and to organization_profile_field, and people owns both",
})

// tableArgText reads a storekit table argument: a string literal, or an
// identifier declared as a string constant in the same package.
func tableArgText(arg ast.Expr, consts map[string]string) (string, bool) {
	switch v := arg.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(v.Value)
		return text, err == nil
	case *ast.Ident:
		text, ok := consts[v.Name]
		return text, ok
	default:
		return "", false
	}
}

// collectTableWrites walks every non-test module/compose source file and
// records each SQL write target (string literals plus the storekit applier and
// row-lock table arguments, see storekitTableArg) under its owning directory.
func collectTableWrites(t *testing.T) map[string][]tableWrite {
	t.Helper()
	writes := map[string][]tableWrite{} // owning dir → writes
	// storekitWrites counts what the CallExpr arm attributes. This gate lost
	// that whole arm once — it matched a method name no Patch has — and a
	// matcher that matches nothing is indistinguishable from a tree with no
	// versioned writes in it. The floor is what tells those two apart.
	storekitWrites := 0
	fset := token.NewFileSet()
	roots := append([]string{"internal/modules", "internal/compose"}, platformStoreDirs(t)...)
	consts := stringConstsByPackage(t, fset, roots)
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") ||
				isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			dir := filepath.ToSlash(filepath.Dir(path))
			owner := owningDir(dir)
			// enclosing is the declaration the walk is currently inside. Set
			// per top-level declaration below rather than tracked during the
			// walk: ast.Inspect is flat, and a stack maintained by hand here
			// would be a second traversal to keep correct.
			enclosing := unnamedDeclSite
			record := func(pos token.Pos, tables []sqlTarget) {
				for _, target := range tables {
					writes[owner] = append(writes[owner], tableWrite{
						pos:   fset.Position(pos).String(),
						table: target.table,
						verb:  target.verb,
						cols:  target.cols,
						// Relative to the owner, which the key already names:
						// the absolute path would repeat that prefix in every
						// entry and push the part a reader is actually
						// comparing off the end of the line.
						site: strings.TrimPrefix(path, owner+"/") + ":" + enclosing,
					})
				}
			}
			visit := func(n ast.Node) {
				switch node := n.(type) {
				case *ast.BasicLit:
					if node.Kind != token.STRING {
						return
					}
					text, err := strconv.Unquote(node.Value)
					if err != nil {
						return
					}
					record(node.Pos(), sqlWrites(text))
				case *ast.CallExpr:
					sel, ok := node.Fun.(*ast.SelectorExpr)
					if !ok || !storekitTableArg[sel.Sel.Name] || len(node.Args) < 4 {
						return
					}
					table, ok := tableArgText(node.Args[2], consts[dir])
					if !ok {
						if indirectTableArg.Waived(t, owner+":"+exprText(fset, node.Args[2])) {
							return
						}
						// A table this walker cannot read is a table it cannot
						// attribute, and a skip here reads exactly like a module
						// that writes nothing. Reported, so the write names its
						// table where a reader — and this gate — can see it.
						t.Errorf("%s: %s.%s takes its table from an expression this gate cannot read — "+
							"name the table in a string literal or a package-level string constant, "+
							"or the write is attributed to no owner at all",
							fset.Position(node.Pos()), exprText(fset, sel.X), sel.Sel.Name)
						return
					}
					// Not "insert", and that is checkable rather than assumed:
					// storekitTableArg holds Apply* and Lock* only, and none
					// of them creates a row. TestNoPendingWriterHasAWriter
					// leans on that — an INSERT cannot arrive through this arm
					// and be read as an update.
					record(node.Pos(), []sqlTarget{{table: strings.ToLower(table), verb: "storekit"}})
					storekitWrites++
				}
			}
			walkDeclSites(fset, file, func(site string, n ast.Node) {
				enclosing = site
				visit(n)
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if storekitWrites < storekitWriteFloor {
		t.Fatalf("attributed only %d storekit table arguments, expected at least %d — the %v matcher has stopped matching, and every versioned write in the tree is now invisible to this gate",
			storekitWrites, storekitWriteFloor, slices.Sorted(maps.Keys(storekitTableArg)))
	}
	return writes
}

// storekitWriteFloor is set below the live count so ordinary refactoring does
// not trip it; it catches the arm going to zero, not a write being deleted.
const storekitWriteFloor = 25

// waiverKey is the subject crossStoreWrites ratifies: the writing package, the
// table it reaches into, and the INSTANCE that reaches it.
//
// The declaration is the point. Keyed on owner and table alone, one entry
// ratifies the CATEGORY — "people may write activity_link" — and a second,
// differently written copy of that write inside the same package is admitted by
// the entry the first one earned, with no finding to notice. A cross-store
// write is ratified on its own evidence or it is not ratified.
func waiverKey(owner string, w tableWrite) string {
	return owner + ":" + w.table + ":" + w.site
}

// unratifiedCrossStoreWrites returns one finding per write that reaches a table
// its package does not own and that no waiver ratifies.
//
// It RETURNS the findings rather than reporting them, so the gate's decision can
// be exercised against synthetic writes without failing the test that exercises
// it, and it takes the waiver set rather than reaching for the package-level one
// so such a test can pass a throwaway copy. Querying the real set MARKS entries
// matched, and gatekit accumulates that across every test in the package — a
// probe that consulted it would silently satisfy AssertAllMatched for the entry
// it named, which is the one staleness a stale-waiver gate exists to report. A gate whose only input is the tree it happens to be checked out over can
// be tested for what it accepts today and never for what it would refuse — and
// refusing is the half that has to keep working.
func unratifiedCrossStoreWrites(t testing.TB, waivers *gatekit.Waivers[string], writes map[string][]tableWrite) []string {
	t.Helper()
	owners := slices.Sorted(maps.Keys(writes))
	var findings []string
	for _, owner := range owners {
		for _, w := range writes[owner] {
			declared, known := tableOwners[w.table]
			if !known {
				findings = append(findings, fmt.Sprintf(
					"%s: %s writes table %q which has no declared owner — add it to tableOwners in backend/gates/tableownership_test.go",
					w.pos, owner, w.table,
				))
				continue
			}
			if declared == owner {
				continue
			}
			key := waiverKey(owner, w)
			if waivers.Waived(t, key) {
				continue
			}
			findings = append(findings, fmt.Sprintf(
				"%s: %s writes table %q owned by %s — move the write into the owning module, or ratify THIS write in crossStoreWrites[%q] with a self-contained rationale. "+
					"A waiver a sibling write in the same package already holds does not cover this one: the key names the function, so every copy is ratified on its own evidence",
				w.pos, owner, w.table, declared, key,
			))
		}
	}
	return findings
}

func TestEveryPackageOnlyWritesTablesItOwns(t *testing.T) {
	t.Parallel()
	defer crossStoreWrites.AssertAllMatched(t)
	defer indirectTableArg.AssertAllMatched(t)

	for _, finding := range unratifiedCrossStoreWrites(t, crossStoreWrites, collectTableWrites(t)) {
		t.Error(finding)
	}
}

// TestASecondCopyOfARatifiedWriteIsNotCoveredByTheFirst is this gate's own
// defect case, and it is why the key names a declaration rather than a package
// and a table.
//
// Keyed on package and table alone, a waiver ratifies the CATEGORY, and a
// second, differently written copy of the same cross-store write is admitted by
// the entry the first one earned — silently, because a key that already exists
// produces no finding to notice.
//
// The waiver set is a throwaway rather than crossStoreWrites: querying the real
// one marks its entries matched for the whole package, which would quietly
// satisfy the staleness sweep for whichever entry this case names.
func TestASecondCopyOfARatifiedWriteIsNotCoveredByTheFirst(t *testing.T) {
	t.Parallel()
	const (
		owner = "internal/modules/people"
		table = "activity_link"
		first = "ensure.go:Store.linkActivityToPerson"
	)
	ratified := tableWrite{pos: "internal/modules/people/ensure.go:1:1", table: table, site: first}
	// Same package, same table, a different declaration.
	planted := tableWrite{
		pos:   "internal/modules/people/planted.go:1:1",
		table: table,
		site:  "planted.go:aSecondWriterOfARatifiedTable",
	}

	// The plant only tests coverage if it is a write the tree really ratifies
	// and really does not own; both halves can drift out from under it.
	declared, known := tableOwners[table]
	if !known || declared == owner {
		t.Fatalf("%s is no longer a table %s writes without owning, so this case plants nothing", table, owner)
	}
	if !slices.Contains(crossStoreWrites.Subjects(), waiverKey(owner, ratified)) {
		t.Fatalf("crossStoreWrites no longer ratifies %s — repoint this case at a live waiver",
			waiverKey(owner, ratified))
	}
	// And the two must differ ONLY in the part the key added, or the case would
	// pass on a distinction the superseded key already drew.
	if owner+":"+ratified.table != owner+":"+planted.table {
		t.Fatalf("the plant differs from the ratified write in package or table, so a key naming " +
			"neither would already separate them and this case proves nothing about the declaration")
	}

	waivers := gatekit.Waive(map[string]string{
		waiverKey(owner, ratified): "the write this case treats as already ratified",
	})

	t.Run("the second copy is refused", func(t *testing.T) {
		findings := unratifiedCrossStoreWrites(t, waivers, map[string][]tableWrite{owner: {planted}})
		if len(findings) != 1 {
			t.Fatalf("planted one unratified write, got %d findings: %v", len(findings), findings)
		}
		// Named, not merely counted: a finding about some other write would
		// satisfy a bare count while the planted copy went through.
		if !strings.Contains(findings[0], planted.site) {
			t.Errorf("the finding does not name the planted write %q, so this case cannot tell that the "+
				"planted copy was the one refused:\n%s", planted.site, findings[0])
		}
	})

	t.Run("the ratified copy still passes", func(t *testing.T) {
		// The other direction. A gate that refused the planted write by
		// refusing every write would pass the subtest above and fail every
		// ratified write in the tree on the next push.
		if findings := unratifiedCrossStoreWrites(t, waivers, map[string][]tableWrite{owner: {ratified}}); len(findings) != 0 {
			t.Errorf("the ratified write is no longer covered by its own waiver: %v", findings)
		}
	})

	t.Run("two same-named methods on different receivers are two sites", func(t *testing.T) {
		// This tree writes two workers into one file as a matter of course, and
		// a bare method name would collapse both onto one site — the category
		// ratification above, one level down.
		const src = `package p

type aWorker struct{}
type aWorkspaceWorker struct{}

func (w *aWorker) Work() {}
func (w *aWorkspaceWorker) Work() {}
`
		// The type declarations are in the fixture only so the methods have
		// receivers; the sites under test are the two Work methods.
		sites := methodSitesOf(t, "jobs.go", src)
		if len(sites) != 2 {
			t.Fatalf("the fixture no longer declares exactly two methods: %v", sites)
		}
		if sites[0] == sites[1] {
			t.Errorf("both methods name the site %q, so one waiver would ratify both writes — "+
				"the receiver has stopped reaching the key", sites[0])
		}
	})

	t.Run("two package-level statements in one file are two sites", func(t *testing.T) {
		// GROUPED, which is the shape a per-declaration answer gets wrong:
		// `const (...)` is ONE declaration holding two statements. Separate
		// `const` statements are two declarations and pass either way, so a
		// fixture written that way exercises the walk only where it was
		// already right.
		const src = `package p

const (
	blankOne  = ` + "`UPDATE t SET a = NULL`" + `
	deleteOne = ` + "`DELETE FROM t`" + `
)
`
		sites := literalSitesOf(t, "statements.go", src)
		if len(sites) != 2 {
			t.Fatalf("the fixture no longer declares exactly two statements: %v", sites)
		}
		if sites[0] == sites[1] {
			t.Errorf("both statements name the site %q, so one waiver would ratify both — the walk "+
				"is answering a grouped block per declaration rather than per statement", sites[0])
		}
	})

	t.Run("two statements bound by one spec are two sites", func(t *testing.T) {
		// ONE ValueSpec binding two names. The grouped-block arm above answers
		// per spec, which is still one answer for both statements here — the
		// same collapse a third level in, and the reason the walk pairs values
		// with names rather than stopping at the spec.
		const src = `package p

const blankOne, deleteOne = ` + "`UPDATE t SET a = NULL`" + `, ` + "`DELETE FROM t`" + `
`
		sites := literalSitesOf(t, "onespec.go", src)
		if len(sites) != 2 {
			t.Fatalf("the fixture no longer binds exactly two statements: %v", sites)
		}
		if sites[0] == sites[1] {
			t.Errorf("both statements name the site %q, so one waiver would ratify both — the walk "+
				"is answering one spec per spec rather than per value bound in it", sites[0])
		}
	})
}
