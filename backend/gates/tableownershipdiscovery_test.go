// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// WHICH packages the ownership gate walks, derived rather than remembered.
//
// Separate from the judgement next door because it answers a different
// question. That file asks whether a package writes only what it owns; this one
// asks who the writers ARE — and the answer used to be a list somebody had to
// keep, which is the shape a derivation replaces.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// platformRoot is the tree the platform writers are DISCOVERED in. Everything
// under it that writes a row is walked; everything else is not swept in.
const platformRoot = "internal/platform"

// platformStoreDirs answers every package under internal/platform that writes a
// row.
//
// Derived, not named. Three were named here — settings, extsecrets, keyvault —
// and the list was demonstrably short: platform/events writes event_outbox and
// platform/jobs deletes river_job, so neither write was ever compared against
// the ownership map it is declared in. A hand-kept list of writers is one
// somebody has to remember to extend, and nothing fails when they do not; that
// is exactly the hole `vault_secret` sat in until somebody noticed.
//
// Widening to the whole of internal/platform would be the other mistake: it
// would sweep in packages that write no rows and make the gate judge files it
// has no declaration for. Discovery keeps the set exactly as wide as the
// writing.
func platformStoreDirs(t *testing.T) []string {
	t.Helper()
	found := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(platformRoot, func(path string, d fs.DirEntry, err error) error {
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
		if dir == storekitOwned {
			// The applier itself, not an owner. Its writes name whatever table
			// the CALLER passes, so judging it as a writer would attribute
			// every module's versioned write to this one package — and every
			// one of those is already judged where the table is named.
			return nil
		}
		if writesARow(file) {
			found[dir] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s for row writers: %v", platformRoot, err)
	}
	dirs := make([]string, 0, len(found))
	for dir := range found {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	if len(dirs) < platformWriterFloor {
		t.Fatalf("only %d package(s) under %s were found to write rows, and this gate is pinned "+
			"at %d: %v\n\nA discovery that stopped finding writers reports a clean tree in the "+
			"same words as a tree whose writers are all declared.",
			len(dirs), platformRoot, platformWriterFloor, dirs)
	}
	return dirs
}

// platformWriterFloor is how many row-writing platform packages the discovery
// found when it replaced the hand-kept list.
const platformWriterFloor = 5

// writesARow reports whether the file writes one, in either shape the walker
// judges.
//
// Both shapes, because discovery narrower than the walk is discovery that
// hides a write: a package whose only row write goes through a storekit
// applier — no inline SQL anywhere in it — would never be walked, and its
// write never compared against tableOwners. That is the same hole the
// hand-kept list had, one level in.
func writesARow(file *ast.File) bool {
	for _, statement := range gatekit.SQLStatementsOf(file) {
		if len(sqlWriteTargets(statement)) > 0 {
			return true
		}
	}
	return callsAStorekitApplier(file)
}

// callsAStorekitApplier reports whether the file reaches a storekit call that
// carries its table — the second way this gate learns a package writes rows.
//
// The SAME shape the walker attributes from, argument count included. Matching
// a bare method name would declare a writer out of any package holding a method
// that happens to share one, and the walker would then reach a package with no
// table to read and fail on its own unreadable-argument arm — a gate failing
// over a file that writes nothing. And a package that does not import storekit
// cannot be calling it at all.
//
// The import is as far as this can be narrowed, and the residual is stated
// rather than left to be discovered: a file that imports storekit AND declares
// its own four-argument method under one of these names is counted here on the
// name alone. Narrowing further by QUALIFIER — checking the selector reaches
// the storekit identifier — is what the shapes rule out: a third of the tree's
// applier calls are methods on a `*storekit.Patch` value whose receiver is
// named by its holder (`p.ApplyGuarded`), so that check would stop recognising
// them. Between the two, this gate over-recognises: an extra package joins the
// walk and either attributes nothing or fails LOUDLY on the table it cannot
// read, where the qualifier check drops real writers SILENTLY and every table
// they own goes back to being unowned — the direction a census is not allowed
// to fail in.
func callsAStorekitApplier(file *ast.File) bool {
	if imported, _ := gatekit.ImportedAs(file, storekitImportPath); imported == "" {
		return false
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || len(call.Args) < storekitTableArgArity {
			return true
		}
		if method, isMethod := call.Fun.(*ast.SelectorExpr); isMethod && storekitTableArg[method.Sel.Name] {
			found = true
		}
		return !found
	})
	return found
}

// storekitImportPath is the package a storekit-shaped call must come from.
const storekitImportPath = "github.com/margince/margince/backend/internal/platform/database/storekit"

// storekitTableArgArity is how many arguments such a call carries before its
// table is at Args[2] — the walker's own guard, so discovery cannot count a
// call the walk would decline to read.
const storekitTableArgArity = 4

// TestThePlatformWritersAreDiscoveredRatherThanRemembered is the derivation,
// from the other end.
//
// Three platform packages were named here by hand, and the list was short by
// two: platform/events writes event_outbox and platform/jobs deletes river_job,
// so neither write was ever compared against the ownership map it is declared
// in. A hand-kept list of writers is one somebody has to remember to extend,
// and nothing failed when they did not.
func TestThePlatformWritersAreDiscoveredRatherThanRemembered(t *testing.T) {
	t.Parallel()
	found := map[string]bool{}
	for _, dir := range platformStoreDirs(t) {
		found[dir] = true
	}
	// The three that were named, and the two that were not.
	for _, dir := range []string{
		settingsStoreDir, extSecretsStoreDir, keyVaultStoreDir,
		"internal/platform/events", jobsStoreDir,
	} {
		if !found[dir] {
			t.Errorf("%s writes rows and the discovery did not find it", dir)
		}
	}
	// And the applier is NOT one: its writes name whatever table the caller
	// passes, so counting it would attribute every module's versioned write to
	// one package.
	if found[storekitOwned] {
		t.Errorf("%s was discovered as a writer; it is the mechanism every writer goes through",
			storekitOwned)
	}
	// A package that writes no rows stays outside. Widening to the whole of
	// internal/platform would make the gate judge files it has no declaration
	// for, which is the other way to get this wrong.
	if found["internal/platform/blobstore"] {
		t.Error("internal/platform/blobstore writes no rows and was swept in anyway")
	}
}

// withStorekit puts a probe in a package that imports storekit, which is what
// makes a storekit-shaped call a storekit call.
func withStorekit(body string) string {
	return "package p\n\nimport \"" + storekitImportPath + "\"\n\n" + body
}

// The second write shape, from the other end.
//
// A package whose only row write goes through a storekit applier holds no
// inline SQL at all, so a discovery reading literals alone would never walk it
// — and its write would never be compared against tableOwners. That is the hand
// -kept list's hole one level in, so the shape is planted rather than assumed.
func TestAStorekitOnlyWriterIsDiscovered(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		source string
		want   bool
	}{
		{
			// The applier as a METHOD on a storekit value, which is a third of
			// the tree's call sites and the reason `callsAStorekitApplier`
			// cannot narrow to calls that reach the storekit identifier: the
			// receiver here is named by whoever holds the patch.
			name: "an applier call with no SQL in the file",
			source: withStorekit("func write(p *storekit.Patch, tx pgx.Tx) error {\n" +
				"\treturn p.ApplyWithVersion(ctx, tx, \"vault_secret\", id, version)\n}\n"),
			want: true,
		},
		{
			name: "a row lock, which is where ApplyLocked's table is legible",
			source: withStorekit("func lock(tx pgx.Tx) error {\n" +
				"\t_, err := storekit.LockRow(ctx, tx, \"vault_secret\", id)\n\treturn err\n}\n"),
			want: true,
		},
		{
			name:   "a package that reads and writes nothing",
			source: "package p\nfunc name() string { return \"vault_secret\" }\n",
			want:   false,
		},
		{
			// A method that merely shares a name. Counted, this package would
			// join the walk with no table to read, and the walker would fail on
			// its own unreadable-argument arm over a file that writes nothing.
			name: "a same-named method on somebody else's type",
			source: "package p\ntype cache struct{}\nfunc (c cache) LockRow(a, b, d, e int) {}\n" +
				"func use(c cache) { c.LockRow(1, 2, 3, 4) }\n",
			want: false,
		},
		{
			// The walker declines to read a call this short, so discovery must
			// not count one either — the two have to agree on what a write is.
			name:   "a storekit-shaped call too short to carry a table",
			source: withStorekit("func short(p *storekit.Patch) { p.LockRow(ctx, tx) }\n"),
			want:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := writesARow(parsed); got != tc.want {
				t.Errorf("writesARow = %t, want %t", got, tc.want)
			}
		})
	}
}
