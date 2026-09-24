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
	"path/filepath"
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
			for _, assigned := range namedResultAssignments(fn) {
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
					t.Errorf("%s:%d: %s logs an error (in %s) and returns nil — River will record this job as completed while the work failed. Return the failure, or ratify it in api/jobs.yaml with fault: {nil_after_logging: …} naming the retry policy that makes success honest.",
						pos.Filename, pos.Line, recv, logger)
				}
			}
		}
	}
	if workers < workerFloor {
		t.Fatalf("found only %d Work methods, expected at least %d — the walker matched nothing", workers, workerFloor)
	}
	// Staleness is only meaningful once the sweep above actually ran: on the
	// vacuity Fatal every entry would report as unmatched, burying the one
	// failure that explains all of them. An entry reported here names a worker
	// that no longer logs-and-returns-nil, so its kind's fault block in
	// api/jobs.yaml is what goes.
	nilAfterLogging.AssertAllMatched(t)
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
	if !returnsNil {
		return ""
	}
	return pkg.loggerIn(fn, map[*ast.FuncDecl]bool{})
}

// packageFuncs is one package's declarations, so a call from a worker can be
// followed to the body it runs.
type packageFuncs struct {
	funcs   map[string]*ast.FuncDecl
	methods map[string]map[string]*ast.FuncDecl
}

// indexPackageFuncs groups the declarations of files by the directory — the
// package — each lives in.
func indexPackageFuncs(fset *token.FileSet, files []*ast.File) map[string]*packageFuncs {
	packages := map[string]*packageFuncs{}
	for _, file := range files {
		dir := filepath.Dir(fset.Position(file.Pos()).Filename)
		pkg := packages[dir]
		if pkg == nil {
			pkg = &packageFuncs{funcs: map[string]*ast.FuncDecl{}, methods: map[string]map[string]*ast.FuncDecl{}}
			packages[dir] = pkg
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Recv == nil {
				pkg.funcs[fn.Name.Name] = fn
				continue
			}
			recv := receiverTypeName(fn)
			if pkg.methods[recv] == nil {
				pkg.methods[recv] = map[string]*ast.FuncDecl{}
			}
			pkg.methods[recv][fn.Name.Name] = fn
		}
	}
	return packages
}

// loggerIn names the function, fn or one it hands a failure to in this
// package, that makes an error or warn log call, or "" when none does. Calls
// are followed without a depth limit; seen stops a recursive pair from looping.
//
// Only a call made as a statement — plain, deferred or spawned — is followed:
// one whose result, if it has one, nobody reads. That is the call a failure disappears into: a helper whose
// error Work returns has handed the failure back, and whatever it logged on
// the way is the same failure Work then reports through jobs.Fault.
//
// What it resolves, without type information: a package-level function called
// by its bare name, and a method called through fn's own receiver. A method
// reached through a field or a local value is not followed, because its type is
// not visible to a syntactic walk.
func (pkg *packageFuncs) loggerIn(fn *ast.FuncDecl, seen map[*ast.FuncDecl]bool) string {
	if seen[fn] {
		return ""
	}
	seen[fn] = true
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
				logger = pkg.loggerThrough(fn, call, seen)
			}
		case *ast.DeferStmt:
			logger = pkg.loggerThrough(fn, v.Call, seen)
		case *ast.GoStmt:
			logger = pkg.loggerThrough(fn, v.Call, seen)
		}
		return logger == ""
	})
	return logger
}

// loggerThrough is loggerIn of what call, made inside fn, resolves to.
func (pkg *packageFuncs) loggerThrough(fn *ast.FuncDecl, call *ast.CallExpr, seen map[*ast.FuncDecl]bool) string {
	if callee := pkg.callee(fn, call); callee != nil {
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

// callee resolves call, made inside fn, to a declaration in this package.
func (pkg *packageFuncs) callee(fn *ast.FuncDecl, call *ast.CallExpr) *ast.FuncDecl {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return pkg.funcs[fun.Name]
	case *ast.SelectorExpr:
		recv, ok := fun.X.(*ast.Ident)
		if !ok || fn.Recv == nil || len(fn.Recv.List[0].Names) == 0 || fn.Recv.List[0].Names[0].Name != recv.Name {
			return nil
		}
		return pkg.methods[receiverTypeName(fn)][fun.Sel.Name]
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

// namedResultAssignments returns every value assigned to fn's named error
// result. A Work method with unnamed results has none, and the defer that
// assigns one is reached through a FuncLit — which topLevelReturns
// deliberately skips — so this walk descends into literals rather than
// stopping at them.
func namedResultAssignments(fn *ast.FuncDecl) []ast.Expr {
	if fn.Type.Results == nil {
		return nil
	}
	named := map[string]bool{}
	for _, result := range fn.Type.Results.List {
		for _, name := range result.Names {
			if name.Name != "_" {
				named[name.Name] = true
			}
		}
	}
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
			for _, lhs := range stmt.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && named[ident.Name] {
					assigned = append(assigned, stmt.Rhs[0])
					break
				}
			}
			return true
		}
		for i, lhs := range stmt.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || !named[ident.Name] || i >= len(stmt.Rhs) {
				continue
			}
			assigned = append(assigned, stmt.Rhs[i])
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
		"another package's function of the same name": {`type worker struct{}
func (w *worker) Work() error { other.logFailure(); return nil }
func logFailure() { slog.Error("x") }`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.Join("planted", "worker.go"), prelude+tc.src, 0)
			if err != nil {
				t.Fatalf("parse the planted worker: %v", err)
			}
			pkg := indexPackageFuncs(fset, []*ast.File{file})["planted"]
			work := pkg.methods["worker"]["Work"]
			if work == nil {
				t.Fatal("the planted worker declares no Work method the index can see")
			}
			if got := errorLogsAndReturnsNil(work, pkg); got != tc.want {
				t.Errorf("the census reports a log in %q; want %q", got, tc.want)
			}
		})
	}
}
