// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Code that runs on a caller's `pgx.Tx` acquires no connection of its own.
//
// The C5 shared-tx seams exist so a caller can commit a sibling module's write
// and its own in ONE transaction. A seam that reaches for the pool while
// standing inside someone else's transaction breaks that in two ways, and
// neither announces itself. The second connection commits separately, so the
// "both or neither" claim the shape is chosen for is quietly false. And if the
// caller's transaction holds a lock the second one waits on, the two block each
// other inside a single goroutine — a deadlock Postgres cannot detect and will
// not break, because it sees two unrelated sessions waiting.
//
// The rule is not new; it is written on the acquirer that breaks it most often.
// The custom-field catalog read documents "callers fetch BEFORE opening their
// write/read transaction (never inside it — a nested pool acquire under load is
// a deadlock shape)". Deriving the obligation from the tree is what makes a
// seam written next month inherit it instead of re-deciding it.
//
// Two things bound what this can prove, and both are why the pool-of-one suite
// in compose/integration exists beside it: the gate catches the class across
// the diff, that suite catches the instance in a database.
//
//   - The acquirers are NAMED rather than inferred, because "does this call
//     reach the pool" is not decidable from syntax. They are the spellings this
//     tree has: the pool itself, the transaction openers (database.DB.Tx, the
//     modules' `s.tx` helper over it, database.With*Tx, a raw Begin), and the
//     field-catalog read, which opens a transaction of its own two calls down.
//     A new spelling would slip past until it is added here.
//   - A call reaches the pool through a package-local helper rather than
//     directly, and the walk does not follow it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// gatekit:fixture the calls this gate reads as taking a connection, and what
// each one does — the vocabulary the walk matches on, not waived costs
//
// connectionAcquirers are the calls that take a connection from the pool,
// keyed by the selector a call site spells. Each is a call a tx-accepting
// function must have made before it was handed the transaction, or delegated
// to a caller that did.
var connectionAcquirers = map[string]string{
	"Acquire":          "takes a connection straight from the pool",
	"Begin":            "opens a transaction on a pool, which takes a connection to hold it",
	"BeginTx":          "opens a transaction on a pool, which takes a connection to hold it",
	"Tx":               "opens a transaction on the bound handle (database.DB.Tx)",
	"tx":               "opens a transaction through the store's own helper over database.DB.Tx",
	"WithWorkspaceTx":  "opens a workspace-bound transaction",
	"WithInfraTx":      "opens an unbound infrastructure transaction",
	"activeColumns":    "reads the custom-field catalog, which opens a transaction of its own",
	"activeColumnsFor": "reads the custom-field catalog, which opens a transaction of its own",
	"ActiveColumns":    "reads the custom-field catalog through the fieldcatalog seam, which opens a transaction of its own",
	// The stores' own catalog reads, which a composite calls by name. They
	// exist precisely so the answer can be fetched above a transaction, so
	// calling one from inside a borrowed transaction reinstates the defect
	// they were introduced to remove.
	"ActivePersonColumns":       "reads the person custom-field catalog, which opens a transaction of its own",
	"ActiveCompanyColumns": "reads the company custom-field catalog, which opens a transaction of its own",
	"ActiveDealColumns":         "reads the deal custom-field catalog, which opens a transaction of its own",
	// The drill-through's display names, resolved through each module's own
	// gated label read — every one of which opens a transaction. It exists to
	// be called ABOVE the report's transaction, exactly like the catalog reads
	// above, so calling it from inside one reinstates the defect.
	"labelDerivationRows": "names the drill-through's rows through the stores' label reads, each of which opens a transaction of its own",
}

func TestATxAcceptingFunctionAcquiresNoConnectionOfItsOwn(t *testing.T) {
	t.Parallel()
	exempt := gatekit.Waive(map[string]string{
		// The seeding tool, which is not a seam: it opens the transaction it
		// then passes down, so there is no caller whose transaction could be
		// borrowed and no second connection to take. The roots stay at the
		// tiers that serve requests rather than widening to tools/, because
		// this gate's subject is the C5 shared-tx seam and a demo fixture that
		// owns its own connection is not one.
	})
	roots := []string{"internal", "cmd"}
	holders, declared := txHoldingReceivers(t, roots)
	scope := gatekit.Scope{
		Roots: roots,
		// A file holding any tx-borrowing body, by either rule. The parameter
		// rule alone excluded a port that keeps the caller's transaction in a
		// FIELD — its methods take no pgx.Tx at all — so `holders` is read
		// here too, and it is built from a walk of its own rather than from
		// this scope: the type and its verbs live in the same file today, and
		// a subject rule that assumed so would report PASS the day somebody
		// moves them apart.
		Subject: func(p string, file *ast.File) bool {
			return len(txBorrowingBodies(file, holders[path.Dir(p)], declared[path.Dir(p)])) > 0
		},
		Exempt: exempt,
	}
	defer exempt.AssertAllMatched(t)

	index := indexPackageFunctions(t, roots)
	for _, parsed := range scope.Files(t) {
		dir := path.Dir(parsed.Path)
		for _, body := range txBorrowingBodies(parsed.File, holders[dir], declared[dir]) {
			for _, found := range body.acquires() {
				t.Errorf("%s: %s runs on a caller's pgx.Tx and then %s (%s) — fetch it before the "+
					"transaction opens and thread the result in, as mergePersonTx and createDealTx do; "+
					"a second connection inside someone else's transaction commits separately and "+
					"deadlocks undetectably against a lock that transaction holds",
					parsed.Path, body.name, connectionAcquirers[found], found)
			}
			// The same prohibition one hop further out. A wrapper takes no
			// pgx.Tx, so nothing above judges it, and the connection it takes
			// deadlocks exactly as the direct one would.
			for _, called := range body.calledNames() {
				if _, direct := connectionAcquirers[called]; direct {
					continue // already reported above, in source order
				}
				if chain, reaches := index.reachesAcquirer(dir, called, map[string]bool{}); reaches {
					t.Errorf("%s: %s runs on a caller's pgx.Tx and calls %s, which takes a "+
						"connection %s — same deadlock as taking it here, one call further "+
						"away from the transaction that will wait on it. Fetch above the "+
						"transaction and thread the result in.",
						parsed.Path, body.name, called, strings.Join(chain, " → "))
				}
			}
		}
	}
}

// txBorrowing is one body that runs on a transaction somebody else opened: a
// named function that takes a pgx.Tx, or the callback literal a transaction
// opener is handed. Both are the same obligation. The literal matters most —
// `WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error { … })` is where this tree
// writes, so a gate that judged only named seams would miss the shape it is
// most likely to meet.
type txBorrowing struct {
	name   string
	params *ast.FieldList
	body   *ast.BlockStmt
	// pgxName is the local name the file imports pgx under, so a seam in a
	// file that aliases the import is judged rather than skipped.
	pgxName string
	// recv and heldTx describe the third shape: a method on a receiver that
	// HOLDS the caller's transaction in a field. `recv` is the receiver's own
	// name in this method (`c` in `func (c extensionCore) …`) and heldTx the
	// fields carrying a pgx.Tx, so a call spelled `c.tx.Query(…)` is read as
	// running on the borrowed transaction rather than as a second connection.
	// Both empty for the parameter and callback shapes. recvType names the
	// receiver's type, which is what lets the reach walk resolve a call on it.
	// A heldTx entry of promotedTx means the type EMBEDS the transaction, so
	// its methods are reached on the receiver itself.
	recv     string
	recvType string
	heldTx   []string
	// declares names the methods the receiver's type declares itself, so a
	// declared name is not mistaken for a promoted one.
	declares map[string]bool
}

// promotedTx is the heldTx entry for an EMBEDDED pgx.Tx, which has no field
// name to spell: its methods are promoted onto the receiver.
const promotedTx = ""

// promotedTxMethods are the acquirer names that belong to pgx.Tx ITSELF, so a
// call on a receiver embedding one is the borrowed handle rather than a method
// the embedding type declared.
//
// Only Begin, because it is the only name in connectionAcquirers that pgx.Tx
// has: the rest are the pool's, the bound handle's, or this tree's own catalog
// reads, and a type embedding a transaction is as free to declare a method
// called Acquire or activeColumns as any other. Read without this, an
// embedding receiver made every acquirer on itself invisible — the one shape
// that looks most like a borrowed handle hiding the defect best.
var promotedTxMethods = map[string]bool{"Begin": true}

// embedsTx reports whether this body's receiver embeds the transaction rather
// than naming a field for it.
func (b txBorrowing) embedsTx() bool {
	for _, field := range b.heldTx {
		if field == promotedTx {
			return true
		}
	}
	return false
}

// txBorrowingBodies answers every tx-borrowing body in one file, outermost
// first. `holders` names the receiver types in this file's PACKAGE that keep a
// caller's transaction in a field, and the fields that carry it.
func txBorrowingBodies(file *ast.File, holders map[string][]string, declared map[string]map[string]bool) []txBorrowing {
	// No early return on a file that does not import pgx. The parameter rule
	// needs the import — a body cannot take a pgx.Tx without naming the package
	// — but a METHOD on a receiver that holds one does not: it reads `c.tx` and
	// mentions pgx nowhere. Returning here left every verb of a port whose type
	// is declared in a SIBLING file invisible, which is the same blindness this
	// rule was widened to remove, one file over. An absent import makes the
	// parameter rule match nothing, which is the right answer for it.
	pgxName, _ := pgxLocalName(file)
	var out []txBorrowing
	add := func(name string, params *ast.FieldList, body *ast.BlockStmt) bool {
		if body == nil || !takesPgxTx(params, pgxName) {
			return false
		}
		out = append(out, txBorrowing{name: name, params: params, body: body, pgxName: pgxName})
		return true
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		// Both rules, not one or the other. A method on a tx-holding receiver
		// that ALSO takes a pgx.Tx borrows two, and recording it under the
		// parameter rule alone left its receiver unknown — so its own `c.tx`
		// read came back as a second connection, and the gate accused a body
		// that was doing the right thing twice over.
		if add(fn.Name.Name, fn.Type.Params, fn.Body) {
			attachHeldTx(&out[len(out)-1], fn, holders, declared)
		} else {
			addHeldTxMethod(&out, fn, holders, declared, pgxName)
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.FuncLit)
			if !ok {
				return true
			}
			// A tx-taking literal is judged as its own body, so the walk does
			// not descend into it twice.
			return !add(fn.Name.Name+"'s transaction callback", lit.Type.Params, lit.Body)
		})
	}
	return out
}

// addHeldTxMethod records a method whose RECEIVER holds the caller's
// transaction, which is the same obligation reached a different way.
//
// It exists because a port can keep the transaction instead of passing it:
// compose/extcore.go's extensionCore does, and every verb on it runs on that
// field. Judged by the parameter rule alone the whole file was invisible, and
// it did not stay hypothetical — the first version of that port read the
// workspace's mode through a second pool acquire inside the caller's open
// transaction, the exact deadlock this gate refuses, and the gate was green.
// Review caught it; the class was still uncaught.
func addHeldTxMethod(out *[]txBorrowing, fn *ast.FuncDecl, holders map[string][]string, declared map[string]map[string]bool, pgxName string) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return
	}
	if _, holds := holders[receiverTypeName(fn)]; !holds {
		return
	}
	body := txBorrowing{
		name:    receiverTypeName(fn) + "." + fn.Name.Name,
		params:  fn.Type.Params,
		body:    fn.Body,
		pgxName: pgxName,
	}
	attachHeldTx(&body, fn, holders, declared)
	*out = append(*out, body)
}

// attachHeldTx records which transaction a body's receiver holds, so a call on
// it reads as the borrowed handle rather than as a second connection.
//
// An unnamed or blank receiver cannot spell `c.tx`, so nothing in the body can
// be reading the held transaction and every acquirer found is a second
// connection. Left with no receiver name rather than skipped.
func attachHeldTx(body *txBorrowing, fn *ast.FuncDecl, holders map[string][]string, declared map[string]map[string]bool) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return
	}
	fields, holds := holders[receiverTypeName(fn)]
	if !holds {
		return
	}
	body.recvType, body.heldTx = receiverTypeName(fn), fields
	body.declares = declared[receiverTypeName(fn)]
	if names := fn.Recv.List[0].Names; len(names) > 0 && names[0].Name != "_" {
		body.recv = names[0].Name
	}
}

// txHoldingReceivers indexes, per package directory, the struct types that keep
// a pgx.Tx in a field and the fields that hold it.
//
// It walks the roots itself rather than reading the gate's own scope, and the
// two reasons are the same reason. The scope is SELECTED by this answer, so
// deriving it from the scope would be circular; and a type declared in one file
// and given its verbs in another is the ordinary arrangement, which a per-file
// answer would report PASS on. A census that can only see the tidy case has
// already failed.
func txHoldingReceivers(t *testing.T, roots []string) (map[string]map[string][]string, map[string]map[string]map[string]bool) {
	t.Helper()
	tree := moduleRoot(t)
	out := map[string]map[string][]string{}
	declared := map[string]map[string]map[string]bool{}
	fset := token.NewFileSet()
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
				addTxHolders(out, dir, file)
				addTxDeclaredMethods(declared, dir, file)
				return nil
			})
		if err != nil {
			t.Fatalf("indexing %s for tx-holding receivers: %v", root, err)
		}
	}
	return out, declared
}

// addTxDeclaredMethods records, per package directory, the method names each
// type declares itself.
//
// It exists for the case a type that EMBEDS pgx.Tx creates by declaring its own
// `Begin`. `c.Begin(…)` resolves to that method, not to the promoted
// transaction's, so exempting it by name waves through an acquire wearing a
// name this walk otherwise reads as a savepoint. A declared name wins over a
// promoted one in Go, and this is what lets the walk say so.
func addTxDeclaredMethods(out map[string]map[string]map[string]bool, dir string, file *ast.File) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil {
			continue
		}
		recv := receiverTypeName(fn)
		if recv == "" {
			continue
		}
		if out[dir] == nil {
			out[dir] = map[string]map[string]bool{}
		}
		if out[dir][recv] == nil {
			out[dir][recv] = map[string]bool{}
		}
		out[dir][recv][fn.Name.Name] = true
	}
}

// addTxHolders records one file's struct types that hold a pgx.Tx.
func addTxHolders(out map[string]map[string][]string, dir string, file *ast.File) {
	pgxName, imported := pgxLocalName(file)
	if !imported {
		return
	}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				continue
			}
			var held []string
			for _, field := range st.Fields.List {
				if !isPgxTx(field.Type, pgxName) {
					continue
				}
				if len(field.Names) == 0 {
					// EMBEDDED. There is no field name to spell, and the
					// transaction's own methods are promoted onto the receiver:
					// the borrowed handle is reached as `c.Begin(…)`, which
					// without this reads as a second connection taken by the
					// very body already holding one.
					held = append(held, promotedTx)
					continue
				}
				for _, name := range field.Names {
					held = append(held, name.Name)
				}
			}
			if len(held) == 0 {
				continue
			}
			if out[dir] == nil {
				out[dir] = map[string][]string{}
			}
			out[dir][ts.Name.Name] = held
		}
	}
}

// pgxLocalName answers the name this file spells the pgx package under, and
// whether it imports it at all.
func pgxLocalName(file *ast.File) (string, bool) {
	for _, spec := range file.Imports {
		if spec.Path.Value != `"github.com/jackc/pgx/v5"` {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name, spec.Name.Name != "_"
		}
		return "pgx", true
	}
	return "", false
}

func takesPgxTx(params *ast.FieldList, pgxName string) bool {
	if params == nil {
		return false
	}
	for _, param := range params.List {
		if isPgxTx(param.Type, pgxName) {
			return true
		}
	}
	return false
}

func isPgxTx(expr ast.Expr, pgxName string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Tx" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == pgxName
}

// acquires answers the acquirer selectors called in this body, in source
// order, excluding the tx-borrowing literals inside it — each of those is
// judged as a body of its own.
//
// A call ON the borrowed transaction is never an acquirer: `tx.Begin` opens a
// savepoint on the connection the body was handed, which is the opposite of
// reaching for a second.
func (b txBorrowing) acquires() []string {
	var found []string
	ast.Inspect(b.body, func(n ast.Node) bool {
		if lit, ok := n.(*ast.FuncLit); ok {
			return !takesPgxTx(lit.Type.Params, b.pgxName)
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.SelectorExpr:
			if _, isAcquirer := connectionAcquirers[fn.Sel.Name]; !isAcquirer {
				return true
			}
			if b.receiverIsTheBorrowedTx(fn.X, fn.Sel.Name) {
				return true
			}
			found = append(found, fn.Sel.Name)
		case *ast.Ident:
			// A package-local helper called by bare name. It has no receiver
			// that could be the borrowed transaction, so a registered
			// acquirer spelled this way is always a second connection.
			//
			// The walk read selectors only until an analytics helper reached
			// a borrowed transaction unseen: the defect this gate exists to
			// catch, wearing the one spelling it could not read.
			if _, isAcquirer := connectionAcquirers[fn.Name]; !isAcquirer {
				return true
			}
			found = append(found, fn.Name)
		}
		return true
	})
	return found
}

// receiverIsTheBorrowedTx reports whether a call's receiver is the transaction
// this body borrowed: one of its own pgx.Tx parameters, or the field its
// receiver holds one in.
func (b txBorrowing) receiverIsTheBorrowedTx(recv ast.Expr, called string) bool {
	if sel, ok := recv.(*ast.SelectorExpr); ok {
		return b.isHeldTxField(sel)
	}
	ident, ok := recv.(*ast.Ident)
	if !ok {
		return false
	}
	// A receiver that EMBEDS the transaction reaches it by its own name — but
	// only for pgx.Tx's OWN methods, and only where the type has not DECLARED
	// one of that name itself. `c.Begin(…)` on such a receiver is the promoted
	// savepoint; `c.ActiveColumns(…)` is a method the embedding type declared,
	// and so is a `Begin` it declared, which in Go wins over the promoted one.
	// Exempting either by name alone waves through an acquire wearing a name
	// this walk otherwise reads as a savepoint.
	if b.recv != "" && ident.Name == b.recv && b.embedsTx() &&
		promotedTxMethods[called] && !b.declares[called] {
		return true
	}
	for _, param := range b.params.List {
		if !isPgxTx(param.Type, b.pgxName) {
			continue
		}
		for _, name := range param.Names {
			if name.Name == ident.Name {
				return true
			}
		}
	}
	return false
}

// isHeldTxField reports whether `x.y` names the transaction this method's
// receiver holds — `c.tx` in a method on extensionCore.
//
// Anchored on the RECEIVER's name and not on the field's alone: `other.tx` is a
// different object's transaction, and reading it as this one's would wave
// through a call on a handle this body never borrowed.
func (b txBorrowing) isHeldTxField(sel *ast.SelectorExpr) bool {
	base, ok := sel.X.(*ast.Ident)
	if !ok || b.recv == "" || base.Name != b.recv {
		return false
	}
	for _, field := range b.heldTx {
		if sel.Sel.Name == field {
			return true
		}
	}
	return false
}

// Guarding the gate: the walk above is only as good as its ability to see a
// violation, and a fitness function that has never failed is a claim, not a
// gate. Each case below runs it over source holding one defect it exists for,
// and over the repair that must read clean.
func TestTheGateSeesASeamThatReachesForTheCatalogInsideTheCallersTransaction(t *testing.T) {
	t.Parallel()
	const seam = fixtureImports + `
func (s *Store) GetPersonTx(ctx context.Context, tx pgx.Tx, id ids.PersonID) (Person, error) {
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return Person{}, err
	}
	return readPerson(ctx, tx, id, active)
}
`
	assertGateReads(t, seam, "GetPersonTx", "activeColumns")

	const repaired = fixtureImports + `
func (s *Store) GetPersonTx(ctx context.Context, tx pgx.Tx, id ids.PersonID, active []fieldcatalog.Column) (Person, error) {
	if _, err := tx.Begin(ctx); err != nil {
		return Person{}, err
	}
	return readPerson(ctx, tx, id, active)
}
`
	// Nothing, and one of the two reasons is the borrowed transaction: Begin
	// IS an acquirer on a pool, so this arm is what proves the exemption for a
	// savepoint on the connection the seam was handed.
	assertGateReads(t, repaired, "GetPersonTx")
}

// The shape the gate exists to catch most: not a named seam at all, but the
// callback a transaction opener is handed.
func TestTheGateSeesAnAcquireInsideATransactionCallback(t *testing.T) {
	t.Parallel()
	const assembler = fixtureImports + `
func (s *Service) Assemble(ctx context.Context, id ids.PersonID) (Person, error) {
	var out Person
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		active, err := s.people.ActivePersonColumns(ctx)
		if err != nil {
			return err
		}
		out, err = s.people.GetPersonTx(ctx, tx, id, active)
		return err
	})
	return out, err
}
`
	assertGateReads(t, assembler, "Assemble's transaction callback", "ActivePersonColumns")

	const repaired = fixtureImports + `
func (s *Service) Assemble(ctx context.Context, id ids.PersonID) (Person, error) {
	active, err := s.people.ActivePersonColumns(ctx)
	if err != nil {
		return Person{}, err
	}
	var out Person
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		out, err = s.people.GetPersonTx(ctx, tx, id, active)
		return err
	})
	return out, err
}
`
	// The prefetch moved above the opener, so the callback is clean — and
	// Assemble itself borrows no transaction, so the WithWorkspaceTx call it
	// makes is not the gate's business.
	assertGateReads(t, repaired, "Assemble's transaction callback")
}

// A file that spells the import under another name is judged, not skipped.
func TestTheGateFollowsAnAliasedPgxImport(t *testing.T) {
	t.Parallel()
	const aliased = `package people

import pg "github.com/jackc/pgx/v5"

func (s *Store) GetPersonTx(ctx context.Context, tx pg.Tx, id ids.PersonID) (Person, error) {
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return Person{}, err
	}
	return readPerson(ctx, tx, id, active)
}
`
	assertGateReads(t, aliased, "GetPersonTx", "activeColumns")
}

// fixtureImports is the header every fixture above shares: the gate reads the
// import table to learn what "pgx" means in a file, so a fixture without it is
// not the code the gate judges.
const fixtureImports = `package people

import "github.com/jackc/pgx/v5"
`

// assertGateReads runs the walk over one fixture and asserts the acquirers it
// reports for the named body, so a case states the whole answer rather than a
// count.
func assertGateReads(t *testing.T, src, body string, want ...string) {
	t.Helper()
	bodies := txBorrowingBodies(parseGateFixture(t, src), nil, nil)
	for _, b := range bodies {
		if b.name != body {
			continue
		}
		got := b.acquires()
		if len(got) != len(want) {
			t.Fatalf("the gate read %v from %q, want %v", got, body, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("the gate read %v from %q, want %v", got, body, want)
			}
		}
		return
	}
	t.Fatalf("the gate found no body named %q in the fixture — it judged %d others", body, len(bodies))
}

// A pgx.Tx inside a func-TYPED parameter is a callback this function hands
// out, not a transaction it borrows: whoever supplies the literal is judged
// where they wrote it.
func TestTheGateReadsAFuncTypedParameterAsItsSuppliersBusiness(t *testing.T) {
	t.Parallel()
	const callbackTaker = fixtureImports + `
func (s *Store) ClaimAndEnqueue(ctx context.Context, enqueue func(tx pgx.Tx) error) error {
	return s.claim(ctx, enqueue)
}
`
	for _, b := range txBorrowingBodies(parseGateFixture(t, callbackTaker), nil, nil) {
		if b.name == "ClaimAndEnqueue" {
			t.Fatal("a function whose only pgx.Tx is the type of a callback it accepts was judged as borrowing a transaction")
		}
	}
}

func parseGateFixture(t *testing.T, src string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "gatefixture.go", src, 0)
	if err != nil {
		t.Fatalf("parsing the gate fixture: %v\n%s", err, strings.TrimSpace(src))
	}
	return file
}

// A port that HOLDS the caller's transaction is judged like one that takes it.
//
// This is the shape the parameter rule could not see, and it is not a
// hypothetical one: compose/extcore.go's first version read the workspace's
// record mode through a second pool acquire inside the caller's open
// transaction — the deadlock this gate refuses — and the gate was green,
// because no method on that receiver takes a pgx.Tx.
func TestTheGateSeesAnAcquireInAMethodOnAReceiverHoldingTheTransaction(t *testing.T) {
	t.Parallel()
	const port = fixtureImports + `
type core struct {
	tx   pgx.Tx
	pool *pgxpool.Pool
}

func (c core) RefuseOverlay(ctx context.Context) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return readMode(ctx, conn)
}
`
	assertHeldTxGateReads(t, port, "core.RefuseOverlay", "Acquire")
}

// The repair, one line apart: the same read on the transaction the receiver is
// already holding. Without this case the one above would pass against a rule
// that flagged every method on a tx-holding type, which would make the gate
// unusable and get it waived rather than obeyed.
func TestTheGateLeavesAMethodThatReadsTheHeldTransactionAlone(t *testing.T) {
	t.Parallel()
	const repaired = fixtureImports + `
type core struct {
	tx pgx.Tx
}

func (c core) RefuseOverlay(ctx context.Context) error {
	return readMode(ctx, c.tx)
}

func (c core) Nested(ctx context.Context) error {
	_, err := c.tx.Begin(ctx)
	return err
}
`
	assertHeldTxGateReads(t, repaired, "core.RefuseOverlay")
	// `Begin` IS a registered acquirer, and on the borrowed handle it is a
	// savepoint rather than a second connection — the same reasoning the
	// parameter rule already applies to `tx.Begin`.
	assertHeldTxGateReads(t, repaired, "core.Nested")
}

// Another object's transaction is not this body's to read. A rule keyed on the
// FIELD name alone would wave `other.tx` through, which is a second connection
// wearing the borrowed one's spelling.
func TestTheGateReadsAnotherObjectsTransactionAsAnAcquire(t *testing.T) {
	t.Parallel()
	// `other` is a local, so the call reads `other.tx` — an identifier that is
	// not this method's receiver. It is the shape the anchor exists for: a
	// rule keyed on the field name alone reads it as the borrowed handle and
	// waves the second connection through, and it is the ONLY shape that
	// distinguishes the two rules — `c.other.tx` fails a receiver-name test
	// for the unrelated reason that its base is not an identifier at all.
	const foreign = fixtureImports + `
type core struct {
	tx pgx.Tx
}

func (c core) Read(ctx context.Context) error {
	other := sibling()
	_, err := other.tx.Begin(ctx)
	return err
}
`
	assertHeldTxGateReads(t, foreign, "core.Read", "Begin")
}

// assertHeldTxGateReads is assertGateReads for the receiver-held shape: the
// holder index is derived from the fixture itself, exactly as the live gate
// derives it from the package.
func assertHeldTxGateReads(t *testing.T, src, body string, want ...string) {
	t.Helper()
	file := parseGateFixture(t, src)
	holders := map[string]map[string][]string{}
	declared := map[string]map[string]map[string]bool{}
	addTxHolders(holders, ".", file)
	addTxDeclaredMethods(declared, ".", file)
	bodies := txBorrowingBodies(file, holders["."], declared["."])
	for _, b := range bodies {
		if b.name != body {
			continue
		}
		got := b.acquires()
		if len(got) != len(want) {
			t.Fatalf("the gate read %v from %q, want %v", got, body, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("the gate read %v from %q, want %v", got, body, want)
			}
		}
		return
	}
	t.Fatalf("the gate found no body named %q in the fixture — it judged %d others", body, len(bodies))
}

// The rule has a live subject, and says so if it stops having one.
//
// Every case above is a fixture, and a widened rule that matches nothing in the
// tree is a rule nobody is held to: it would report PASS over a tree it had
// stopped reading, which is the one way a gate must not fail. This names the
// port the widening was written for.
func TestTheHeldTransactionRuleHasARealSubject(t *testing.T) {
	t.Parallel()
	holders, _ := txHoldingReceivers(t, []string{"internal", "cmd"})
	compose, ok := holders["internal/compose"]
	if !ok {
		t.Fatal("no receiver in internal/compose holds a pgx.Tx — the rule this file widened for reads nothing, and every case proving it is a fixture")
	}
	fields, held := compose["extensionCore"]
	if !held {
		t.Fatalf("extensionCore no longer holds a transaction; internal/compose holds %d other(s) that do — if the port was renamed, name the new one here", len(compose))
	}
	if len(fields) != 1 || fields[0] != "tx" {
		t.Fatalf("extensionCore holds its transaction in %v, want [tx]", fields)
	}
}

// A port's VERBS may live in a file that never names pgx.
//
// A method on a tx-holding receiver reads `c.tx` and mentions the package
// nowhere, so a file holding nothing but such methods has no pgx import — and
// the walk used to return before consulting the holder index at all. Every verb
// of a port whose type is declared in a sibling file was therefore invisible,
// which is the blindness this rule was widened to remove, one file over.
//
// It was not hypothetical either: dropping that early return immediately found
// project360's company section reading the custom-field catalog inside the
// page's own transaction, fixed in the same change.
func TestTheGateJudgesAHolderMethodInAFileThatNeverNamesPgx(t *testing.T) {
	t.Parallel()
	const declaration = `package compose

import "github.com/jackc/pgx/v5"

type core struct {
	tx   pgx.Tx
	pool *pgxpool.Pool
}
`
	// No pgx import: this file needs none, which is the whole point.
	const verbs = `package compose

func (c core) RefuseOverlay(ctx context.Context) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return nil
}
`
	holders := map[string]map[string][]string{}
	declared := map[string]map[string]map[string]bool{}
	addTxHolders(holders, ".", parseGateFixture(t, declaration))
	verbsFile := parseGateFixture(t, verbs)
	addTxDeclaredMethods(declared, ".", verbsFile)
	bodies := txBorrowingBodies(verbsFile, holders["."], declared["."])
	if len(bodies) != 1 || bodies[0].name != "core.RefuseOverlay" {
		t.Fatalf("the walk judged %d body/bodies in a file with no pgx import, want the one method on the holder", len(bodies))
	}
	if got := bodies[0].acquires(); len(got) != 1 || got[0] != "Acquire" {
		t.Fatalf("the gate read %v, want [Acquire]", got)
	}
}

// A method that holds a transaction AND takes one borrows two.
//
// Recorded under the parameter rule alone its receiver was unknown, so its own
// `c.tx` read came back as a second connection: the gate accused a body that
// was doing the right thing twice over. A false positive here is worse than the
// hole it closes, because a gate that cries wolf gets waived.
func TestTheGateReadsBothTransactionsOfAMethodThatHoldsAndTakesOne(t *testing.T) {
	t.Parallel()
	const both = fixtureImports + `
type core struct {
	tx pgx.Tx
}

func (c core) Reconcile(ctx context.Context, other pgx.Tx) error {
	if _, err := c.tx.Begin(ctx); err != nil {
		return err
	}
	_, err := other.Begin(ctx)
	return err
}
`
	assertHeldTxGateReads(t, both, "Reconcile")
}

// A receiver that EMBEDS the transaction reaches it by its own name.
//
// The census dropped it — an anonymous field has no `field.Names` — so the type
// held nothing as far as the gate could see, and its verbs went unjudged. Worse
// than absent: the promoted `c.Begin(…)` is a savepoint on the borrowed handle,
// and once the type IS recognised, reading it as an acquire would accuse every
// such method.
func TestTheGateReadsAnEmbeddedTransactionAsTheBorrowedOne(t *testing.T) {
	t.Parallel()
	const embedded = fixtureImports + `
type core struct {
	pgx.Tx
	pool *pgxpool.Pool
}

func (c core) Nested(ctx context.Context) error {
	_, err := c.Begin(ctx)
	return err
}

func (c core) Second(ctx context.Context) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return nil
}
`
	assertHeldTxGateReads(t, embedded, "core.Nested")
	assertHeldTxGateReads(t, embedded, "core.Second", "Acquire")
}

// An embedding type's OWN method is not the promoted transaction.
//
// `c.Begin(…)` on a receiver embedding pgx.Tx is that transaction's; `c.Tx(…)`
// or `c.activeColumns(…)` is a method the type declared, and reading it as the
// borrowed handle because the receiver happens to embed one would hide an
// acquire inside the shape that looks most like a borrowed handle. Nothing
// stops a type embedding a transaction from declaring either name.
func TestTheGateJudgesAnEmbeddingTypesOwnMethodRatherThanPromotingIt(t *testing.T) {
	t.Parallel()
	const shadowing = fixtureImports + `
type core struct {
	pgx.Tx
}

func (c core) Read(ctx context.Context) error {
	_, err := c.activeColumns(ctx, "person")
	return err
}
`
	assertHeldTxGateReads(t, shadowing, "core.Read", "activeColumns")
}

// A `Begin` the embedding type DECLARES is that method, not the promoted one.
//
// Go resolves a declared method over a promoted one, so `c.Begin(…)` here runs
// the type's own — and exempting it by name would wave through an acquire
// wearing a name this walk otherwise treats as a savepoint. That is the worst
// place to leave a hole: a shape that reads as a borrowed handle.
func TestTheGateJudgesADeclaredBeginOverThePromotedOne(t *testing.T) {
	t.Parallel()
	const shadowed = fixtureImports + `
type core struct {
	pgx.Tx
	pool *pgxpool.Pool
}

func (c core) Begin(ctx context.Context) error {
	conn, err := c.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	return nil
}

func (c core) Nested(ctx context.Context) error {
	_, err := c.Begin(ctx)
	return err
}
`
	// The declaring method is judged on its own body, which takes a connection.
	assertHeldTxGateReads(t, shadowed, "core.Begin", "Acquire")
	// And its CALLER's `c.Begin(…)` is that method rather than a savepoint, so
	// the promoted exemption does not apply to it either.
	assertHeldTxGateReads(t, shadowed, "core.Nested", "Begin")
}
