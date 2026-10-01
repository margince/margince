// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every put into the object store declares its key provisional first, or says why it need not.
//
// A put and the row that names its bytes are two writes to two systems, and the
// object left behind when the row's transaction fails is one nothing can find
// again: an erasure reads a key off a row. platform/storedobject closes that by
// recording the key BEFORE the put, so the reap can find it. Each put either
// records its key first, which is read off the code rather than listed, or
// carries a stated exemption.
//
// A put is found by TYPE, not by spelling: the receiver is blob, blobs, h.blob
// or a parameter, and what they share is that the method is blobstore's Put.
// Each package is type-checked loosely (typeCheckLoosely), except that the
// blobstore import is checked from source, so a value declared as
// blobstore.Store resolves. What the loose check cannot type — a store reached
// through another package's field or return — is kept rather than dropped: any
// five-argument Put on an untyped or interface receiver is judged too, so the
// census errs towards naming too much.
//
// What it cannot see: a Record on a branch that does not run before the put (it
// reads source order, not paths of control), and a key reassigned between the
// record and the put under the same spelling.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	blobstorePath   = modulePath + "/internal/platform/blobstore"
	storedObjectPkg = modulePath + "/internal/platform/storedobject"
)

// blobPutRoots are the trees production Go lives in, relative to the module
// root. extensions/ holds its own modules, which are swept file by file all the
// same.
var blobPutRoots = []string{"internal", "pkg", "cmd", "../extensions"}

// blobPutExemptions holds, per function whose put records nothing, why its key
// is not provisional. Keyed by file and function, because a line moves with
// every edit above it.
//
// gatekit:fixture the reason each exempt writer gives — expected data about the tree.
var blobPutExemptions = map[string]string{
	"internal/modules/contacts/handlers_companylogo.go Handlers.writeBackTrimmedLogo": "overwrites a key a company row " +
		"already names with a trimmed copy of the same mark; recording it would let the reap delete a live logo",
	"internal/modules/privacy/suppressionjournal.go Eraser.ExportSuppressions": "the suppression journal is row-less " +
		"by design, outside every workspace prefix, and must survive a restore; the reap must never reach it",
}

func TestEveryBlobPutRecordsItsIntentOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	sites := sweepBlobPuts(t, blobPutRoots)
	// The tree carries several writers today, so nothing found means the sweep
	// went blind, and an empty census reports PASS.
	if len(sites) == 0 {
		t.Fatal("no blobstore Put call sites found, but the tree has several — the sweep has gone blind")
	}
	for _, problem := range judgeBlobPuts(sites, blobPutExemptions) {
		t.Error(problem)
	}
}

// TestTheBlobPutCensusCanFail plants the shapes the census exists to refuse and
// those it must accept, so a checker that stopped seeing records — or started
// seeing them everywhere — fails here rather than reading green over the tree.
func TestTheBlobPutCensusCanFail(t *testing.T) {
	t.Parallel()
	sites := plantedBlobPuts(t, "planted/writers.go", plantedBlobWriters)
	byFunc := map[string]blobPutSite{}
	for _, site := range sites {
		byFunc[site.function] = site
	}
	want := map[string]bool{
		"writer.putWithoutRecord":      false,
		"writer.recordAfterPut":        false,
		"writer.recordOtherKey":        false,
		"putThroughParameter":          false,
		"putThroughOtherPackage":       false,
		"writer.methodValue":           false,
		"writer.declareSiblingThenPut": false,
		"writer.recordThenPut":         true,
		"writer.declareThenPut":        true,
	}
	for function, records := range want {
		site, found := byFunc[function]
		if !found {
			t.Errorf("planted put in %s was not found: the census cannot see that receiver spelling", function)
			continue
		}
		if site.records != records {
			t.Errorf("planted put in %s judged records=%v, want %v", function, site.records, records)
		}
	}
	if len(sites) != len(want) {
		t.Errorf("found %d planted puts, want %d: %v", len(sites), len(want), sites)
	}

	exemptions := map[string]string{
		"planted/writers.go writer.putWithoutRecord": "planted exemption that holds",
		"planted/writers.go writer.recordAfterPut":   " ",
		"planted/writers.go writer.recordThenPut":    "planted stale exemption",
		"planted/writers.go gone":                    "planted exemption on a function that puts nothing",
	}
	if problems := judgeBlobPuts(sites, exemptions); len(problems) != 8 {
		t.Errorf("judging the planted writers raised %d problems, want 8 — five unrecorded puts with no "+
			"exemption, one exemption with no reason, one stale exemption and one exemption that puts "+
			"nothing: %q", len(problems), problems)
	}
}

// plantedBlobWriters is a package with each shape of writer the census judges.
const plantedBlobWriters = `package planted

import (
	"context"
	"io"

	"example.com/wiring"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	so "github.com/margince/margince/backend/internal/platform/storedobject"
)

type writer struct {
	blobs blobstore.Store
	db    any
}

func (w writer) putWithoutRecord(ctx context.Context, key string, r io.Reader) error {
	return w.blobs.Put(ctx, key, r, 1, "text/plain")
}

func (w writer) recordAfterPut(ctx context.Context, key string, r io.Reader) error {
	if err := w.blobs.Put(ctx, key, r, 1, "text/plain"); err != nil {
		return err
	}
	return so.Record(ctx, w.db, key)
}

func (w writer) recordOtherKey(ctx context.Context, key, other string, r io.Reader) error {
	if err := so.Record(ctx, w.db, other); err != nil {
		return err
	}
	return w.blobs.Put(ctx, key, r, 1, "text/plain")
}

func putThroughParameter(ctx context.Context, store blobstore.Store, key string, r io.Reader) error {
	return store.Put(ctx, key, r, 1, "text/plain")
}

// The loose check stands wiring in empty, so this receiver has no type at all.
func putThroughOtherPackage(ctx context.Context, deps wiring.Deps, key string, r io.Reader) error {
	return deps.Blobs.Put(ctx, key, r, 1, "text/plain")
}

func (w writer) methodValue(ctx context.Context, key string, r io.Reader) error {
	put := w.blobs.Put
	return put(ctx, key, r, 1, "text/plain")
}

func (w writer) recordThenPut(ctx context.Context, key string, r io.Reader) error {
	if err := so.Record(ctx, w.db, key); err != nil {
		return err
	}
	return w.blobs.Put(ctx, key, r, 1, "text/plain")
}

func (w writer) declare(ctx context.Context, key string) error { return so.Record(ctx, w.db, key) }

func (w writer) declareThenPut(ctx context.Context, key string, r io.Reader) error {
	if err := w.declare(ctx, key); err != nil {
		return err
	}
	return w.blobs.Put(ctx, key, r, 1, "text/plain")
}

func (w writer) declareSibling(ctx context.Context, key string) error {
	return so.Record(ctx, w.db, key+".tmp")
}

func (w writer) declareSiblingThenPut(ctx context.Context, key string, r io.Reader) error {
	if err := w.declareSibling(ctx, key); err != nil {
		return err
	}
	return w.blobs.Put(ctx, key, r, 1, "text/plain")
}
`

// blobPutSite is one put into the object store and what precedes it.
type blobPutSite struct {
	path     string
	function string
	line     int
	// records is true when, earlier in the same function, a call to
	// storedobject.Record — or to a function of this package that makes one —
	// takes the put's key.
	records bool
}

func (s blobPutSite) key() string { return s.path + " " + s.function }

// judgeBlobPuts holds the sites and the exemptions to each other: a put that
// records needs no entry, and one that does not needs a reason.
func judgeBlobPuts(sites []blobPutSite, exemptions map[string]string) []string {
	var problems []string
	seen := map[string]bool{}
	for _, site := range sites {
		seen[site.key()] = true
		reason, exempt := exemptions[site.key()]
		switch {
		case !site.records && exempt && strings.TrimSpace(reason) == "":
			problems = append(problems, site.describe("is in blobPutExemptions with no reason: say why its key "+
				"is not provisional"))
		case !site.records && !exempt:
			problems = append(problems, site.describe("puts into the object store and no storedobject.Record of "+
				"the put's key precedes it in this function. Record it on its own transaction before the put and "+
				"storedobject.Clear it on the row's — or, if the key is not provisional, add the function to "+
				"blobPutExemptions with why"))
		case site.records && exempt:
			problems = append(problems, site.describe("records its intent, so its entry in blobPutExemptions is "+
				"stale: drop it"))
		}
	}
	for key := range exemptions {
		if !seen[key] {
			problems = append(problems, key+" is in blobPutExemptions but puts nothing into the object "+
				"store any more: drop the entry")
		}
	}
	slices.Sort(problems)
	return problems
}

func (s blobPutSite) describe(what string) string {
	return s.path + ":" + strconv.Itoa(s.line) + " (" + s.function + ") " + what
}

// sweepBlobPuts parses every production Go file under roots and returns the
// puts each package makes. testdata is skipped because Go builds nothing in it.
func sweepBlobPuts(t *testing.T, roots []string) []blobPutSite {
	t.Helper()
	byDir := map[string][]*ast.File{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := gatekit.ParseFile(path, 0)
			if err != nil {
				return err
			}
			byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], file)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	var sites []blobPutSite
	for dir, files := range byDir {
		sites = append(sites, blobPutsInPackage(gatekit.SourceFileSet(), dir, files)...)
	}
	slices.SortFunc(sites, func(a, b blobPutSite) int { return strings.Compare(a.key(), b.key()) })
	return sites
}

// plantedBlobPuts checks one synthetic file as its own package.
func plantedBlobPuts(t *testing.T, name, src string) []blobPutSite {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		t.Fatalf("parsing planted %s: %v", name, err)
	}
	return blobPutsInPackage(fset, filepath.Dir(name), []*ast.File{file})
}

// blobPutsInPackage type-checks one package and returns each put it makes.
func blobPutsInPackage(fset *token.FileSet, dir string, files []*ast.File) []blobPutSite {
	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	// The errors a loose check reports are references into its stand-ins,
	// which it moves past; a check that defined nothing at all is the failure.
	conf := types.Config{Importer: blobstoreFromSource{}, Error: func(error) {}}
	if _, err := conf.Check(packagePathOf(dir), fset, files, info); err != nil && len(info.Defs) == 0 {
		return []blobPutSite{{path: filepath.ToSlash(dir), function: "type check failed: " + err.Error()}}
	}

	decls := map[*types.Func]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
				if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
					decls[obj] = fn
				}
			}
		}
	}
	pkg := blobPackage{info: info, decls: decls}
	var sites []blobPutSite
	for _, file := range files {
		path := filepath.ToSlash(fset.Position(file.Pos()).Filename)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			for _, put := range pkg.putsIn(fn) {
				sites = append(sites, blobPutSite{
					path: path, function: qualifiedFuncName(fn),
					line: fset.Position(put.sel.Pos()).Line, records: pkg.recordsBefore(put),
				})
			}
		}
	}
	return sites
}

// packagePathOf names a backend directory by its import path, so the blobstore
// package checked as a subject carries the path its importers see.
func packagePathOf(dir string) string {
	slashed := filepath.ToSlash(dir)
	if strings.HasPrefix(slashed, "..") {
		return slashed
	}
	return modulePath + "/" + slashed
}

type blobPackage struct {
	info  *types.Info
	decls map[*types.Func]*ast.FuncDecl
}

// blobPut is one put, with the function body it is made in — the innermost,
// so a put inside a closure answers to what the closure did first.
type blobPut struct {
	sel  *ast.SelectorExpr
	key  ast.Expr // nil for a method value, which names no key yet
	body *ast.BlockStmt
}

func (pkg blobPackage) putsIn(fn *ast.FuncDecl) []blobPut {
	var puts []blobPut
	bodies := []*ast.BlockStmt{fn.Body}
	var walk func(n ast.Node) bool
	walk = func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			bodies = append(bodies, v.Body)
			ast.Inspect(v.Body, walk)
			bodies = bodies[:len(bodies)-1]
			return false
		case *ast.CallExpr:
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok && pkg.isBlobPut(sel, v.Args) {
				var key ast.Expr
				if len(v.Args) == 5 {
					key = v.Args[1]
				}
				puts = append(puts, blobPut{sel: sel, key: key, body: bodies[len(bodies)-1]})
				for _, arg := range v.Args {
					ast.Inspect(arg, walk)
				}
				return false
			}
		case *ast.SelectorExpr:
			// Reached only outside a call's Fun: a method value, put later.
			if pkg.isBlobPut(v, nil) {
				puts = append(puts, blobPut{sel: v, body: bodies[len(bodies)-1]})
			}
		}
		return true
	}
	ast.Inspect(fn.Body, walk)
	return puts
}

// isBlobPut reports whether sel names blobstore's Put. A five-argument Put
// whose receiver the loose check could not type, or typed only as some
// interface, is kept too: it may be the store reached through a package the
// check stood in empty, and a census must not under-read.
func (pkg blobPackage) isBlobPut(sel *ast.SelectorExpr, args []ast.Expr) bool {
	if sel.Sel.Name != "Put" {
		return false
	}
	if obj, ok := pkg.info.Uses[sel.Sel].(*types.Func); ok && obj.Pkg() != nil && obj.Pkg().Path() == blobstorePath {
		return true
	}
	if len(args) != 5 {
		return false
	}
	recv := pkg.info.TypeOf(sel.X)
	if recv == nil || recv == types.Typ[types.Invalid] {
		return true
	}
	_, isInterface := recv.Underlying().(*types.Interface)
	return isInterface
}

// recordsBefore reports whether a recording call taking the put's key comes
// before the put in the put's own function body.
func (pkg blobPackage) recordsBefore(put blobPut) bool {
	if put.key == nil {
		return false
	}
	key := types.ExprString(put.key)
	found := false
	ast.Inspect(put.body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if found || !ok || call.Pos() >= put.sel.Pos() {
			return !found
		}
		if pkg.recordsKey(call, key, map[*ast.FuncDecl]bool{}) {
			found = true
		}
		return !found
	})
	return found
}

// recordsKey reports whether call is storedobject.Record of the key spelled
// key, or a call to a function of this package that hands the key on to one —
// a wrapper is verified down to the parameter the key arrives in, not listed.
func (pkg blobPackage) recordsKey(call *ast.CallExpr, key string, seen map[*ast.FuncDecl]bool) bool {
	var name *ast.Ident
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		if qualifier, ok := fun.X.(*ast.Ident); ok {
			if imported, ok := pkg.info.Uses[qualifier].(*types.PkgName); ok {
				// Record(ctx, db, key): the key is the third argument.
				return imported.Imported().Path() == storedObjectPkg && fun.Sel.Name == "Record" &&
					len(call.Args) == 3 && types.ExprString(call.Args[2]) == key
			}
		}
		name = fun.Sel
	case *ast.Ident:
		name = fun
	default:
		return false
	}
	callee, ok := pkg.info.Uses[name].(*types.Func)
	if !ok {
		return false
	}
	decl := pkg.decls[callee]
	if decl == nil || seen[decl] {
		return false
	}
	seen[decl] = true
	params := paramNames(decl)
	found := false
	for i, arg := range call.Args {
		if found || i >= len(params) || types.ExprString(arg) != key || params[i] == "_" {
			continue
		}
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			if inner, ok := n.(*ast.CallExpr); ok && !found && pkg.recordsKey(inner, params[i], seen) {
				found = true
			}
			return !found
		})
	}
	return found
}

// paramNames lists a function's parameters by position, as its callers pass
// arguments: the receiver is not one of them.
func paramNames(decl *ast.FuncDecl) []string {
	var names []string
	for _, field := range decl.Type.Params.List {
		if len(field.Names) == 0 {
			names = append(names, "_")
		}
		for _, ident := range field.Names {
			names = append(names, ident.Name)
		}
	}
	return names
}

// blobstoreFromSource stands every import in empty, as typeCheckLoosely does,
// except blobstore, which it checks from source so blobstore.Store resolves.
type blobstoreFromSource struct{}

func (blobstoreFromSource) Import(importPath string) (*types.Package, error) {
	if importPath != blobstorePath {
		return standInImports{}.Import(importPath)
	}
	return checkedBlobstore()
}

var checkedBlobstore = sync.OnceValues(func() (*types.Package, error) {
	dir := strings.TrimPrefix(blobstorePath, modulePath+"/")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(dir, name), 0)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	// Checked as loosely as its importers, so its errors are stand-in
	// references too; only a check that built no package is a failure.
	conf := types.Config{Importer: standInImports{}, Error: func(error) {}}
	pkg, err := conf.Check(blobstorePath, gatekit.SourceFileSet(), files, nil)
	if pkg == nil {
		return nil, err
	}
	return pkg, nil
})
