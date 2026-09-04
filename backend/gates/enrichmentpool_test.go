// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A provider handler set that can queue a RUN must carry the pool its
// visibility check reads through.
//
// The check refuses a paid lookup on a contact the caller cannot open. It
// needs a transaction, so a handler set built without a pool cannot perform it
// — and the fail-closed answer is a refusal of every enrichment request.
//
// That is exactly how it shipped broken once: WithProvider, the option that
// turns the surface ON, built its handlers from the store alone, while the
// integration test built its own handler WITH a pool and proved nothing about
// the wiring. A test that supplies its own version of production is what this
// gate replaces.
//
// The corpus is derived: every composite literal of integrationsHandlers in
// non-test compose code. A second wiring added tomorrow is gated the day it
// lands.
func TestAProviderHandlerSetCarriesItsPool(t *testing.T) {
	t.Parallel()

	const composeDir = "internal/compose"
	fset := token.NewFileSet()
	entries, err := os.ReadDir(composeDir)
	if err != nil {
		t.Fatalf("reading %s: %v", composeDir, err)
	}

	var seen, offenders []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(composeDir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		{
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				name, ok := lit.Type.(*ast.Ident)
				if !ok || name.Name != "integrationsHandlers" {
					return true
				}
				pos := fset.Position(lit.Pos())
				where := fmt.Sprintf("%s:%d", filepath.Base(path), pos.Line)
				fields := map[string]bool{}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, ok := kv.Key.(*ast.Ident); ok {
						fields[key.Name] = true
					}
				}
				seen = append(seen, where)
				// A set with no run service answers not-implemented before the
				// visibility check is ever reached, so it needs no pool.
				if fields["runs"] && !fields["pool"] {
					offenders = append(offenders, where)
				}
				return true
			})
		}
	}

	// Under-recognition is the one way this must not break: a walk that read
	// the wrong directory would find no literals and report PASS.
	if len(seen) < 3 {
		t.Fatalf("found %d integrationsHandlers literals (%v), want at least the "+
			"three in %s — the scan is reading less than it thinks",
			len(seen), seen, composeDir)
	}
	if len(offenders) > 0 {
		t.Errorf("these handler sets can queue a run but carry no pool: %v\n"+
			"The visibility check reads through a transaction, so without a pool "+
			"every enrichment request is refused. Pass the pool the option already holds.",
			offenders)
	}
}
