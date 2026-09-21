// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind reachability H2

package gates

// Every approval effect redeems the approval it ran.
//
// WHY THIS IS LOAD-BEARING. An effect runs after its decision commits, so a
// failure leaves the row approved with consumed_at NULL. Deciding again
// re-drives it (approvals.effectIsStillOwed), which is what stops a transient
// fault from stranding a human's yes forever — and the only reason that is safe
// is that consumed_at answers "did this run" as a FACT. It answers it because
// every effect consumes the approval in the same transaction as its write: one
// that wrote anything left consumed_at set and is refused a second run.
//
// An effect registered without a redemption breaks that silently and in the
// worst direction. It would report success, leave consumed_at NULL, and be
// re-driven by the next decision on the row — applying its write a second time,
// for kinds that are emphatically not idempotent (a send, a merge, an owner
// reassignment). Nothing else in the tree would fail.
//
// WHAT THIS CANNOT SEE, because a reader should not take more from it than it
// says. The walk is syntactic: it asks whether a redemption is REACHABLE from
// the registered function, not whether every path through it redeems. An effect
// that redeems on one branch and returns nil on another satisfies this gate,
// and only the branch's own test would catch it. What the gate is for is the
// omission — a kind registered with no redemption anywhere, which is how a new
// effect arrives — and that it does catch, including one added to the
// table-driven registration.

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

// redemptionCalls are the ways an effect consumes the approval it ran. All
// three, because which one an effect reaches is its own business: the plain
// Redeem for a kind that writes through a store opening its own transaction,
// RedeemAndApply where the write can join the redemption's, RedeemInTx where
// the caller already holds one.
var redemptionCalls = []string{"Redeem", "RedeemAndApply", "RedeemInTx"}

// effectRegistrationScope sweeps for files that register an approval effect.
//
// The subject is the WithEffect CALL rather than a directory listing, so a
// registration added in a new file joins the corpus by being a registration.
// gatekit.Scope then sweeps the negative space: a registration outside the
// composition root is reported here rather than passed over, which is what
// makes the root a proof instead of an assertion.
var effectRegistrationScope = gatekit.Scope{
	Roots:   []string{"internal/compose"},
	Subject: func(_ string, file *ast.File) bool { return registersAnEffect(file) },
	Exempt:  gatekit.Waive(map[string]string{}),
}

func registersAnEffect(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "WithEffect" {
			found = true
		}
		return !found
	})
	return found
}

func TestEveryRegisteredEffectRedeemsTheApprovalItRan(t *testing.T) {
	t.Parallel()
	files := composeFilesFor(t)
	decls := functionDeclarations(files)
	registered := effectConstructors(t, effectRegistrationScope.Files(t))
	if len(registered) == 0 {
		t.Fatal("no approval effect is registered anywhere under the composition root — this gate read an " +
			"empty corpus, which is the one way it can report success over a surface it never saw")
	}
	for _, name := range registered {
		decl, known := decls[name]
		if !known {
			t.Errorf("%s is registered as an approval effect and its declaration is not in the composition "+
				"root — this gate cannot see whether it redeems, and an effect it cannot see is one it "+
				"cannot vouch for", name)
			continue
		}
		if !reachesARedemption(decl, decls, map[string]bool{}) {
			t.Errorf("%s is registered as an approval effect and reaches no redemption. An effect that "+
				"does not consume its approval leaves consumed_at NULL after it succeeds, so the next "+
				"decision on that row re-drives it and its write lands twice. Redeem through the service "+
				"(Redeem, RedeemAndApply or RedeemInTx) in the same transaction as the write", name)
		}
	}
}

// effectConstructors reads the function named as each WithEffect's second
// argument — the thing that builds the effect, and therefore the thing whose
// body holds the redemption.
func effectConstructors(t *testing.T, files []gatekit.ParsedFile) []string {
	t.Helper()
	byField := functionsAssignedToFields(files)
	seen := map[string]bool{}
	for _, parsed := range files {
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "WithEffect" || len(call.Args) != 2 {
				return true
			}
			if named := effectFunctionIdent(call.Args[1]); named != nil {
				name := named.Name
				// A registration may hand over a struct FIELD holding the
				// function (`late.effect(svc, …)`, sendpath.go), in which case
				// the name here is the field's and the functions are in the
				// literal that fills it. Following that is what keeps a table-
				// driven registration inside the gate instead of reported as
				// unreadable — and a table is precisely where a kind gets added
				// without anybody looking at this gate.
				if supplied := byField[name]; len(supplied) > 0 {
					for _, fn := range supplied {
						seen[fn] = true
					}
					return true
				}
				seen[name] = true
				return true
			}
			// An effect written inline has no name to follow. Reported rather
			// than skipped: an unreadable registration is exactly the shape
			// this gate must not pass over in silence.
			t.Errorf("%s: an approval effect is registered as an expression this gate cannot resolve to a "+
				"function, so whether it redeems is unchecked. Register a named constructor", parsed.Path)
			return true
		})
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// effectFunctionIdent is the identifier a registration argument ultimately
// names, whether the effect is built by calling something
// (`heldAcceptEffect(svc, …)`), handed over by name, or reached through a
// selector.
//
// It answers with the IDENTIFIER rather than a string, because the question is
// which declaration this argument points at — not what text the expression
// holds. gatekit.StringExpr answers that other question, and reaching for it
// here would be asking a value extractor to name a function.
func effectFunctionIdent(arg ast.Expr) *ast.Ident {
	switch node := arg.(type) {
	case *ast.CallExpr:
		return effectFunctionIdent(node.Fun)
	case *ast.SelectorExpr:
		return node.Sel
	case *ast.Ident:
		return node
	}
	return nil
}

// reachesARedemption walks from an effect's constructor into the functions it
// calls, because the redemption is rarely in the body that was registered: a
// constructor returns the closure, and several delegate the write to a helper
// beside them.
//
// Bounded by the functions this corpus holds, and by `walked`, so a cycle
// terminates. A call out of the corpus is not a redemption and not a defect on
// its own — what the gate asserts is that a redemption IS reachable, and a name
// it cannot follow simply is not one.
func reachesARedemption(decl *ast.FuncDecl, decls map[string]*ast.FuncDecl, walked map[string]bool) bool {
	if walked[decl.Name.Name] {
		return false
	}
	walked[decl.Name.Name] = true
	if gatekit.CallsAny(decl, redemptionCalls) {
		return true
	}
	reached := false
	ast.Inspect(decl, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || reached {
			return !reached
		}
		if next, known := decls[calleeName(call)]; known && reachesARedemption(next, decls, walked) {
			reached = true
		}
		return !reached
	})
	return reached
}

// functionDeclarations indexes the corpus by function name, which is how the
// walk above follows a call.
func functionDeclarations(files []gatekit.ParsedFile) map[string]*ast.FuncDecl {
	decls := map[string]*ast.FuncDecl{}
	for _, parsed := range files {
		for _, d := range parsed.File.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil {
				decls[fn.Name.Name] = fn
			}
		}
	}
	return decls
}

// composeFilesFor parses the whole composition root, not only the files that
// register an effect: the redemption is often a call away, in a sibling file
// the registration never mentions.
//
// A plain read rather than a gatekit.Scope, because it makes no coverage claim
// — effectRegistrationScope is what proves the gate looks where the
// registrations are, and this only supplies the bodies that walk follows.
func composeFilesFor(t *testing.T) []gatekit.ParsedFile {
	t.Helper()
	root := filepath.Join(repoRoot, "backend", "internal", "compose")
	fset := token.NewFileSet()
	var parsed []gatekit.ParsedFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return parseErr
		}
		parsed = append(parsed, gatekit.ParsedFile{Path: path, File: file})
		return nil
	})
	if err != nil {
		t.Fatalf("parsing the composition root: %v", err)
	}
	if len(parsed) == 0 {
		t.Fatalf("no Go source under %s — the walk below would follow no call and every effect would "+
			"report as unreadable", root)
	}
	return parsed
}

// functionsAssignedToFields indexes every function name a composite literal
// assigns to a named field, so a registration that hands over a field can be
// followed to what fills it.
//
// By field name across the whole corpus rather than by resolved type: the type
// checker would answer precisely, and it is a far heavier gate for an answer
// this shape already gives — a field named `effect` filled with a function is
// an effect, whatever struct carries it. Over-reach costs a redundant check;
// under-reach would be an effect nobody looked at.
func functionsAssignedToFields(files []gatekit.ParsedFile) map[string][]string {
	byField := map[string][]string{}
	for _, parsed := range files {
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				field, isField := kv.Key.(*ast.Ident)
				value, isFunc := kv.Value.(*ast.Ident)
				if isField && isFunc {
					byField[field.Name] = append(byField[field.Name], value.Name)
				}
			}
			return true
		})
	}
	return byField
}
