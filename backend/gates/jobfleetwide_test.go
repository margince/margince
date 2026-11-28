// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// A FleetWide declaration is a promise: this job enumerates and enqueues, or
// runs one pass over the installation, and writes no tenant row inline
// (jobs.FleetWide). jobrole_test.go proves every job declares a kind; this gate
// proves the FleetWide half of that declaration is true of the code.
//
// # The allowlist
//
// "Reaches an insert" is interprocedural, since a dispatch travels through
// package helpers, injected inserters and store transactions. So it is not
// checkable on the syntax tree. This gate is a closed allowlist instead. A
// FleetWide Work must call dispatchOnce, dispatchOne or runInstallationPass.
//
// A direct River insert is not in the set. The helpers are where a child's
// insert options are built, which stamps the sweep tag and reads the declared
// queue and attempt cap. A dispatcher that inserts around them enqueues a child
// that neither sweep gauge sees. A fourth spelling is a finding, so adding one
// takes a reviewer.
//
// # Kinds that do their own pass
//
// Some FleetWide kinds do the work in their own row and have nothing to fan
// out to. Such a kind is named in fleetWideDoesItsOwnPass with the reason, and
// only the fan-out arm is waived. It is still held to the no-write arm below.
//
// # Reads are fine; writes are not
//
// Several dispatchers read tenant tables to find the units that have work, and
// jobs.FleetWide permits that. The second arm prohibits a SQL write statement
// in the dispatcher's own body, or in a helper method on its worker that Work
// calls. No dispatcher writes inline today. The first one that does is a
// finding and not a precedent.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// fleetWideDispatcherFloor guards against a vacuous pass. This gate resolves an
// association across two types — args declare FleetWide, a separate worker owns
// a Work over *river.Job[Args] — so a walker that matched no files, or a
// resolution step that silently stopped associating the two, would inspect
// nothing and report green. The tree declares 23 FleetWide args types
// today, every one of them resolving to a worker; the floor sits at 20 so
// retiring a pass or two does not drag the gate along, while any wholesale
// failure to resolve still trips it.
const fleetWideDispatcherFloor = 20

// fanOutHelpers are the ways a FleetWide job reaches the installation:
// dispatchOnce and dispatchOne queue a child, and runInstallationPass runs the
// pass in this process.
var fanOutHelpers = []string{
	"dispatchOnce", "dispatchOne", "runInstallationPass",
}

// fleetWideDispatcher is one resolved args→worker→Work association and what
// the gate found in it.
type fleetWideDispatcher struct {
	args    string
	worker  string
	pos     token.Position
	fansOut bool
	writes  []string
}

// fleetWideArgsTypes returns every args type declaring the FleetWide marker.
func fleetWideArgsTypes(files []*ast.File) map[string]bool {
	marked := map[string]bool{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "FleetWide" {
				continue
			}
			if recv := receiverTypeName(fn); recv != "" {
				marked[recv] = true
			}
		}
	}
	return marked
}

// workerByArgs maps an args type name to the worker type that works it, read
// off the Work method's own *river.Job[Args] parameter. This is the
// association a method-name index cannot make: the marker is on the args, the
// behaviour is on the worker, and that generic parameter is the only thing
// joining them — a worker embeds nothing now that jobs.Govern supplies River's
// option methods, so the Work signature is the whole of what names its kind.
func workerByArgs(files []*ast.File) map[string]string {
	byArgs := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Work" {
				continue
			}
			recv := receiverTypeName(fn)
			if recv == "" {
				continue
			}
			if args := riverJobParamType(fn); args != "" {
				byArgs[args] = recv
			}
		}
	}
	return byArgs
}

// riverJobParamType reads the T out of a Work method's *river.Job[T]
// parameter, or "" when it has none — which is what a same-named method on
// something that is not a River worker looks like.
func riverJobParamType(fn *ast.FuncDecl) string {
	if fn.Type.Params == nil {
		return ""
	}
	for _, param := range fn.Type.Params.List {
		star, ok := param.Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		index, ok := star.X.(*ast.IndexExpr)
		if !ok {
			continue
		}
		sel, ok := index.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Job" {
			continue
		}
		if args, ok := index.Index.(*ast.Ident); ok {
			return args.Name
		}
	}
	return ""
}

// methodDeclsByType indexes every method body by its receiver type, so the gate
// can follow a Work body one hop into a helper on the same worker.
func methodDeclsByType(files []*ast.File) map[string]map[string]*ast.FuncDecl {
	byType := map[string]map[string]*ast.FuncDecl{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			recv := receiverTypeName(fn)
			if recv == "" {
				continue
			}
			if byType[recv] == nil {
				byType[recv] = map[string]*ast.FuncDecl{}
			}
			byType[recv][fn.Name.Name] = fn
		}
	}
	return byType
}

// dispatchBodies is Work's body plus the bodies of the helpers on the SAME
// worker type that it calls. One hop, and only through the receiver, because
// that is the extent to which "what this dispatcher itself does" is resolvable
// without type information — embedReindexWorker.fanOut is why the hop exists at
// all, and a deeper walk would be a reachability analysis this gate is
// deliberately not.
func dispatchBodies(work *ast.FuncDecl, methods map[string]*ast.FuncDecl) []*ast.BlockStmt {
	bodies := []*ast.BlockStmt{work.Body}
	if work.Recv == nil || len(work.Recv.List[0].Names) == 0 {
		// An unnamed receiver can call no helper on itself.
		return bodies
	}
	recv := work.Recv.List[0].Names[0].Name
	seen := map[string]bool{work.Name.Name: true}
	ast.Inspect(work.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); !ok || id.Name != recv {
			return true
		}
		helper, ok := methods[sel.Sel.Name]
		if !ok || seen[sel.Sel.Name] {
			return true
		}
		seen[sel.Sel.Name] = true
		bodies = append(bodies, helper.Body)
		return true
	})
	return bodies
}

// callNames returns the plain function names called anywhere in bodies. Only
// plain idents, because every sanctioned fan-out is a package-level helper
// called by its bare name: a selector would be someone else's method, which is
// exactly what must not count.
func callNames(bodies []*ast.BlockStmt) map[string]bool {
	called := map[string]bool{}
	for _, body := range bodies {
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if fun, ok := call.Fun.(*ast.Ident); ok {
				called[fun.Name] = true
			}
			return true
		})
	}
	return called
}

// fansOut reports whether bodies contain a call from the sanctioned fan-out set.
func fansOut(bodies []*ast.BlockStmt) bool {
	called := callNames(bodies)
	for _, helper := range fanOutHelpers {
		if called[helper] {
			return true
		}
	}
	return false
}

// tenantWriteIn names the write verb of a SQL write statement, or "" when the
// literal is not one. Only string LITERALS are inspected: these files discuss
// inserts and updates in prose constantly, and a gate that read comments would
// fire on every one of them.
func tenantWriteIn(literal string) string {
	sql := strings.ToUpper(strings.Join(strings.Fields(literal), " "))
	// A locking read and an upsert's conflict clause both spell UPDATE without
	// being an UPDATE statement; a genuine upsert still trips INSERT INTO.
	sql = strings.ReplaceAll(sql, "FOR UPDATE", "")
	sql = strings.ReplaceAll(sql, "DO UPDATE", "")
	switch {
	case strings.Contains(sql, "INSERT INTO "):
		return "INSERT"
	case strings.Contains(sql, "DELETE FROM "):
		return "DELETE"
	case strings.Contains(sql, "TRUNCATE "):
		return "TRUNCATE"
	case strings.Contains(sql, "UPDATE ") && strings.Contains(sql, " SET "):
		// SET is what separates an UPDATE statement from the word "update" in
		// an error message.
		return "UPDATE"
	}
	return ""
}

// writesIn returns the write verbs of every SQL write statement in bodies.
func writesIn(bodies []*ast.BlockStmt) []string {
	var verbs []string
	for _, body := range bodies {
		for _, statement := range gatekit.SQLStatementsOf(body) {
			if verb := tenantWriteIn(statement); verb != "" {
				verbs = append(verbs, verb)
			}
		}
	}
	return verbs
}

// analyzeFleetWide resolves every FleetWide args type to its worker's Work
// method and reports what that dispatcher does. orphans are the marked args
// types no worker works — a dispatcher nothing runs.
func analyzeFleetWide(fset *token.FileSet, files []*ast.File) (dispatchers []fleetWideDispatcher, orphans []string) {
	marked := fleetWideArgsTypes(files)
	workers := workerByArgs(files)
	methods := methodDeclsByType(files)
	for args := range marked {
		worker, ok := workers[args]
		if !ok {
			orphans = append(orphans, args)
			continue
		}
		work, ok := methods[worker]["Work"]
		if !ok {
			orphans = append(orphans, args)
			continue
		}
		bodies := dispatchBodies(work, methods[worker])
		dispatchers = append(dispatchers, fleetWideDispatcher{
			args:    args,
			worker:  worker,
			pos:     fset.Position(work.Pos()),
			fansOut: fansOut(bodies),
			writes:  writesIn(bodies),
		})
	}
	// Map iteration order is randomized, so findings would arrive in a different
	// order every run — a diff between two runs of a red gate would be noise
	// rather than signal. Report in source order, as the neighbouring gates do.
	sort.Slice(dispatchers, func(i, j int) bool {
		if dispatchers[i].pos.Filename != dispatchers[j].pos.Filename {
			return dispatchers[i].pos.Filename < dispatchers[j].pos.Filename
		}
		return dispatchers[i].pos.Line < dispatchers[j].pos.Line
	})
	sort.Strings(orphans)
	return dispatchers, orphans
}

// fleetWideArgsType is the vocabulary this gate's waivers are drawn from: the
// name of a job args type that declares FleetWide().
type fleetWideArgsType string

// fleetWideDoesItsOwnPass names the FleetWide kinds that do the work in their
// own row instead of fanning it out, each with the reason it has no fleet to
// fan out to. Ratified, not discovered: a kind arrives here by a reviewed edit,
// and gatekit reports one that outlives its subject.
var fleetWideDoesItsOwnPass = gatekit.Waive(map[fleetWideArgsType]string{
	"DataTaskRunArgs":       "one installation-wide correction ledger; the executor serializes batches with a database advisory lock and binds the installation through the canonical resolver",
	"DataTaskReconcileArgs": "one installation-wide correction ledger; seed and enqueue are conflict-safe and never fan out the same corpus",
	"EmbedReindexArgs":      "phase D un-scoped the embedding corpus (ADR-0091 §8), so the per-workspace children all rebuilt the SAME rows and all but the first found every one already fresh; one pass rebuilds it",
})

// checkFleetWideDispatchers runs the gate over one directory.
func checkFleetWideDispatchers(t *testing.T, dir string) {
	t.Helper()
	fset, files := parseGoFilesUnder(t, dir)
	dispatchers, orphans := analyzeFleetWide(fset, files)
	for _, args := range orphans {
		t.Errorf("%s declares FleetWide() but no worker has a Work method over *river.Job[%s]: a dispatcher nothing runs enqueues nothing, and every tenant it was to fan out to is silently never swept.", args, args)
	}
	for _, d := range dispatchers {
		if fleetWideDoesItsOwnPass.Waived(t, fleetWideArgsType(d.args)) {
			if d.fansOut {
				t.Errorf("%s:%d: %s is waived as doing its own pass but its Work DOES fan out — delete the waiver, it now tells the next reader the opposite of the code.",
					d.pos.Filename, d.pos.Line, d.args)
			}
			continue
		}
		if !d.fansOut {
			t.Errorf("%s:%d: %s works FleetWide args %s but never reaches the fleet. It must do so through dispatchOnce or dispatchOne — which build the child's insert options, so a direct River insert around them loses the sweep tag and the declared attempt cap — or through runInstallationPass, which runs the pass over the installation in this process (ADR-0103).",
				d.pos.Filename, d.pos.Line, d.worker, d.args)
		}
		for _, verb := range d.writes {
			t.Errorf("%s:%d: %s works FleetWide args %s and issues a tenant write (%s). A dispatcher may read to discover work; the write belongs to the workspace job, which succeeds or fails as its own row (jobs.FleetWide). Move the write into the workspace worker.",
				d.pos.Filename, d.pos.Line, d.worker, d.args, verb)
		}
	}
	fleetWideDoesItsOwnPass.AssertAllMatched(t)
	if len(dispatchers) < fleetWideDispatcherFloor {
		t.Fatalf("resolved only %d FleetWide dispatchers, expected at least %d — the args→worker association matched almost nothing and this gate would pass vacuously", len(dispatchers), fleetWideDispatcherFloor)
	}
}

// TestEveryFleetWideJobOnlyDispatches is the gate. See the file comment for the
// allowlist and what it deliberately does not prove.
func TestEveryFleetWideJobOnlyDispatches(t *testing.T) {
	t.Parallel()
	checkFleetWideDispatchers(t, filepath.Join("internal", "compose"))
}
