// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A precheck that exists but is not wired protects nothing.
//
// approvals.Service.WithPrecheck refuses a DECISION whose payload the effect
// could not use, which is the only defence a kind has against an approved row
// nothing can re-decide: the decision commits before the effect runs, and a
// failed effect never un-decides it. Registering it is one line in the
// composition, and forgetting that line is invisible — every test that wires
// its own service still passes, and the product silently records a yes that
// produces nothing.
//
// So the obligation is derived from the source rather than listed: a
// `func xPrecheck(` under internal/compose must be REFERENCED as an identifier
// from another non-test file. Writing one and not wiring it is the failure this
// catches.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A precheck reaches WithPrecheck two ways in this tree: passed directly, or
// carried as a struct field through a registration table (sendpath.go's
// lateApprovalEffects). Matching only the direct call reported that table's
// entry as unwired, so what counts as wiring is any use of the function's
// IDENTIFIER from a non-test file other than the one declaring it.
//
// It reads the parsed syntax tree rather than the file text. A text scan counts
// the name inside a comment or a string literal as a use, so deleting the real
// registration while leaving the name in a sentence above it keeps the gate
// green — under-recognition, which is the one way a census must not break.
// go/ast sees identifiers only, so prose cannot satisfy it.
func precheckName(name string) bool {
	return strings.HasSuffix(name, "Precheck") || strings.HasSuffix(name, "precheck")
}

// composeFile is one parsed non-test file: its package-qualified position and
// the syntax tree the scan reads.
type composeFile struct {
	path string
	pkg  string
	file *ast.File
}

func parseComposeFiles(t *testing.T, root string) []composeFile {
	t.Helper()
	fset := token.NewFileSet()
	var out []composeFile
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		// Parse without comments so a name in prose is not in the tree at all.
		parsed, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		out = append(out, composeFile{path: path, pkg: parsed.Name.Name, file: parsed})
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return out
}

func TestEveryPrecheckInComposeIsWired(t *testing.T) {
	t.Parallel()
	files := parseComposeFiles(t, filepath.Join("internal", "compose"))

	// Keyed by package AND name: two compose subpackages may each declare a
	// helper of the same name, and a bare-name key would let one package's
	// wiring vouch for the other's unwired copy.
	type symbol struct{ pkg, name string }
	declared := map[symbol]string{}
	usedElsewhere := map[symbol]bool{}

	for _, f := range files {
		for _, decl := range f.file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && precheckName(fn.Name.Name) {
				declared[symbol{f.pkg, fn.Name.Name}] = f.path
			}
		}
	}
	for _, f := range files {
		ast.Inspect(f.file, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok || !precheckName(id.Name) {
				return true
			}
			key := symbol{f.pkg, id.Name}
			if at, isDeclared := declared[key]; isDeclared && at != f.path {
				usedElsewhere[key] = true
			}
			return true
		})
	}

	if len(declared) == 0 {
		t.Fatal("no precheck found under internal/compose, so this gate read a smaller tree than it thinks")
	}
	for key, path := range declared {
		if !usedElsewhere[key] {
			t.Errorf("%s declares %s but no other non-test file under internal/compose "+
				"references it — an unwired precheck refuses nothing, and the kind's "+
				"effect can still fail after its decision has committed", path, key.name)
		}
	}
}
