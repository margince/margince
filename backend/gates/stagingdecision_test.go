// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package gates

// Every path that stages a delivery records why it was allowed to.
//
// The transmit ticket catches a delivery that reaches a provider with nothing
// on record, but it catches it in a worker — minutes or days later, against a
// parked row, with the person who typed the message long gone. The staging
// decision is what makes a refusal answerable, and this is what stops a new
// send path from quietly not taking one.
//
// Derived rather than listed, on both axes. The method names come from the
// comms store's own declarations, so a fourth staging method is judged the day
// it is declared; and the corpus walks every package under the compose tier,
// because a stager wired through WithDelivery may live in a subpackage. A
// hand-kept list on either axis would go stale exactly when a new door
// appeared, which is the failure this gate exists to prevent.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// commsStoreDir holds the delivery store whose staging methods this gate
// derives its subject from.
const commsStoreDir = "internal/modules/comms"

func TestEveryStagedDeliveryRecordsWhyItWasAllowed(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	stagers := stagingMethodNames(t, fset)

	staging := map[string]bool{}     // functions that stage a delivery
	authorizing := map[string]bool{} // functions that record a decision

	for _, file := range parseTreeUnder(t, fset, composeTier) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// Receiver-qualified, because the method NAME is fixed by the
			// interface: every type satisfying activities.DeliveryStager must
			// call its method StageTx. Keyed on the bare name, a wrapper that
			// staged without deciding would land in the same map entry as the
			// one that decides, and the gate would absorb it and report PASS —
			// which is the exact shape a second send door takes.
			name := receiverQualified(fn)
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch {
				case stagers[sel.Sel.Name]:
					staging[name] = true
				case sel.Sel.Name == "AuthorizeStagingTx":
					authorizing[name] = true
				}
				return true
			})
		}
	}

	// Under-recognition is the one way this gate must not break: a scan that
	// found nothing would report PASS over an empty corpus.
	if len(staging) == 0 {
		t.Fatal("found no delivery staging in compose — the scan is looking in the wrong place")
	}
	for fn := range staging {
		if !authorizing[fn] {
			t.Errorf("%s stages a delivery and records no authorization decision — "+
				"a message queued with nothing on record about why is one nobody can answer for", fn)
		}
	}
}

// stagingMethodNames reads the comms store's own declarations for the methods
// that queue a message. Deriving them is what makes a fourth door — a
// StageBroadcastTx, say — a subject of this gate on the day it is written
// rather than on the day somebody remembers to add it here.
//
// StageOrJoinPendingInTx on the site-read transport is deliberately not
// matched: it declares no delivery and queues no message, so it owes no
// decision. That is why this reads the comms store alone rather than every
// Stage* method in the tree.
func stagingMethodNames(t *testing.T, fset *token.FileSet) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, file := range parsePackageDir(t, fset, commsStoreDir) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "Stage") && strings.HasSuffix(fn.Name.Name, "Tx") {
				names[fn.Name.Name] = true
			}
		}
	}
	if len(names) == 0 {
		t.Fatalf("found no Stage…Tx methods in %s — the scan is looking in the wrong place", commsStoreDir)
	}
	return names
}

// parseTreeUnder parses every hand-written Go file under a directory, walking
// into subpackages. A gate that names identifiers needs one package's scope,
// but this one counts call sites, and a call site in a subpackage stages just
// as real a message as one beside the wiring.
func parseTreeUnder(t *testing.T, fset *token.FileSet, root string) []*ast.File {
	t.Helper()
	var files []*ast.File
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		files = append(files, file)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return files
}

// receiverQualified names a function the way this gate must count it: the
// receiver type and the method, so two types implementing one interface method
// are two subjects rather than one.
func receiverQualified(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}
