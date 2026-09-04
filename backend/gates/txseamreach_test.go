// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// Following a call one hop further than the name it is spelled with.
//
// The prohibition next door reads a tx-borrowing body and matches the calls in
// it against the acquirers it knows. That judges the call it can SEE. One hop
// of indirection walked straight past it:
//
//	// inside WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { … })
//	decorateRows(ctx, e.names, rows)        // an unregistered name: passed
//
//	func decorateRows(...) { labelDerivationRows(...) }   // registered, never reached
//
// The wrapper takes no `pgx.Tx`, so it is not a borrowing body of its own and
// nothing judged it. The deadlock is the same one either way: a second
// connection taken while the outer transaction holds the first, and a pool of
// 16 wedges the API under a handful of concurrent requests.
//
// So the question the gate asks is no longer "is this call an acquirer" but
// "does any chain from here REACH one". That answers the second edge for free:
// an acquirer nobody registered is still found, as long as it is reached from
// the package it was written in.
//
// # What is derived and what is still named
//
// The ROOTS stay named, because "this call takes a connection from the pool"
// is not a property this tree can prove about `pgxpool.Pool.Acquire`. Those
// are the library primitives, and they do not change.
//
// Everything above them is derived. `activeColumns` is the worked example: it
// was on the hand-maintained list with the note "opens a transaction of its own
// two calls down", which is exactly the reasoning a walk does not need a human
// for.
//
// # Package-local, and why that is the honest boundary
//
// A call is resolved against the functions declared in its OWN directory. That
// is the scope Go itself resolves a bare identifier in, and it needs no type
// information — which matters, because resolving `x.Foo()` to a package would
// need full type checking, and a gate that guessed would report the wrong file
// with total confidence.
//
// The cost is stated rather than hidden, and there are two halves of it. An
// acquirer reached through a call into ANOTHER package is seen only if that
// call's own name is a named root. And a call through a RECEIVER — `s.helper()`
// — is not followed either, because resolving which type's method that is needs
// type information this walk does not have.
//
// So the blind spot narrows from "one hop anywhere" to "one hop through a
// receiver or across a package boundary". It does not close. The tests below
// pin both limits, so they are tested facts rather than comments somebody hopes
// are still true.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// declaredFunc is one named function or method, with the pgx spelling of the
// file it was declared in — needed to tell a savepoint on a borrowed
// transaction from a reach for a second connection, at every depth.
type declaredFunc struct {
	decl    *ast.FuncDecl
	pgxName string
}

// funcIndex maps directory → function name → the declarations with that name.
//
// Keyed by directory because that is a Go package. It holds only RECEIVER-LESS
// functions, because a bare identifier is the only call this walk follows and a
// bare identifier cannot reach a method: Go requires `s.StageSemantic(…)` for
// one, never `StageSemantic(…)`. Indexing methods too is not a wider net, it is
// a wrong one — `deals` has both a `StageSemantic` string type and a
// `(*Store).StageSemantic` that opens a transaction, so the conversion
// `StageSemantic(*in.Semantic)` was read as the method and reported as a
// deadlock. Nothing in that sentence involves a connection.
type funcIndex map[string]map[string][]declaredFunc

// indexedFunctionFloor is the fail-short guard on the index.
//
// The walk reports a violation only when it FOLLOWS a call, so an index that
// read a smaller tree than it thinks finds nothing and the gate reports PASS
// having checked one hop of nothing. A count is the cheapest thing that fails
// when that happens. It moves down freely — the floor is well under the real
// figure — and is here to catch a walk reading no tree, not to pin a number.
const indexedFunctionFloor = 2000

// indexPackageFunctions reads every package-level function in the roots, once.
//
// A plain walk rather than a gatekit.Scope, because Scope proves that the
// obligated code lies inside the roots and this is not obligated code — it is
// the lookup table the judgement uses. Given a Subject matching everything, that
// proof reads every file in the tree as a site the gate must judge and fails on
// the ones outside. The obligation is still Scope-proved next door, where the
// judging happens.
func indexPackageFunctions(t *testing.T, roots []string) funcIndex {
	t.Helper()
	tree := moduleRoot(t)
	idx := funcIndex{}
	fset := token.NewFileSet()
	counted := 0
	for _, root := range roots {
		err := filepath.WalkDir(filepath.Join(tree, root),
			func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") ||
					strings.HasSuffix(p, "_test.go") {
					return err
				}
				file, parseErr := parser.ParseFile(fset, p, nil, 0)
				if parseErr != nil {
					return parseErr
				}
				rel, relErr := filepath.Rel(tree, p)
				if relErr != nil {
					return relErr
				}
				counted += idx.add(filepath.ToSlash(filepath.Dir(rel)), file)
				return nil
			})
		if err != nil {
			t.Fatalf("indexing %s for the reach walk: %v", root, err)
		}
	}
	if counted < indexedFunctionFloor {
		t.Fatalf("the reach index holds %d functions, below the %d floor — it is reading "+
			"a smaller tree than these roots hold, and a walk that follows nothing "+
			"reports PASS having checked one hop of nothing", counted, indexedFunctionFloor)
	}
	return idx
}

// add records one file's package-level functions, answering how many.
func (idx funcIndex) add(dir string, file *ast.File) int {
	pgxName, _ := pgxLocalName(file)
	added := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Recv != nil {
			continue
		}
		if idx[dir] == nil {
			idx[dir] = map[string][]declaredFunc{}
		}
		idx[dir][fn.Name.Name] = append(idx[dir][fn.Name.Name],
			declaredFunc{decl: fn, pgxName: pgxName})
		added++
	}
	return added
}

// reachesAcquirer answers whether any chain from the named function takes a
// connection, and the chain that gets there.
//
// `seen` is the cycle guard and it is also the memo: a name already on the
// stack answers false, which is right — a cycle that reached an acquirer would
// have reported it on the way in.
func (idx funcIndex) reachesAcquirer(dir, name string, seen map[string]bool) ([]string, bool) {
	key := dir + "." + name
	if seen[key] {
		return nil, false
	}
	seen[key] = true
	for _, fn := range idx[dir][name] {
		body := txBorrowing{name: name, params: fn.decl.Type.Params, body: fn.decl.Body, pgxName: fn.pgxName}
		if found := body.acquires(); len(found) > 0 {
			return []string{name, found[0]}, true
		}
		for _, called := range body.calledNames() {
			if chain, yes := idx.reachesAcquirer(dir, called, seen); yes {
				return append([]string{name}, chain...), true
			}
		}
	}
	return nil, false
}

// calledNames answers the name of every call in this body that Go itself
// resolves package-locally: a BARE IDENTIFIER, and nothing else.
//
// Selector calls are deliberately excluded, and this is the load-bearing
// decision in the file. Following `x.Foo()` by its method name alone is not
// approximation, it is a different question: `privacy` holds both
// `e.blob.Delete(ctx, key)` — an object-store delete that touches no pool —
// and `PolicyStore.Delete`, which opens a transaction. Matching on the name
// reports the first as the second. Measured, not feared: an earlier draft of
// this walk did exactly that and produced seventeen findings, every one of
// them false, in the tree as it stands.
//
// A gate that cries wolf seventeen times is worse than the hop it was closing,
// because the next person to see it red deletes it. Resolving a receiver needs
// full type information, so the honest line is the one Go draws itself.
func (b txBorrowing) calledNames() []string {
	bound := locallyBound(b.params, b.body)
	var names []string
	ast.Inspect(b.body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			// A tx-taking literal is judged as a body of its own, exactly as
			// acquires() leaves it alone.
			return !takesPgxTx(lit.Type.Params, b.pgxName)
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if fn, ok := call.Fun.(*ast.Ident); ok && !bound[fn.Name] {
			names = append(names, fn.Name)
		}
		return true
	})
	return names
}

// locallyBound collects the names this body binds itself: parameters, results,
// and anything declared inside it.
//
// A call through one of them is a call to a VALUE, and the value is whatever the
// caller passed — `func run(step func() error) { step() }` calls `step`, which
// is not the package-level `step` the index would find. Following the name would
// report a deadlock in a function that does not have one, and this gate's whole
// case for existing is that it does not do that.
//
// Over-collecting is the safe direction here. A name bound anywhere in the body
// is treated as bound throughout it, which can lose a real call whose name a
// local shadows later — a miss, where the alternative is a confident false
// accusation. Block scoping would need the type information the walk
// deliberately does without.
func locallyBound(params *ast.FieldList, body *ast.BlockStmt) map[string]bool {
	bound := map[string]bool{}
	add := func(exprs []ast.Expr) {
		for _, e := range exprs {
			if id, ok := e.(*ast.Ident); ok && id.Name != "_" {
				bound[id.Name] = true
			}
		}
	}
	if params != nil {
		for _, field := range params.List {
			for _, name := range field.Names {
				bound[name.Name] = true
			}
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE {
				add(node.Lhs)
			}
		case *ast.ValueSpec:
			for _, name := range node.Names {
				bound[name.Name] = true
			}
		case *ast.RangeStmt:
			add([]ast.Expr{node.Key, node.Value})
		case *ast.FuncLit:
			if node.Type.Params != nil {
				for _, field := range node.Type.Params.List {
					for _, name := range field.Names {
						bound[name.Name] = true
					}
				}
			}
		}
		return true
	})
	return bound
}

// moduleRoot answers the directory holding go.mod, walking up from wherever the
// test binary was started.
//
// Not a relative constant. `..` is right when the package directory is the
// working directory and wrong under a composed workspace, and the failure is a
// walk over a directory that does not exist — which is at least loud. Resolving
// it the way gatekit resolves its own sweep universe keeps the index and the
// judgement reading one tree.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving the working directory to find the module root: %v", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s, so the reach index has no tree to read", dir)
		}
		dir = parent
	}
}

// Guarding the widened walk. Each case below is a shape the gate had to start
// seeing, or a false positive an earlier draft produced and must not again.

// fixtureIndex builds a one-package index out of fixture sources, so a case can
// state a call chain without writing files into the tree.
func fixtureIndex(t *testing.T, sources ...string) funcIndex {
	t.Helper()
	idx := funcIndex{}
	for _, src := range sources {
		idx.add("fixture", parseGateFixture(t, src))
	}
	return idx
}

// assertReaches runs the reach walk over a fixture package and states the whole
// chain, so a case proves WHICH route was followed rather than that something
// somewhere failed.
func assertReaches(t *testing.T, idx funcIndex, from string, wantChain ...string) {
	t.Helper()
	chain, reaches := idx.reachesAcquirer("fixture", from, map[string]bool{})
	if len(wantChain) == 0 {
		if reaches {
			t.Fatalf("the walk reached an acquirer from %q via %v, and must not", from, chain)
		}
		return
	}
	if !reaches {
		t.Fatalf("the walk found no acquirer from %q, want %v", from, wantChain)
	}
	if strings.Join(chain, " → ") != strings.Join(wantChain, " → ") {
		t.Fatalf("the walk reached an acquirer from %q via %v, want %v", from, chain, wantChain)
	}
}

// The reported defect: a wrapper takes no pgx.Tx, so nothing judged it as a
// borrowing body, and the acquirer it called was one hop out of sight.
func TestTheGateSeesAnAcquirerOneCallAway(t *testing.T) {
	t.Parallel()
	// INVENTED names, not the ones the reported defect used. The wrapper it
	// was found in called `labelDerivationRows`, and #4116 has since put that
	// name in connectionAcquirers — so a fixture borrowing it stopped testing
	// the REACH and started testing the direct match, one hop earlier. A
	// fixture that names a real acquirer is coupled to a registry it has no
	// reason to track.
	const wrapper = fixtureImports + `
func decorateRows(ctx context.Context, names Names, rows []map[string]any) {
	stampRowLabels(ctx, names, "deal", rows)
}

func stampRowLabels(ctx context.Context, names Names, kind string, rows []map[string]any) {
	_ = names.pool.Acquire(ctx)
}
`
	assertReaches(t, fixtureIndex(t, wrapper), "decorateRows",
		"decorateRows", "stampRowLabels", "Acquire")
}

// Two hops, and then the same chain reached from the middle: a walk that only
// looked one level down would answer the first and miss nothing here, which is
// why the case asserts the whole route rather than a boolean.
func TestTheGateFollowsAChainPastTheFirstHop(t *testing.T) {
	t.Parallel()
	const chain = fixtureImports + `
func outer(ctx context.Context, s *Store) { middle(ctx, s) }

func middle(ctx context.Context, s *Store) { inner(ctx, s) }

func inner(ctx context.Context, s *Store) { _ = s.db.Tx(ctx, nil) }
`
	idx := fixtureIndex(t, chain)
	assertReaches(t, idx, "outer", "outer", "middle", "inner", "Tx")
	assertReaches(t, idx, "middle", "middle", "inner", "Tx")
}

// Mutual recursion must terminate, and must not be reported as a reach on its
// own. A cycle guard that answered "true" on re-entry would fail every pair of
// functions that call each other.
func TestTheReachWalkTerminatesOnACycleAndClaimsNothing(t *testing.T) {
	t.Parallel()
	const loop = fixtureImports + `
func ping(ctx context.Context) { pong(ctx) }

func pong(ctx context.Context) { ping(ctx) }
`
	assertReaches(t, fixtureIndex(t, loop), "ping")
}

// A CONVERSION is not a call. `deals` holds both a StageSemantic string type
// and a (*Store).StageSemantic that opens a transaction, and an earlier draft
// of this walk read `StageSemantic(*in.Semantic)` as the method and reported a
// deadlock in a switch statement. The index holds package-level functions only,
// which is what a bare identifier can actually reach.
func TestTheReachWalkIsNotFooledByAConversionSharingAMethodName(t *testing.T) {
	t.Parallel()
	const shadowed = fixtureImports + `
type StageSemantic string

func committedWinProbability(in UpdateStageInput) *int {
	switch StageSemantic(*in.Semantic) {
	case SemanticWon:
		return nil
	}
	return in.WinProbability
}

func (s *Store) StageSemantic(ctx context.Context, id ids.UUID) error {
	return s.db.Tx(ctx, nil)
}
`
	assertReaches(t, fixtureIndex(t, shadowed), "committedWinProbability")
}

// The stated limit, pinned so it stays a fact. A receiver call is not followed,
// because resolving which type's method it names needs type information this
// walk does not have — and guessing by name reported an object-store delete as
// a pool acquire.
func TestTheReachWalkStopsAtAReceiverCall(t *testing.T) {
	t.Parallel()
	const throughReceiver = fixtureImports + `
func eraseAttachments(ctx context.Context, e *eraser) error {
	return e.blob.Delete(ctx, "key")
}

func (s *PolicyStore) Delete(ctx context.Context, id ids.UUID) error {
	return s.db.Tx(ctx, nil)
}
`
	assertReaches(t, fixtureIndex(t, throughReceiver), "eraseAttachments")
}

// The other stated limit: a call into another package is not followed, because
// this index is keyed by directory and holds one package per key.
func TestTheReachWalkStopsAtThePackageEdge(t *testing.T) {
	t.Parallel()
	const caller = fixtureImports + `
func decorateRows(ctx context.Context) { elsewhere.Label(ctx) }
`
	assertReaches(t, fixtureIndex(t, caller), "decorateRows")
}

// A call through a PARAMETER is a call to whatever the caller passed, not to a
// package-level function that happens to share the name. Following the name
// would accuse a function that does not have the defect — and a gate that does
// that once gets deleted the next time it is red.
func TestTheReachWalkDoesNotFollowACallThroughAParameter(t *testing.T) {
	t.Parallel()
	const shadowed = fixtureImports + `
func run(ctx context.Context, step func(context.Context) error) error {
	return step(ctx)
}

func step(ctx context.Context, s *Store) error { return s.db.Tx(ctx, nil) }
`
	assertReaches(t, fixtureIndex(t, shadowed), "run")
}

// The same for a local function value. `handler := func() {…}` then `handler()`
// is the shape a dispatch table takes, and the name it binds is ordinary enough
// to collide with something real.
func TestTheReachWalkDoesNotFollowACallThroughALocalValue(t *testing.T) {
	t.Parallel()
	const shadowed = fixtureImports + `
func dispatch(ctx context.Context) error {
	notify := func() error { return nil }
	return notify()
}

func notify(ctx context.Context, s *Store) error { return s.db.Tx(ctx, nil) }
`
	assertReaches(t, fixtureIndex(t, shadowed), "dispatch")
}

// And the exclusion must not swallow the real case: a package-level call whose
// name nothing binds is still followed. Without this the two cases above are
// satisfied by a walk that follows nothing at all.
func TestTheReachWalkStillFollowsAnUnboundName(t *testing.T) {
	t.Parallel()
	const genuine = fixtureImports + `
func outer(ctx context.Context, s *Store) error {
	local := "not a function"
	_ = local
	return inner(ctx, s)
}

func inner(ctx context.Context, s *Store) error { return s.db.Tx(ctx, nil) }
`
	assertReaches(t, fixtureIndex(t, genuine), "outer", "outer", "inner", "Tx")
}
