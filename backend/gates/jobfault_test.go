// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// Every River worker returns through jobs.Fault. River stores whatever a
// Work method returns into river_job.errors verbatim — a column with no
// workspace, no RLS, and a fleet-wide audience. A worker that returns its
// raw cause publishes it there; this gate is what stops the next worker
// from doing so by habit.
//
// The rule is syntactic on purpose: a return in a Work body is nil, a
// jobs.Fault call, or one of River's own control returns. Anything else
// fails, which keeps the gate readable and impossible to argue with.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// workerFloor guards against a vacuous pass, as in the role gate.
const workerFloor = 20

// workerHome is where a River worker lives, and the return rule above only
// holds for workers this gate can see — it walks this one directory.
//
// Nothing used to assert that a worker IS here. The two rules leaned on each
// other with nothing underneath: the return rule assumed the location, and the
// location was a path constant nobody checked. A worker declared in a module
// package would be invisible, and invisible the quiet way — the gate stays
// green while the obligation goes unenforced for exactly that worker.
const workerHome = "internal/compose"

// workersOutsideTheirHome ratifies each Work method that lives elsewhere.
//
// A Work method is not always a River worker: the governed wrapper below
// declares one to DELEGATE, which is why the exemption is by name and carries
// what it is rather than being a directory this walk skips.
var workersOutsideTheirHome = gatekit.Waive(map[string]string{
	"internal/platform/jobs/govern.go": "the governing wrapper's own Work, which runs no job of its own: it decorates the worker it wraps and returns what that worker returned, so the return rule is satisfied by the wrapped worker and asking it of the wrapper would ask it twice of one value. It lives in platform because it is the seam every compose worker is registered through, and a seam that lived among the workers it governs would be one of them",
})

// TestEveryRiverWorkerLivesWhereTheFaultGateLooks is the other half of the
// return rule: it is only an obligation over the workers the walk above reads,
// so a worker somewhere else is one the rule does not reach.
//
// Derived from the tree rather than trusted: the walk is over the whole module,
// and every Work method it finds outside the home directory has to be named
// here with what it is. That way the location rule and the return rule hold
// each other up, instead of one assuming the other.
func TestEveryRiverWorkerLivesWhereTheFaultGateLooks(t *testing.T) {
	t.Parallel()
	defer workersOutsideTheirHome.AssertAllMatched(t)

	fset, files := parseGoFilesUnder(t, "internal")
	found, outside := 0, 0
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Work" || fn.Body == nil {
				continue
			}
			found++
			rel := filepath.ToSlash(filepath.Dir(fset.Position(fn.Pos()).Filename))
			if rel == workerHome || strings.HasPrefix(rel, workerHome+"/") {
				continue
			}
			outside++
			if workersOutsideTheirHome.Waived(t, filepath.ToSlash(fset.Position(fn.Pos()).Filename)) {
				continue
			}
			pos := fset.Position(fn.Pos())
			t.Errorf("%s:%d: a Work method outside %s is one TestEveryWorkerReturnsThroughJobsFault "+
				"never reads, so whatever it returns reaches river_job.errors unchecked — a column "+
				"with no workspace, no RLS and an installation-wide audience.\n"+
				"  Move it under %s, or name it in workersOutsideTheirHome with what it is.",
				pos.Filename, pos.Line, workerHome, workerHome)
		}
	}
	// The same vacuity floor the return rule takes, for the same reason: a walk
	// that stopped recognising Work methods would find nothing outside the home
	// and report a clean tree.
	if found < workerFloor {
		t.Fatalf("this walk found %d Work method(s) across the module and expects at least %d — "+
			"it has stopped recognising them rather than the tree having lost them", found, workerFloor)
	}
	t.Logf("Work methods: %d in all, %d outside %s", found, outside, workerHome)
}

func TestEveryWorkerReturnsThroughJobsFault(t *testing.T) {
	t.Parallel()
	// The ratified log-and-return-nil workers, each bound to the durable retry
	// policy that makes its green River row honest. They are DECLARED per kind
	// in api/jobs.yaml (fault: nil_after_logging) and joined to the receiver
	// name below through the registration that binds the two — a worker type
	// serves exactly one args type, because Work's signature names it, so the
	// join is one-to-one by construction. A worker not waived there must
	// return its failure.
	//
	// The set is derived rather than written, and gatekit then holds it to the
	// same two obligations as every hand-written one: a reason that states a
	// cost, and an entry that still describes live code.
	census, err := compose.NewJobCensus()
	if err != nil {
		t.Fatalf("building the job census: %v", err)
	}
	nilAfterLogging := gatekit.Waive(census.NilAfterLoggingWaivers())

	fset, files := parseGoFilesUnder(t, filepath.Join("internal", "compose"))
	packages := indexPackageFuncs(fset, files)
	workers := 0
	for _, file := range files {
		pkg := packages[filepath.Dir(fset.Position(file.Pos()).Filename)]
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Work" || fn.Body == nil {
				continue
			}
			workers++
			for _, ret := range topLevelReturns(fn.Body) {
				for _, result := range ret.Results {
					if sanctionedWorkerReturn(result) {
						continue
					}
					pos := fset.Position(result.Pos())
					t.Errorf("%s:%d: a worker return must be nil, jobs.Fault/FaultContext/FaultForKind(...), or a river control return — a raw cause is written verbatim into river_job.errors",
						pos.Filename, pos.Line)
				}
			}
			// A named error result reaches river_job.errors without passing
			// through any return statement: siteDeepReadWorker.Work declares
			// (workErr error) and its panic-recovery defer assigns it
			// directly (deepreadbudget.go). Checking returns alone would
			// wave that path through, so every assignment to the named
			// result takes the same test.
			for _, assigned := range pkg.namedResultAssignments(fn) {
				if sanctionedWorkerReturn(assigned) {
					continue
				}
				pos := fset.Position(assigned.Pos())
				t.Errorf("%s:%d: an assignment to a worker's named error result must be nil, jobs.Fault/FaultContext/FaultForKind(...), or a river control return — it reaches river_job.errors exactly as a return does",
					pos.Filename, pos.Line)
			}
			// The log-and-return-nil shape is the defect this phase removes,
			// one level up: the tenant failed, the operator sees a green row.
			// A worker that error-logs AND returns nil is that shape unless a
			// durable retry policy elsewhere makes the success honest.
			recv := receiverTypeName(fn)
			if logger := errorLogsAndReturnsNil(fn, pkg); logger != "" {
				if !nilAfterLogging.Waived(t, recv) {
					pos := fset.Position(fn.Pos())
					t.Errorf("%s:%d: %s logs an error (in %s) and returns nil — River will record this job as completed while the work failed. Return the failure, or ratify it in api/jobs.yaml with fault: {nil_after_logging: …} naming why its success is honest — a durable retry policy, or a log line that reports a finding rather than a failure.",
						pos.Filename, pos.Line, recv, logger)
				}
			}
		}
	}
	if workers < workerFloor {
		t.Fatalf("found only %d Work methods, expected at least %d — the walker matched nothing", workers, workerFloor)
	}
	assertCallsResolve(t, packages)
	// Staleness is only meaningful once the sweep above actually ran: on the
	// vacuity Fatal every entry would report as unmatched, burying the one
	// failure that explains all of them. An entry reported here names a worker
	// that no longer logs-and-returns-nil, so its kind's fault block in
	// api/jobs.yaml is what goes.
	nilAfterLogging.AssertAllMatched(t)
}

// assertCallsResolve fails when the loose type-check stopped resolving this
// tree's calls. The log census follows a call only when it resolves to a
// declaration here, so a check that resolved nothing would follow nothing and
// find no log anywhere — a clean result from a census that read nothing.
func assertCallsResolve(t *testing.T, packages map[string]*packageFuncs) {
	t.Helper()
	resolving, unresolved := 0, 0
	for _, pkg := range packages {
		unresolved += pkg.unresolved
		for _, fn := range pkg.decls {
			if fn.Name.Name == "Work" && fn.Recv != nil && pkg.resolvesACall(fn) {
				resolving++
			}
		}
	}
	if resolving < workerFloor {
		t.Fatalf("only %d Work methods make a call the census resolves into their own package, "+
			"expected at least %d — the type-check has stopped resolving this tree", resolving, workerFloor)
	}
	t.Logf("%d Work methods make a call into their own package; %d references into stood-in imports left unresolved",
		resolving, unresolved)
}

// resolvesACall reports whether fn makes any call the census follows.
func (pkg *packageFuncs) resolvesACall(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && pkg.callee(call) != nil {
			found = true
		}
		return !found
	})
	return found
}

// errorLogsAndReturnsNil reports where fn logs a failure when it also returns
// nil, naming the function holding the log call, or "" when it does not have
// that shape. Warn counts as well as Error: the defect is the SHAPE — a
// tenant's failure becoming a green River row — and the level a worker
// happened to log it at does not change what the operator sees in the job list.
// A heuristic, and deliberately a broad one: the cost of a false positive is
// writing one waiver with a rationale, while the cost of a false negative is
// a tenant failure that never surfaces anywhere.
//
// A bare return counts as returning nil: it hands back the named results, and
// a worker that logs a failure it caught in a shadowing `err :=` and then
// returns bare hands back a named error nothing assigned.
//
// The log is looked for through the calls Work makes into its own package, as
// well as in its own body: a worker that hands its failure to a helper which
// logs it, then returns nil, turns the same tenant failure into the same green
// row.
func errorLogsAndReturnsNil(fn *ast.FuncDecl, pkg *packageFuncs) string {
	returnsNil := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if ret, ok := n.(*ast.ReturnStmt); ok {
			for _, r := range ret.Results {
				if ident, ok := r.(*ast.Ident); ok && ident.Name == "nil" {
					returnsNil = true
				}
			}
		}
		return true
	})
	for _, ret := range topLevelReturns(fn.Body) {
		returnsNil = returnsNil || len(ret.Results) == 0
	}
	if !returnsNil {
		return ""
	}
	return pkg.loggerIn(fn, map[*ast.FuncDecl]bool{})
}

// packageFuncs is one package's declarations, type-checked so a call from a
// worker can be followed to the body it runs and a variable told from another
// of the same name.
type packageFuncs struct {
	info  *types.Info
	decls map[*types.Func]*ast.FuncDecl
	// unresolved counts the references the check could not resolve — each
	// one into an import, which stands in empty; see typeCheckLoosely.
	unresolved int
}

// indexPackageFuncs type-checks the files of each directory — each package —
// on its own.
func indexPackageFuncs(fset *token.FileSet, files []*ast.File) map[string]*packageFuncs {
	byDir := map[string][]*ast.File{}
	for _, file := range files {
		dir := filepath.Dir(fset.Position(file.Pos()).Filename)
		byDir[dir] = append(byDir[dir], file)
	}
	packages := map[string]*packageFuncs{}
	for dir, group := range byDir {
		info, unresolved := typeCheckLoosely(fset, dir, group)
		pkg := &packageFuncs{info: info, decls: map[*types.Func]*ast.FuncDecl{}, unresolved: unresolved}
		for _, file := range group {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
					if obj, ok := info.Defs[fn.Name].(*types.Func); ok {
						pkg.decls[obj] = fn
					}
				}
			}
		}
		packages[dir] = pkg
	}
	return packages
}

// typeCheckLoosely checks one package with every import standing in as an
// empty package, and reports how many references that left unresolved.
//
// The walk follows only this package's own declarations, and those resolve
// without the imports: a local, a field of a local type, a method on one. What
// a stand-in costs is a call on a value of an IMPORTED type, which the walk
// would not follow in any case. The checker reports each reference into a
// stand-in and moves past it, so one pass resolves everything else.
func typeCheckLoosely(fset *token.FileSet, dir string, files []*ast.File) (*types.Info, int) {
	info := &types.Info{
		Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{},
		Types: map[ast.Expr]types.TypeAndValue{}, Selections: map[*ast.SelectorExpr]*types.Selection{},
	}
	unresolved := 0
	conf := types.Config{Importer: standInImports{}, Error: func(error) { unresolved++ }}
	if _, err := conf.Check(dir, fset, files, info); err == nil {
		return info, 0
	}
	return info, unresolved
}

// standInImports answers every import with an empty package named for the
// last element of its path.
type standInImports struct{}

func (standInImports) Import(importPath string) (*types.Package, error) {
	pkg := types.NewPackage(importPath, path.Base(importPath))
	pkg.MarkComplete()
	return pkg, nil
}

// method is the declaration of the method named name on the type named recv.
func (pkg *packageFuncs) method(recv, name string) *ast.FuncDecl {
	for _, fn := range pkg.decls {
		if fn.Name.Name == name && receiverTypeName(fn) == recv {
			return fn
		}
	}
	return nil
}

// loggerIn names the function, fn or one it hands a failure to in this
// package, that makes an error or warn log call, or "" when none does. Calls
// are followed without a depth limit; seen stops a recursive pair from looping.
//
// A call is followed when nothing fn returns carries its result: one made as
// a statement — plain, deferred or spawned — or one whose assigned variables no
// return statement reads. That is the call a failure disappears into. A helper
// whose error fn returns has handed the failure back, and whatever it logged on
// the way is the same failure Work then reports through jobs.Fault.
//
// Variables and calls are resolved by type, not by name: a shadowing `err`
// that is never returned is not the `err` a later return reads, and a method
// reached through a field (w.engine.run) is followed to its body. A method
// called through an interface is not, because which body runs is decided at
// run time.
func (pkg *packageFuncs) loggerIn(fn *ast.FuncDecl, seen map[*ast.FuncDecl]bool) string {
	if seen[fn] {
		return ""
	}
	seen[fn] = true
	returned := pkg.returnedValues(fn)
	var logger string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if logger != "" {
			return false
		}
		switch v := n.(type) {
		case *ast.CallExpr:
			if isErrorLog(v) {
				logger = fn.Name.Name
			}
		case *ast.ExprStmt:
			if call, ok := v.X.(*ast.CallExpr); ok {
				logger = pkg.loggerThrough(call, seen)
			}
		case *ast.AssignStmt:
			if call, ok := singleCall(v.Rhs); ok && !pkg.assignsAny(v.Lhs, returned) {
				logger = pkg.loggerThrough(call, seen)
			}
		case *ast.DeferStmt:
			logger = pkg.loggerThrough(v.Call, seen)
		case *ast.GoStmt:
			logger = pkg.loggerThrough(v.Call, seen)
		}
		return logger == ""
	})
	return logger
}

// returnedValues holds the variables fn hands back: those a return statement
// reads, and fn's named results when a bare return hands those back.
func (pkg *packageFuncs) returnedValues(fn *ast.FuncDecl) map[types.Object]bool {
	returned := map[types.Object]bool{}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		ast.Inspect(ret, func(m ast.Node) bool {
			if ident, ok := m.(*ast.Ident); ok && pkg.info.Uses[ident] != nil {
				returned[pkg.info.Uses[ident]] = true
			}
			return true
		})
		if len(ret.Results) == 0 {
			for obj := range pkg.namedResults(fn) {
				returned[obj] = true
			}
		}
		return true
	})
	return returned
}

// namedResults is fn's named results, the blank one aside.
func (pkg *packageFuncs) namedResults(fn *ast.FuncDecl) map[types.Object]bool {
	named := map[types.Object]bool{}
	if fn.Type.Results == nil {
		return named
	}
	for _, result := range fn.Type.Results.List {
		for _, name := range result.Names {
			if obj := pkg.info.Defs[name]; obj != nil && name.Name != "_" {
				named[obj] = true
			}
		}
	}
	return named
}

// singleCall is the call an assignment's one right-hand side makes.
func singleCall(rhs []ast.Expr) (*ast.CallExpr, bool) {
	if len(rhs) != 1 {
		return nil, false
	}
	call, ok := rhs[0].(*ast.CallExpr)
	return call, ok
}

// assignsAny reports whether any assigned variable is one of objs.
func (pkg *packageFuncs) assignsAny(lhs []ast.Expr, objs map[types.Object]bool) bool {
	for _, expr := range lhs {
		if objs[pkg.assignedObject(expr)] {
			return true
		}
	}
	return false
}

// assignedObject is the variable an assignment to expr writes, declared there
// or before; nil for anything but a named variable.
func (pkg *packageFuncs) assignedObject(expr ast.Expr) types.Object {
	ident, ok := expr.(*ast.Ident)
	if !ok || ident.Name == "_" {
		return nil
	}
	if obj := pkg.info.Defs[ident]; obj != nil {
		return obj
	}
	return pkg.info.Uses[ident]
}

// loggerThrough is loggerIn of what call resolves to.
func (pkg *packageFuncs) loggerThrough(call *ast.CallExpr, seen map[*ast.FuncDecl]bool) string {
	if callee := pkg.callee(call); callee != nil {
		return pkg.loggerIn(callee, seen)
	}
	return ""
}

// isErrorLog reports a call to a method named for an error or warn log.
func isErrorLog(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	switch sel.Sel.Name {
	case "Error", "ErrorContext", "Warn", "WarnContext":
		return true
	}
	return false
}

// callee resolves call to its declaration in this package: a function by
// name, or a method through any value of a type declared here.
func (pkg *packageFuncs) callee(call *ast.CallExpr) *ast.FuncDecl {
	var ident *ast.Ident
	switch fun := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		ident = fun
	case *ast.SelectorExpr:
		ident = fun.Sel
	default:
		return nil
	}
	if f, ok := pkg.info.Uses[ident].(*types.Func); ok {
		return pkg.decls[f.Origin()]
	}
	return nil
}

// topLevelReturns collects the return statements belonging to fn itself,
// descending through control flow but NOT into nested function literals —
// a closure passed to a helper has its own contract and returns to its own
// caller, not to River.
func topLevelReturns(body *ast.BlockStmt) []*ast.ReturnStmt {
	var found []*ast.ReturnStmt
	ast.Inspect(body, func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			found = append(found, v)
			return false
		}
		return true
	})
	return found
}

// namedResultAssignments returns the values assigned to fn's named error
// results. A Work method with unnamed results has none, and the defer that
// assigns one is reached through a FuncLit — which topLevelReturns
// deliberately skips — so this walk descends into literals rather than
// stopping at them. A shadowing variable of the same name is a different
// variable, and what it is assigned never reaches River.
func (pkg *packageFuncs) namedResultAssignments(fn *ast.FuncDecl) []ast.Expr {
	named := pkg.namedResults(fn)
	if len(named) == 0 {
		return nil
	}
	var assigned []ast.Expr
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		stmt, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		// A tuple assignment (`x, workErr = f()`) has one Rhs for many Lhs, so
		// there is no per-index expression to test. It cannot be a sanctioned
		// return either — jobs.Fault returns one value — so the CALL is what
		// gets reported, rather than the index being skipped and the path
		// waved through.
		if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 {
			if pkg.assignsAny(stmt.Lhs, named) {
				assigned = append(assigned, stmt.Rhs[0])
			}
			return true
		}
		for i, lhs := range stmt.Lhs {
			if named[pkg.assignedObject(lhs)] && i < len(stmt.Rhs) {
				assigned = append(assigned, stmt.Rhs[i])
			}
		}
		return true
	})
	return assigned
}

// sanctionedWorkerReturn reports whether one returned expression is an
// allowed worker result.
func sanctionedWorkerReturn(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok && ident.Name == "nil" {
		return true
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	// Every spelling of the substitution seam, and no more. FaultForKind is the
	// composed-job spelling: it takes the River kind as well, which is what lets
	// it verify an extension's declared failure class before publishing its
	// sentence. All three replace the cause's text; a fourth entry here has to
	// do the same, or this gate stops meaning what it says.
	if pkg.Name == "jobs" && (sel.Sel.Name == "Fault" || sel.Sel.Name == "FaultContext" || sel.Sel.Name == "FaultForKind") {
		return true
	}
	// River's own control returns are not failures: a snooze reschedules and
	// a cancel is a deliberate stop. Neither carries a cause to publish.
	return pkg.Name == "river" && (sel.Sel.Name == "JobSnooze" || sel.Sel.Name == "JobCancel")
}

// The log-and-return-nil census has to see a log wherever Work hands a failure
// to be logged, and has to stop where the failure is handed back instead. Each
// planted worker names the function the census must report, or "" for none.
func TestTheJobFaultCensusSeesALogMadeThroughAHelper(t *testing.T) {
	t.Parallel()
	const prelude = "package planted\n"
	for name, tc := range map[string]struct{ src, want string }{
		"a log in Work itself": {`type worker struct{ log logger }
func (w *worker) Work() error { if err := run(); err != nil { w.log.Error("x", "err", err); return nil }; return nil }
func run() error { return nil }`, "Work"},
		"a package function Work hands the failure to": {`type worker struct{}
func (w *worker) Work() error { if err := run(); err != nil { logFailure(err) }; return nil }
func logFailure(err error) { slog.ErrorContext(nil, "x", "err", err) }
func run() error { return nil }`, "logFailure"},
		"a method on Work's own receiver": {`type worker struct{ log logger }
func (w *worker) Work() error { w.note(); return nil }
func (w *worker) note() { w.log.WarnContext(nil, "x") }`, "note"},
		"a log two calls down": {`type worker struct{}
func (w *worker) Work() error { outer(); return nil }
func outer() { inner() }
func inner() { slog.Warn("x") }`, "inner"},
		"a deferred helper": {`type worker struct{}
func (w *worker) Work() error { defer cleanup(); return nil }
func cleanup() { slog.Error("x") }`, "cleanup"},
		"a helper whose result Work reads but never returns": {`type worker struct{}
func (w *worker) Work() error { n := sweep(); _ = n; return nil }
func sweep() int { slog.Error("x"); return 0 }`, "sweep"},
		"a recursive pair that never logs": {`type worker struct{}
func (w *worker) Work() error { ping(); return nil }
func ping() { pong() }
func pong() { ping() }`, ""},
		"a helper whose failure Work returns": {`type worker struct{}
func (w *worker) Work() error { if err := check(); err != nil { return jobs.Fault(err) }; return nil }
func check() error { slog.Error("x"); return errors.New("x") }`, ""},
		"a helper logging while Work never returns nil": {`type worker struct{}
func (w *worker) Work() error { logFailure(); return jobs.Fault(nil) }
func logFailure() { slog.Error("x") }`, ""},
		"a bare return after a failure caught in a shadowing err": {`type worker struct{ log logger }
func (w *worker) Work() (err error) { if err := run(); err != nil { w.log.Error("x", "err", err); return }; return jobs.Fault(nil) }
func run() error { return nil }`, "Work"},
		"a helper assigned to a shadowing err no return reads": {`type worker struct{}
func (w *worker) Work() error {
	if err := logFailure(); err != nil { w.drop(err) }
	if err := check(); err != nil { return jobs.Fault(err) }
	return nil
}
func (w *worker) drop(error) {}
func logFailure() error { slog.Error("x"); return nil }
func check() error { return nil }`, "logFailure"},
		"a helper whose failure a bare return hands back": {`type worker struct{}
func (w *worker) Work() (err error) { if err = check(); err != nil { return }; return nil }
func check() error { slog.Error("x"); return errors.New("x") }`, ""},
		"a method reached through a field": {`type engine struct{}
func (e *engine) run() { slog.Error("x") }
type worker struct{ engine *engine }
func (w *worker) Work() error { w.engine.run(); return nil }`, "run"},
		"another package's function of the same name": {`type worker struct{}
func (w *worker) Work() error { other.logFailure(); return nil }
func logFailure() { slog.Error("x") }`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg, work := plantedWorker(t, prelude+tc.src)
			if got := errorLogsAndReturnsNil(work, pkg); got != tc.want {
				t.Errorf("the census reports a log in %q; want %q", got, tc.want)
			}
		})
	}
}

// A named error result reaches River through whatever is assigned to it, and
// only that: a shadowing `err :=` of the same name is another variable, and
// what it holds never reaches river_job.errors. Each planted worker lists the
// values the census must report as assigned to the named result.
func TestTheJobFaultCensusReadsANamedResultByVariable(t *testing.T) {
	t.Parallel()
	const prelude = "package planted\ntype worker struct{}\nfunc run() error { return nil }\nfunc pair() (int, error) { return 0, nil }\n"
	for name, tc := range map[string]struct {
		src  string
		want []string
	}{
		"a raw cause assigned, then a bare return": {`func (w *worker) Work() (err error) { err = run(); return }`, []string{"run()"}},
		"a tuple assigning the named result":       {`func (w *worker) Work() (err error) { _, err = pair(); return }`, []string{"pair()"}},
		"an assignment in a deferred literal":      {`func (w *worker) Work() (err error) { defer func() { err = run() }(); return nil }`, []string{"run()"}},
		"a shadowing err of the same name": {`func (w *worker) Work() (err error) {
	if err := run(); err != nil { return jobs.Fault(err) }
	return nil
}`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			pkg, work := plantedWorker(t, prelude+tc.src)
			var got []string
			for _, assigned := range pkg.namedResultAssignments(work) {
				got = append(got, types.ExprString(assigned))
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("the census reports %q assigned to the named result; want %q", got, tc.want)
			}
		})
	}
}

// plantedWorker type-checks one planted file and finds its worker's Work.
func plantedWorker(t *testing.T, src string) (*packageFuncs, *ast.FuncDecl) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("planted", "worker.go"), src, 0)
	if err != nil {
		t.Fatalf("parse the planted worker: %v", err)
	}
	pkg := indexPackageFuncs(fset, []*ast.File{file})["planted"]
	work := pkg.method("worker", "Work")
	if work == nil {
		t.Fatal("the planted worker declares no Work method the index can see")
	}
	return pkg, work
}
