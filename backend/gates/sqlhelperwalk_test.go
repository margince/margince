// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The walk two "one definition" censuses share: how a SQL statement assembled
// out of literals, concatenation and helper calls is rendered back into the
// text it builds, and how a call to THE definition is told from a lookalike.
//
// It lives on its own because there are two ports of it now — employment
// currency (people.EmploymentIsCurrentSQL) and workforce liveness
// (identity.LiveMemberSQL) — and a walk copied for the second port drifts from
// the first. The narrower copy then reads a smaller tree and says PASS, which
// is the failure mode a census cannot report about itself.

import (
	"go/ast"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// flattenSQL renders a string expression as the text it builds, marking every
// node it consumed so an inner piece is not judged again on its own.
//
// A call to the owning module's helper renders as a neutral marker: the predicate it
// produces exists only at runtime, so the statement's text carries no
// hand-written test from it, and a statement that calls the helper is simply a
// statement with nothing hand-written left to find.
//
// It replaced an exemption — "this statement mentions the helper, so skip
// it" — which was too coarse in the direction that matters: a query calling
// the helper for one half and hand-writing the other was skipped WHOLESALE.
// Calling the helper is not a licence to write a second predicate beside it.
//
// Any other call renders as its ARGUMENTS and not its name, because a
// formatter holds its SQL in an argument — `fmt.Sprintf(`… <predicate> … `, …)` keeps the whole statement inside the call, and a flattener that
// stopped at the callee name would judge nothing. The name is dropped because
// it is not part of the SQL; only the helper's is kept, as the marker that
// says the helper was reached.
func flattenSQL(n ast.Node, seen map[ast.Node]bool, owner helperScope) (string, bool) {
	switch v := n.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		seen[n] = true
		return gatekit.TextOf(v), true
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, lok := flattenSQL(v.X, seen, owner)
		right, rok := flattenSQL(v.Y, seen, owner)
		if !lok && !rok {
			return "", false
		}
		seen[n] = true
		return left + right, true
	case *ast.CallExpr:
		seen[n] = true
		if owner.isOneDefinition(v) {
			markSeen(v, seen)
			return " " + calleeName(v) + " ", true
		}
		text := ""
		for _, a := range v.Args {
			if part, ok := flattenSQL(a, seen, owner); ok {
				text += part
			}
		}
		return " " + text + " ", true
	case *ast.CompositeLit:
		// A statement assembled as `strings.Join([]string{…}, " ")` kept its
		// pieces in a slice literal, which rendered as a blank — so each piece
		// was judged alone, and the piece naming the table carried no predicate
		// while the piece carrying the predicate named no table. Neither half
		// could ever be a finding.
		seen[n] = true
		text := ""
		for _, elt := range v.Elts {
			if part, ok := flattenSQL(elt, seen, owner); ok {
				text += part
			}
		}
		return text, true
	case *ast.FuncLit:
		// NOT claimed. A callback body is where most of this tree's SQL lives —
		// `db.Tx(ctx, func(tx pgx.Tx) error { … })` is the shape every store
		// write takes — and rendering the literal as a neutral space marked it
		// seen, so the walk refused to descend and every statement inside it
		// went unjudged. Returning false leaves it unclaimed for the walk to
		// reach on its own.
		return "", false
	case ast.Expr:
		seen[n] = true
		return " ", true
	}
	return "", false
}

// helperScope says how the owning module's helper is reachable from one file, and it is
// a struct rather than a string because the string had two meanings.
//
// `qualifier == ""` was read as "this file IS the owning package", where a bare
// call is the helper's own. But importAliasOf returns "" for every file that
// does not import the owner at all — which is most of the tree — so in any of
// them a bare call to the helper's NAME was accepted as canonical and its
// arguments hidden. `inside` says the one thing the empty string could not.
type helperScope struct {
	qualifier string          // the local name the owning module is bound to, "" if not imported
	inside    bool            // this file IS the owning package
	names     map[string]bool // the exported helper(s) that ARE the definition
}

// isOneDefinition reports whether the call is the owning module's helper — the
// PACKAGE as well as the name.
//
// The name alone was not enough, and the gap is the one this gate exists to
// close: a helper call's whole subtree is claimed, so `other.
// EmploymentIsCurrentSQL(…)` would have been treated as canonical and its
// arguments hidden, letting a hand-written currency test ride inside a
// lookalike.
func (h helperScope) isOneDefinition(call *ast.CallExpr) bool {
	// A scope with no names can never recognise the definition, so every call
	// to it would flatten to its arguments and the gate would judge a compliant
	// statement as a hand-written one — or, worse, pass it because the argument
	// alone matches nothing. That is a construction error in the caller and
	// never a property of the tree, so it fails here rather than quietly
	// changing what the census means.
	if len(h.names) == 0 {
		panic("helperScope built with no helper names: it cannot tell the one definition from anything else")
	}
	named := func(n string) bool { return h.names[n] }
	switch f := call.Fun.(type) {
	case *ast.Ident:
		return h.inside && named(f.Name)
	case *ast.SelectorExpr:
		pkg, ok := f.X.(*ast.Ident)
		return ok && h.qualifier != "" && pkg.Name == h.qualifier && named(f.Sel.Name)
	}
	return false
}

// importAliasOf returns the local name an import path is bound to in this file,
// or "" if the file does not import it. A dot import returns "" too: a
// dot-imported call is a bare identifier, and the gate would rather miss it
// than name the wrong function.
func importAliasOf(file *ast.File, path string) string {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, `"`) != path {
			continue
		}
		if spec.Name != nil {
			if spec.Name.Name == "." {
				return ""
			}
			return spec.Name.Name
		}
		// No alias: the local name is the package's own, which for every path
		// this repo binds is the last segment. Returning a FIXED name here (it
		// said "people", a leftover from the single-port version) made every
		// unaliased import of any other module read as a lookalike, so the
		// owning module's helper was never recognised in the one place it is
		// most used.
		return path[strings.LastIndex(path, "/")+1:]
	}
	return ""
}

// markSeen claims a whole subtree, so nothing inside a helper call is judged as
// though somebody had written it into the statement.
func markSeen(n ast.Node, seen map[ast.Node]bool) {
	ast.Inspect(n, func(c ast.Node) bool {
		if c != nil {
			seen[c] = true
		}
		return true
	})
}

// handWrittenGoSources walks the module for source a person maintains.
func handWrittenGoSources(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if name := entry.Name(); name == "node_modules" || name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, ".gen.go") {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for Go source: %v", err)
	}
	if len(paths) < 500 {
		t.Fatalf("the walk found only %d Go files, so this census covered almost nothing", len(paths))
	}
	return paths
}
