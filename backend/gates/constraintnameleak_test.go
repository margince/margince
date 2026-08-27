// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A constraint's name goes in the operator's log, never in the caller's refusal.
//
// storekit.CheckViolation and UniqueViolation hand back the name of the rule
// that fired, and there are two things to do with it. Switch on it, to answer
// with a refusal written for that rule — which every site in this tree does, and
// which is the whole point of exposing it. Or put it in the message, which
// `activities/handlers.go` once did:
//
//	"the request violates the " + constraint + " business rule"
//
// A schema identifier tells a caller the shape of a table they cannot otherwise
// see and nothing they can act on — `activity_link_shape` names no field in the
// published contract — and it is not API: renaming a constraint in a migration
// would silently change a response body. httperr's own constraint net says so
// at its generic path, and refuses to name one there (issue #1522).
//
// A PROHIBITION rather than a census, because the failure is a leak by default:
// interpolation costs nothing to write and the next constraint inherits it. The
// rule is that the value never becomes message text — passing it to a mapper
// that switches on it is the sanctioned use and is what the sweep expects to
// find.
//
// What this cannot see, stated rather than implied: a name copied into another
// variable first, or one that reaches a message through a helper this walk does
// not follow. The walk is syntactic. It catches the shape that was actually
// written here and the shape the next author would reach for, which is the
// concatenation and the format verb — not an author determined to launder it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const storekitPkg = "github.com/margince/margince/backend/internal/platform/database/storekit"

// The two storekit doors that hand back a constraint name.
//
// Matched through the file's own import of storekit, never by the selector name
// alone. `CheckViolation` is an ordinary method name — an unrelated receiver
// carrying one would enter this sweep, produce findings about a value that is
// not a constraint name, and, worse, keep the empty-sweep guard below satisfied
// after every real storekit call had gone.
var constraintDoors = map[string]bool{
	"CheckViolation":  true,
	"UniqueViolation": true,
}

// storekitDoor reports a call to one of those doors THROUGH this file's storekit
// import, under whatever name the file gave it.
func storekitDoor(file *ast.File, expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || !constraintDoors[sel.Sel.Name] {
		return false
	}
	qualifier, dotImported := gatekit.ImportedAs(file, storekitPkg)
	receiver, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	// A dot-import puts the door in scope bare, and there is no receiver to
	// check — the selector itself is then the whole reference.
	if dotImported {
		return true
	}
	return qualifier != "" && receiver.Name == qualifier
}

// The formatters a leak would travel through. A name reaching one of these is
// on its way into a string, and every string this tree builds from an error is
// a string somebody may write to a client.
var stringBuilders = map[string]bool{
	"Sprintf": true, "Errorf": true, "Sprint": true, "Sprintln": true,
	"Join": true, "Replace": true, "ReplaceAll": true,
}

func TestNoConstraintNameIsBuiltIntoAMessage(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileReadsAConstraintName,
		// Not extensions/: a unit cannot import storekit at all, so it never
		// holds one of these names to leak.
	}.Files(t)

	sites, judged := 0, 0
	for _, parsed := range files {
		// storekit itself is where the name is produced and named; its own
		// error text is the operator's, not a caller's.
		if strings.Contains(parsed.Path, "/platform/database/storekit/") {
			continue
		}
		judged++
		for _, leak := range constraintNameLeaksIn(parsed) {
			sites++
			t.Errorf("%s: %s\n"+
				"\tA constraint name is schema the caller cannot otherwise see, and nothing they "+
				"can act on — it names no field in the published contract, and renaming it in a "+
				"migration would silently change a response body.\n"+
				"\tSwitch on it to answer with a refusal written for that rule, and let anything "+
				"unmapped fall through to httperr's field-less one. Log the name if an operator "+
				"needs it.", parsed.Path, leak)
		}
	}
	// The gate has to have JUDGED something, and the count is taken after the
	// exclusion rather than before it. `len(files)` includes storekit's own
	// files, so a tree where every remaining caller had gone — or a walk that
	// found only storekit — would report a swept, successful, empty run. Every
	// module that answers a schema refusal reads one of these doors, so nothing
	// judged means the walk broke or the doors were renamed.
	if judged == 0 {
		t.Fatal("no file OUTSIDE storekit reads a constraint name, so this prohibition judged " +
			"nothing — the walk is broken, or storekit's doors were renamed and this rule now " +
			"binds names nobody calls")
	}
	if sites == 0 {
		t.Logf("judged %d file(s) that read a constraint name; none builds it into a message", judged)
	}
}

func fileReadsAConstraintName(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && storekitDoor(file, call.Fun) {
			found = true
		}
		return true
	})
	return found
}

// constraintNameLeaksIn reports each place a name bound from one of the doors is
// concatenated or formatted, described for the message above.
func constraintNameLeaksIn(parsed gatekit.ParsedFile) []string {
	var leaks []string
	for _, name := range constraintBindings(parsed.File) {
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BinaryExpr:
				// `"…" + constraint + "…"` — the shape that was written.
				if node.Op == token.ADD && (identIs(node.X, name) || identIs(node.Y, name)) {
					leaks = append(leaks, "`"+name+"` is concatenated into a string")
				}
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || !stringBuilders[sel.Sel.Name] {
					return true
				}
				for _, arg := range node.Args {
					if identIs(arg, name) {
						leaks = append(leaks, "`"+name+"` is formatted by "+sel.Sel.Name)
					}
				}
			}
			return true
		})
	}
	return dedupeLeaks(leaks)
}

// constraintBindings reads the identifiers this file binds from a door — the
// `constraint` in `constraint, ok := storekit.CheckViolation(err)`, whatever it
// is spelled.
//
// Held by: TestNoConstraintNameIsBuiltIntoAMessage
func constraintBindings(file *ast.File) []string {
	var names []string
	// Both spellings of one binding. `constraint, ok := storekit.CheckViolation(err)`
	// is an assignment; `var constraint, ok = storekit.CheckViolation(err)` is a
	// declaration, and Go gives it a different node. A walk that knew only the
	// first would sweep such a file — it calls a door — and then track no name
	// in it, so every leak there would pass.
	bind := func(lhs []ast.Expr, rhs []ast.Expr) {
		if len(rhs) != 1 || len(lhs) == 0 {
			return
		}
		call, ok := rhs[0].(*ast.CallExpr)
		if !ok || !storekitDoor(file, call.Fun) {
			return
		}
		if ident, ok := lhs[0].(*ast.Ident); ok && ident.Name != "_" {
			names = append(names, ident.Name)
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			bind(node.Lhs, node.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, 0, len(node.Names))
			for _, name := range node.Names {
				lhs = append(lhs, name)
			}
			bind(lhs, node.Values)
		}
		return true
	})
	return names
}

func identIs(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func dedupeLeaks(items []string) []string {
	var kept []string
	seen := map[string]bool{}
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			kept = append(kept, item)
		}
	}
	return kept
}

// The falsification half: what the rule must NOT catch, and what it must.
//
// Every case above runs over a clean tree, so all of them pass whether the
// qualifier is checked or not — there is no unrelated `CheckViolation` in this
// repository to be confused by. That is exactly why the check is easy to write
// wrongly and impossible to notice: `CheckViolation` is an ordinary method name,
// and a receiver that grew one would enter the sweep, produce findings about a
// value that is not a constraint name, and keep the empty-sweep guard satisfied
// long after every real storekit call had gone.
//
// So the decoy is written here rather than committed to the tree.
func TestOnlyStorekitsOwnDoorsCount(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		source string
		reads  bool
		leaks  int
	}{
		"storekit's door, leaked": {
			source: `package p
import (
	"fmt"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)
func f(err error) error {
	if constraint, ok := storekit.CheckViolation(err); ok {
		return fmt.Errorf("violates %s", constraint)
	}
	return err
}`,
			reads: true,
			leaks: 1,
		},
		"storekit's door under an alias": {
			source: `package p
import (
	"fmt"
	sk "github.com/margince/margince/backend/internal/platform/database/storekit"
)
func f(err error) error {
	if constraint, ok := sk.UniqueViolation(err); ok {
		return fmt.Errorf("violates %s", constraint)
	}
	return err
}`,
			reads: true,
			leaks: 1,
		},
		// The decoy. Same method name, a receiver that is not storekit — an
		// audit log, a policy engine, anything. Neither its value nor its
		// message is this rule's business, and a gate that swept it would
		// report a leak about a string that is not a constraint name.
		"an unrelated receiver of the same name": {
			source: `package p
import "fmt"
type checker struct{}
func (checker) CheckViolation(err error) (string, bool) { return "", false }
func f(c checker, err error) error {
	if reason, ok := c.CheckViolation(err); ok {
		return fmt.Errorf("violates %s", reason)
	}
	return err
}`,
			reads: false,
			leaks: 0,
		},
		// The declaration form, which Go gives a different node from the
		// assignment above. A walk that knew only `:=` would SWEEP this file —
		// it calls a door — and then track no name in it, so the leak passes.
		"a var declaration binding, leaked": {
			source: `package p
import (
	"fmt"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)
func f(err error) error {
	var constraint, ok = storekit.CheckViolation(err)
	if ok {
		return fmt.Errorf("violates %s", constraint)
	}
	return err
}`,
			reads: true,
			leaks: 1,
		},
		"the sanctioned use — switched on, never printed": {
			source: `package p
import (
	"errors"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)
func f(err error) error {
	if constraint, ok := storekit.CheckViolation(err); ok {
		switch constraint {
		case "contract_term_order":
			return errors.New("a term cannot end before it starts")
		}
	}
	return err
}`,
			reads: true,
			leaks: 0,
		},
	} {
		t.Run(name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", tc.source, 0)
			if err != nil {
				t.Fatalf("parsing the probe: %v", err)
			}
			if got := fileReadsAConstraintName("probe.go", file); got != tc.reads {
				t.Errorf("fileReadsAConstraintName = %v, want %v — the sweep %s this file",
					got, tc.reads,
					map[bool]string{true: "must not enter", false: "must enter"}[got])
			}
			leaks := constraintNameLeaksIn(gatekit.ParsedFile{Path: "probe.go", File: file})
			if len(leaks) != tc.leaks {
				t.Errorf("found %d leak(s) %v, want %d", len(leaks), leaks, tc.leaks)
			}
		})
	}
}
