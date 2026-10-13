// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// An import's rows are written only by the worker. A commit or an undo runs as
// fenced slices on the import queues. A request that walked the engine itself,
// or spawned a goroutine to, would write rows that no fence, lock or drain
// window covers.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	migrationPackageDir = "internal/modules/migration"
	composePackageDir   = "internal/compose"
	// importQueuePrefix names the queues an import's heavy kinds run on. The
	// run sweep is on the default queue. It settles runs one statement at a
	// time, through constructors every handler calls.
	importQueuePrefix = "import_"
)

// importEntryPoint names one function that writes an import's rows.
type importEntryPoint struct{ pkg, recv, name string }

// importEngineEntryPoints are the engine's walks over a source or a run's rows.
// A Work method reaches them through the slice function it hands its slicer.
// Engine.Run, the unsliced flip, has no caller a Work method could name.
// DryRun is the test run's walk, which holds a snapshot for the whole file.
// createMissing writes the tags a file names before its rows, through no seam.
var importEngineEntryPoints = []importEntryPoint{
	{"migration", "Engine", "Run"},
	{"migration", "Engine", "DryRun"},
	{"migration", "Engine", "RunSlice"},
	{"migration", "RunStore", "UndoSlice"},
	{"compose", "tagResolver", "createMissing"},
}

// importRowWriterSeams are the interfaces whose every implementation writes an
// import's rows, by the method the engine calls. Exists is the read the test
// run classifies its rows with, so it stays out.
var importRowWriterSeams = []struct {
	pkg, iface string
	reads      map[string]bool
}{
	{"migration", "Writers", map[string]bool{"Exists": true}},
	{"migration", "UndoWriters", nil},
}

// importHandlersFloor is under the methods importHandlers has, so a walk that
// stopped recognising its receiver cannot pass by reading none of them.
const importHandlersFloor = 20

func TestImportRowsAreWrittenOnlyByTheImportJobs(t *testing.T) {
	t.Parallel()
	g := loadImportCallGraph(t, migrationPackageDir, composePackageDir, "cmd")
	findings, err := g.heavyCallFindings(importHeavyArgsTypes(t), importEngineEntryPoints)
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		t.Error(finding)
	}
	// Every other import stands in empty, and what that leaves unresolved is
	// dropped. A slice with no caller here means the check resolved nothing.
	for _, entry := range []string{"RunSlice", "UndoSlice"} {
		for fn, obj := range g.owner {
			if g.pkgOf[fn] == "migration" && fn.Name.Name == entry && len(g.sites[obj]) == 0 {
				t.Errorf("the census found no caller of %s: it has stopped resolving calls into migration", g.name(fn))
			}
		}
	}
}

func TestNoImportHandlerSpawnsAGoroutine(t *testing.T) {
	t.Parallel()
	fset, files := parseGoFilesUnder(t, composePackageDir)
	methods := 0
	for _, file := range files {
		if filepath.ToSlash(filepath.Dir(fset.Position(file.Pos()).Filename)) != composePackageDir {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || receiverTypeName(fn) != "importHandlers" {
				continue
			}
			methods++
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if spawn, ok := n.(*ast.GoStmt); ok {
					t.Errorf("%s: importHandlers.%s starts a goroutine. A request queues an import's work "+
						"and answers; the slice runs in the worker under its fence and its drain window",
						fset.Position(spawn.Pos()), fn.Name.Name)
				}
				return true
			})
		}
	}
	if methods < importHandlersFloor {
		t.Fatalf("read %d importHandlers methods, expected at least %d: the walk stopped recognising the receiver",
			methods, importHandlersFloor)
	}
}

// importHeavyArgsTypes is the args type of every import kind on an import
// queue, read from api/jobs.yaml.
func importHeavyArgsTypes(t *testing.T) []string {
	t.Helper()
	var goTypes []string
	for kind, spec := range jobs.Declared() {
		if strings.HasPrefix(kind, "import_") && strings.HasPrefix(spec.Queue, importQueuePrefix) {
			goTypes = append(goTypes, spec.GoType)
		}
	}
	if len(goTypes) == 0 {
		t.Fatalf("api/jobs.yaml declares no import kind on a %s* queue, so this gate would guard nothing", importQueuePrefix)
	}
	return goTypes
}

// importCallGraph records, for every function declared in the packages read,
// each place another declaration names it: a call, or a method value handed on.
// A call through an interface names every method that could answer it.
type importCallGraph struct {
	fset  *token.FileSet
	decls map[*types.Func]*ast.FuncDecl
	owner map[*ast.FuncDecl]*types.Func
	pkgOf map[*ast.FuncDecl]string
	sites map[*types.Func][]callSite
	// methods holds the declared methods by methodKey.
	methods  map[string][]*types.Func
	packages []*types.Package
}

// callSite is one reference; in is nil for a package-level initializer.
type callSite struct {
	in  *ast.FuncDecl
	pos token.Pos
}

// loadImportCallGraph type-checks the packages under dirs, the first dir's
// package before the rest, so a call into migration resolves to its body.
// The arch gates keep every importer of migration in compose or cmd.
func loadImportCallGraph(t *testing.T, dirs ...string) *importCallGraph {
	t.Helper()
	var groups [][]*ast.File
	fset := gatekit.SourceFileSet()
	for _, dir := range dirs {
		_, files := parseGoFilesUnder(t, dir)
		byPackage := map[string][]*ast.File{}
		for _, file := range files {
			key := filepath.Dir(fset.Position(file.Pos()).Filename) + "\x00" + file.Name.Name
			byPackage[key] = append(byPackage[key], file)
		}
		for _, key := range slices.Sorted(maps.Keys(byPackage)) {
			groups = append(groups, byPackage[key])
		}
	}
	return newImportCallGraph(fset, groups)
}

// newImportCallGraph checks each group in order. A later group importing an
// earlier one sees its real types; every other import stands in empty.
func newImportCallGraph(fset *token.FileSet, groups [][]*ast.File) *importCallGraph {
	g := &importCallGraph{
		fset: fset, decls: map[*types.Func]*ast.FuncDecl{}, owner: map[*ast.FuncDecl]*types.Func{},
		pkgOf: map[*ast.FuncDecl]string{}, sites: map[*types.Func][]callSite{}, methods: map[string][]*types.Func{},
	}
	checked := sharedImports{}
	infos := make([]*types.Info, len(groups))
	for i, files := range groups {
		dir := filepath.ToSlash(filepath.Dir(fset.Position(files[0].Pos()).Filename))
		infos[i] = &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
		conf := types.Config{Importer: checked, Error: func(error) {}}
		pkg, _ := conf.Check(modulePath+"/"+dir, fset, files, infos[i])
		checked[pkg.Path()] = pkg
		g.packages = append(g.packages, pkg)
		g.index(files, infos[i])
	}
	for i, files := range groups {
		g.collectSites(files, infos[i])
	}
	return g
}

// sharedImports answers a package already checked with its real types.
type sharedImports map[string]*types.Package

func (s sharedImports) Import(importPath string) (*types.Package, error) {
	if pkg, ok := s[importPath]; ok {
		return pkg, nil
	}
	return standInImports{}.Import(importPath)
}

func (g *importCallGraph) index(files []*ast.File, info *types.Info) {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
				g.decls[obj], g.owner[fn], g.pkgOf[fn] = fn, obj, file.Name.Name
				if fn.Recv != nil {
					g.methods[methodKey(obj)] = append(g.methods[methodKey(obj)], obj)
				}
			}
		}
	}
}

func (g *importCallGraph) collectSites(files []*ast.File, info *types.Info) {
	for _, file := range files {
		for _, decl := range file.Decls {
			var in *ast.FuncDecl
			if fn, ok := decl.(*ast.FuncDecl); ok {
				in = fn
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				ident, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				used, ok := info.Uses[ident].(*types.Func)
				if !ok {
					return true
				}
				for _, callee := range g.callees(used.Origin()) {
					g.sites[callee] = append(g.sites[callee], callSite{in: in, pos: ident.Pos()})
				}
				return true
			})
		}
	}
}

// methodKey names a method by its name and its parameter and result types,
// never their names. Two packages read apart print a stand-in type alike, so
// the text is compared.
func methodKey(fn *types.Func) string {
	var key strings.Builder
	key.WriteString(fn.Name())
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return key.String()
	}
	for _, tuple := range []*types.Tuple{sig.Params(), sig.Results()} {
		key.WriteString("|")
		for v := range tuple.Variables() {
			key.WriteString(types.TypeString(v.Type(), nil) + ",")
		}
	}
	if sig.Variadic() {
		key.WriteString("...")
	}
	return key.String()
}

// callees is the declared functions a use of fn may run: fn itself, or every
// method that could answer it when fn is an interface's.
func (g *importCallGraph) callees(fn *types.Func) []*types.Func {
	if g.decls[fn] != nil {
		return []*types.Func{fn}
	}
	if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil && types.IsInterface(sig.Recv().Type()) {
		return g.methods[methodKey(fn)]
	}
	return nil
}

// heavyCallFindings derives the heavy set from the Work methods of argsTypes,
// the entry points and every row writer of the seams. It reports every
// reference to one from outside the work only those Work methods reach.
func (g *importCallGraph) heavyCallFindings(argsTypes []string, entries []importEntryPoint) ([]string, error) {
	roots, err := g.workMethodsOf(argsTypes)
	if err != nil {
		return nil, err
	}
	heavy := map[*types.Func]bool{}
	for obj, sites := range g.sites {
		for _, site := range sites {
			if roots[site.in] {
				heavy[obj] = true
			}
		}
	}
	for _, entry := range entries {
		obj := g.method(entry.pkg, entry.recv, entry.name)
		if obj == nil {
			return nil, fmt.Errorf("the heavy-work census found no %s.%s.%s; it has stopped reading the tree it guards",
				entry.pkg, entry.recv, entry.name)
		}
		heavy[obj] = true
	}
	if err := g.addRowWriters(heavy); err != nil {
		return nil, err
	}
	inJob := g.reachedOnlyFrom(roots, heavy)
	var findings []string
	for _, obj := range slices.SortedFunc(maps.Keys(heavy), g.byPosition) {
		for _, site := range g.sites[obj] {
			if !inJob[site.in] {
				findings = append(findings, g.finding(obj, site))
			}
		}
	}
	return findings, nil
}

// addRowWriters marks heavy every declared method that implements a writing
// method of a row-writer seam.
func (g *importCallGraph) addRowWriters(heavy map[*types.Func]bool) error {
	for _, seam := range importRowWriterSeams {
		iface := g.seam(seam.pkg, seam.iface)
		if iface == nil {
			return fmt.Errorf("the heavy-work census found no %s.%s; it has stopped reading the tree it guards", seam.pkg, seam.iface)
		}
		impls := g.implementations(iface)
		if len(impls) == 0 {
			return fmt.Errorf("the heavy-work census found no implementation of %s.%s; it reads none of the writers it guards",
				seam.pkg, seam.iface)
		}
		for _, impl := range impls {
			for _, m := range impl {
				if !seam.reads[m.Name()] && g.decls[m] != nil {
					heavy[m] = true
				}
			}
		}
	}
	return nil
}

func (g *importCallGraph) seam(pkg, name string) *types.Interface {
	for _, p := range g.packages {
		if p.Name() != pkg {
			continue
		}
		if obj, ok := p.Scope().Lookup(name).(*types.TypeName); ok {
			if iface, ok := obj.Type().Underlying().(*types.Interface); ok {
				return iface
			}
		}
	}
	return nil
}

// implementations answers, for every named type of the packages read that has
// all of iface's methods, the methods that implement them.
func (g *importCallGraph) implementations(iface *types.Interface) [][]*types.Func {
	var found [][]*types.Func
	for _, p := range g.packages {
		for _, name := range p.Scope().Names() {
			obj, ok := p.Scope().Lookup(name).(*types.TypeName)
			if !ok || types.IsInterface(obj.Type()) {
				continue
			}
			byKey := map[string]*types.Func{}
			for sel := range types.NewMethodSet(types.NewPointer(obj.Type())).Methods() {
				if m, ok := sel.Obj().(*types.Func); ok {
					byKey[methodKey(m)] = m
				}
			}
			var impl []*types.Func
			for m := range iface.Methods() {
				if hit := byKey[methodKey(m)]; hit != nil {
					impl = append(impl, hit)
				}
			}
			if len(impl) == iface.NumMethods() {
				found = append(found, impl)
			}
		}
	}
	return found
}

func (g *importCallGraph) finding(obj *types.Func, site callSite) string {
	where := "a package-level value"
	if site.in != nil {
		where = g.name(site.in)
	}
	return g.fset.Position(site.pos).String() + ": " + where + " reaches " + g.name(g.decls[obj]) +
		", which writes an import's rows or tests its whole file. Only an import job's Work method, or work nothing but " +
		"one reaches, may reach it: a request queues the job and answers"
}

// reachedOnlyFrom is the work only the roots reach: the roots, every heavy
// function, and each function named somewhere and only from that work.
func (g *importCallGraph) reachedOnlyFrom(roots map[*ast.FuncDecl]bool, heavy map[*types.Func]bool) map[*ast.FuncDecl]bool {
	inJob := maps.Clone(roots)
	for obj := range heavy {
		inJob[g.decls[obj]] = true
	}
	for grew := true; grew; {
		grew = false
		for fn, obj := range g.owner {
			if inJob[fn] || len(g.sites[obj]) == 0 {
				continue
			}
			if !slices.ContainsFunc(g.sites[obj], func(site callSite) bool { return !inJob[site.in] }) {
				inJob[fn], grew = true, true
			}
		}
	}
	return inJob
}

// workMethodsOf finds the one Work method taking *river.Job[T] for each T.
func (g *importCallGraph) workMethodsOf(argsTypes []string) (map[*ast.FuncDecl]bool, error) {
	roots := map[*ast.FuncDecl]bool{}
	for _, argsType := range argsTypes {
		found := 0
		for fn := range g.owner {
			if fn.Recv != nil && fn.Name.Name == "Work" && worksArgs(fn, argsType) {
				roots[fn] = true
				found++
			}
		}
		if found != 1 {
			return nil, fmt.Errorf("the heavy-work census found %d Work methods taking *river.Job[%s], want one; "+
				"it has stopped reading the tree it guards", found, argsType)
		}
	}
	return roots, nil
}

// worksArgs reports whether fn's last parameter is *river.Job[argsType].
func worksArgs(fn *ast.FuncDecl, argsType string) bool {
	params := fn.Type.Params.List
	if len(params) == 0 {
		return false
	}
	star, ok := params[len(params)-1].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	index, ok := star.X.(*ast.IndexExpr)
	if !ok {
		return false
	}
	job, ok := index.X.(*ast.SelectorExpr)
	arg, isIdent := index.Index.(*ast.Ident)
	return ok && isIdent && job.Sel.Name == "Job" && arg.Name == argsType
}

func (g *importCallGraph) method(pkg, recv, name string) *types.Func {
	for fn, obj := range g.owner {
		if g.pkgOf[fn] == pkg && receiverTypeName(fn) == recv && fn.Name.Name == name {
			return obj
		}
	}
	return nil
}

func (g *importCallGraph) name(fn *ast.FuncDecl) string {
	if recv := receiverTypeName(fn); recv != "" {
		return g.pkgOf[fn] + "." + recv + "." + fn.Name.Name
	}
	return g.pkgOf[fn] + "." + fn.Name.Name
}

func (g *importCallGraph) byPosition(a, b *types.Func) int {
	return strings.Compare(g.fset.Position(a.Pos()).String(), g.fset.Position(b.Pos()).String())
}

// Each planted package names the references the census must report, by the
// function they sit in.
func TestTheHeavyWorkCensusReportsAReachOutsideTheJob(t *testing.T) {
	t.Parallel()
	const prelude = `package migration
type CommitArgs struct{}
type Engine struct{}
func (e *Engine) Run() {}
func (e *Engine) RunSlice() {}
type RunStore struct{}
func (s *RunStore) UndoSlice(rows int) {}
type worker struct{ e *Engine }
func (w *worker) Work(_ int, _ *river.Job[CommitArgs]) error { return slice(w.commit) }
func slice(func()) error { return nil }
func (w *worker) commit() { w.e.RunSlice() }
type Writers interface{ Exists() bool; Ensure() }
type UndoWriters interface{ Reverse() }
type rows struct{}
func (rows) Exists() bool { return false }
func (rows) Ensure() {}
func (rows) Reverse() {}
type tagResolver struct{}
func (*tagResolver) createMissing() {}
`
	entries := []importEntryPoint{
		{"migration", "Engine", "Run"},
		{"migration", "Engine", "RunSlice"},
		{"migration", "RunStore", "UndoSlice"},
		{"migration", "tagResolver", "createMissing"},
	}
	for name, tc := range map[string]struct {
		src  string
		want []string
	}{
		"a slice reached only through the job": {``, nil},
		"a handler that walks the engine":      {`func approve(e *Engine) { e.RunSlice() }`, []string{"migration.approve"}},
		"a goroutine a handler spawns":         {`func approve(e *Engine) { go func() { e.Run() }() }`, []string{"migration.approve"}},
		"a handler that calls the job's slice": {`func approve(w *worker) { _ = slice(w.commit) }`, []string{"migration.approve", "migration.approve"}},
		"a helper the job and a handler share": {`func approve(w *worker) { w.commit() }`, []string{"migration.approve"}},
		"a package-level value":                {`var hook = (*RunStore).UndoSlice`, []string{"a package-level value"}},
		"a call through an interface":          {`type runner interface{ UndoSlice(n int) }; func approve(s *RunStore) { var r runner = s; r.UndoSlice(1) }`, []string{"migration.approve"}},
		"a handler that writes a row":          {`func approve(w rows) { w.Ensure() }`, []string{"migration.approve"}},
		"a handler that reverses a row":        {`func undo(w UndoWriters) { w.Reverse() }`, []string{"migration.undo"}},
		"a handler that creates the tags":      {`func approve(t *tagResolver) { t.createMissing() }`, []string{"migration.approve"}},
		"a test run that classifies a row":     {`func preview(w Writers) bool { return w.Exists() }`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fset := gatekit.SourceFileSet()
			file, err := parser.ParseFile(fset, filepath.Join("planted", "heavy.go"), prelude+tc.src, 0)
			if err != nil {
				t.Fatalf("parse the planted package: %v", err)
			}
			findings, err := newImportCallGraph(fset, [][]*ast.File{{file}}).heavyCallFindings([]string{"CommitArgs"}, entries)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, finding := range findings {
				_, rest, _ := strings.Cut(finding, ": ")
				where, _, _ := strings.Cut(rest, " reaches ")
				got = append(got, where)
			}
			slices.Sort(got)
			if !slices.Equal(got, tc.want) {
				t.Errorf("the census reports %q, want %q", got, tc.want)
			}
		})
	}
}
