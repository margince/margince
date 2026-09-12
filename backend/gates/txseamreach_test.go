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
	// recv, recvType and heldTx carry the receiver-held shape into the walk, so
	// a method reached through a chain is judged with the same knowledge as one
	// reached directly — otherwise a read of its OWN borrowed `c.tx` would be
	// reported as a second connection the moment something called it.
	recv     string
	recvType string
	heldTx   []string
	// declares names the methods recvType declares itself. It rides along for
	// the same reason heldTx does: attachHeldTx's promoted-method exemption
	// consults it, and a walk that left it nil would answer a different
	// question about the same body than the direct gate next door does.
	declares map[string]bool
}

// funcIndex maps directory → function name → the declarations with that name.
//
// Keyed by directory because that is a Go package. Receiver-less functions are
// keyed by their bare name, and a METHOD by `Type.Method` — a key no bare-name
// lookup can reach, which is what keeps the two vocabularies from colliding.
//
// It held only receiver-less functions, because a bare identifier is the only call this walk follows and a
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
	holders, declared := txHoldingReceivers(t, roots)
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
				dir := filepath.ToSlash(filepath.Dir(rel))
				counted += idx.add(dir, file, holders[dir], declared[dir])
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

// add records one file's package-level functions and the methods of its
// tx-holding receivers, answering how many.
func (idx funcIndex) add(dir string, file *ast.File,
	holders map[string][]string, declared map[string]map[string]bool,
) int {
	pgxName, _ := pgxLocalName(file)
	added := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		entry := declaredFunc{decl: fn, pgxName: pgxName}
		key := fn.Name.Name
		if fn.Recv != nil {
			// Only the methods of a receiver that HOLDS a transaction. Every
			// other method is out of the walk's reach for the reason
			// calledNames gives at length: a selector call cannot be resolved
			// to a type without type information, so following one by name is
			// a different question than the one being asked. A held-tx
			// receiver is the exception because the type IS known — it is the
			// caller's own — so `c.helper()` resolves to exactly one entry.
			recvType := receiverTypeName(fn)
			fields, held := holders[recvType]
			if !held {
				continue
			}
			entry.recvType, entry.heldTx = recvType, fields
			entry.declares = declared[recvType]
			if names := fn.Recv.List[0].Names; len(names) > 0 && names[0].Name != "_" {
				entry.recv = names[0].Name
			}
			key = recvType + "." + fn.Name.Name
		}
		if idx[dir] == nil {
			idx[dir] = map[string][]declaredFunc{}
		}
		idx[dir][key] = append(idx[dir][key], entry)
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
		body := txBorrowing{
			name: name, params: fn.decl.Type.Params, body: fn.decl.Body, pgxName: fn.pgxName,
			recv: fn.recv, recvType: fn.recvType, heldTx: fn.heldTx, declares: fn.declares,
		}
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
// because the next contact to see it red deletes it. Resolving a receiver needs
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
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if !bound[fn.Name] {
				names = append(names, fn.Name)
			}
		case *ast.SelectorExpr:
			// The one selector this walk resolves: a call on the receiver of a
			// method whose receiver holds the transaction. It is not the
			// excluded case above — that one cannot tell `e.blob.Delete` from
			// `PolicyStore.Delete` because nothing says what `e.blob` is. Here
			// the receiver's TYPE is the method's own, so the name resolves to
			// exactly one entry, keyed `Type.Method`. A field that happens to
			// hold a function resolves to no entry and costs nothing.
			// `!bound[b.recv]` for the reason the bare-identifier branch
			// above carries the same guard: a name this body binds itself is a
			// VALUE, and `c := other` makes `c.mode()` a call on somebody
			// else's object. Following it would walk into this type's method
			// and report a deadlock in a body that has none, which is the one
			// thing this walk's case for existing rests on not doing.
			//
			// locallyBound over-collects — a name bound ANYWHERE in the body is
			// bound throughout it — so a method that writes `for _, c := range …`
			// once loses the follow for every `c.method()` in it. That is a
			// MISS, and it is the direction chosen on purpose here as it is
			// there: the alternative to losing a call is accusing a body that
			// does not have the defect, and a gate that cries wolf gets deleted.
			// Block scoping would need the type information this walk
			// deliberately does without.
			if base, ok := fn.X.(*ast.Ident); ok && b.recvType != "" &&
				base.Name == b.recv && !bound[b.recv] {
				names = append(names, b.recvType+"."+fn.Sel.Name)
			}
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
	holders := map[string]map[string][]string{}
	declared := map[string]map[string]map[string]bool{}
	parsed := make([]*ast.File, 0, len(sources))
	for _, src := range sources {
		file := parseGateFixture(t, src)
		parsed = append(parsed, file)
		// Both passes first and over ALL the sources, exactly as the live index
		// does it: a type declared in one fixture file and given verbs in
		// another is the arrangement a per-file answer would miss.
		addTxHolders(holders, "fixture", file)
		addTxDeclaredMethods(declared, "fixture", file)
	}
	for _, file := range parsed {
		idx.add("fixture", file, holders["fixture"], declared["fixture"])
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

// A verb on a tx-holding port reaches the pool through a SIBLING METHOD.
//
// This is the second half of what the parameter rule could not see. The verb
// itself takes no connection; the helper beside it does, and both run inside
// the caller's transaction. Following it is safe here for the reason the bare
// selector rule is not followed anywhere else: the receiver's type is the
// method's own, so `c.mode()` resolves to one entry and not to every `mode`
// in the package.
func TestTheWalkFollowsASiblingMethodOfATxHoldingReceiver(t *testing.T) {
	t.Parallel()
	const port = `package compose

import "github.com/jackc/pgx/v5"

type core struct {
	tx   pgx.Tx
	pool *pgxpool.Pool
}

func (c core) File(ctx context.Context) error {
	if err := c.mode(ctx); err != nil {
		return err
	}
	return nil
}

func (c core) mode(ctx context.Context) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return nil
}
`
	assertReaches(t, fixtureIndex(t, port), "core.File", "core.File", "core.mode", "Acquire")
}

// And the sibling's own read of the HELD transaction is left alone. Without
// this the case above would pass against a walk that flagged every method it
// followed, which would make the widening a source of false findings rather
// than a gate.
func TestTheWalkLeavesASiblingReadingTheHeldTransactionAlone(t *testing.T) {
	t.Parallel()
	const port = `package compose

import "github.com/jackc/pgx/v5"

type core struct {
	tx pgx.Tx
}

func (c core) File(ctx context.Context) error {
	return c.mode(ctx)
}

func (c core) mode(ctx context.Context) error {
	_, err := c.tx.Begin(ctx)
	return err
}
`
	assertReaches(t, fixtureIndex(t, port), "core.File")
}

// An EMBEDDING receiver's promoted `Begin` is the borrowed transaction's own
// savepoint; a `Begin` the type declared itself is not, and in Go the declared
// one is what `c.Begin(…)` resolves to.
//
// The walk needs the type's declared-method census to tell those apart, and it
// is here because the walk was built without it: it read every `c.Begin(…)` on
// an embedding receiver as the promoted savepoint and waved this body through,
// while the direct gate beside it — judging the same body with the census in
// hand — reported it. Two answers to one question, and the gate's own fixtures
// would not have shown the disagreement.
func TestTheWalkReadsADeclaredBeginOnAnEmbeddingReceiverAsAnAcquire(t *testing.T) {
	t.Parallel()
	const declaredBegin = `package compose

import "github.com/jackc/pgx/v5"

type core struct {
	pgx.Tx
	pool *pgxpool.Pool
}

func (c core) File(ctx context.Context) error {
	return c.open(ctx)
}

func (c core) open(ctx context.Context) error {
	_, err := c.Begin(ctx)
	return err
}

// Declared, so it shadows the embedded transaction's promoted Begin — and it
// takes a connection of its own.
func (c core) Begin(ctx context.Context) (pgx.Tx, error) {
	return c.pool.Begin(ctx)
}
`
	assertReaches(t, fixtureIndex(t, declaredBegin), "core.File", "core.File", "core.open", "Begin")
}

// A method on a receiver that holds NO transaction is not followed at all —
// the excluded selector case, unchanged. `e.blob.Delete` and
// `PolicyStore.Delete` are still two questions this walk cannot tell apart,
// and the widening does not pretend otherwise.
func TestTheWalkStillDoesNotFollowAnOrdinaryReceiversMethod(t *testing.T) {
	t.Parallel()
	const ordinary = `package compose

import "github.com/jackc/pgx/v5"

type store struct {
	pool *pgxpool.Pool
}

func (s store) WriteTx(ctx context.Context, tx pgx.Tx) error {
	return s.mode(ctx)
}

func (s store) mode(ctx context.Context) error {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return nil
}
`
	assertReaches(t, fixtureIndex(t, ordinary), "WriteTx")
}

// A local that SHADOWS the receiver's name is not the receiver.
//
// `c := other; c.mode()` is a call on somebody else's object, and following it
// into this type's method reports a deadlock in a body that has none. The
// bare-identifier branch has carried that guard since the walk was written; the
// receiver path needed the same one.
func TestTheWalkDoesNotFollowAShadowedReceiverName(t *testing.T) {
	t.Parallel()
	const shadowed = `package compose

import "github.com/jackc/pgx/v5"

type core struct {
	tx   pgx.Tx
	pool *pgxpool.Pool
}

func (c core) File(ctx context.Context) error {
	c := somebodyElse()
	return c.mode(ctx)
}

func (c core) mode(ctx context.Context) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return nil
}
`
	assertReaches(t, fixtureIndex(t, shadowed), "core.File")
}
