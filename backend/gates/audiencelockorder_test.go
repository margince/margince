// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Several activity audiences are recomputed in ONE agreed order, or two
// transactions deadlock.
//
// RecomputeAudienceTx takes `FOR UPDATE` on the activity it derives, so a caller
// holding several of them is choosing a lock order — and a caller that loops
// over whatever its own query returned is choosing one by accident. Two such
// callers that disagree can each hold the row the other needs next, which
// Postgres resolves by aborting one of them: a failed verdict pass or a failed
// release batch, retried on the next tick.
//
// The plural, RecomputeAudiencesTx, sorts before it locks. This refuses the loop
// that goes around the singular instead, because that is the shape the defect
// took and the shape a new caller would copy from a sibling.
//
// SCOPE, stated rather than assumed. It reads `compose`, which is where the
// callers that can reach the plural live. `capture` loops an INJECTED
// recomputer — a module never imports a sibling, so it cannot call the plural —
// and sorts with the same ids.Ascending in recomputeEach; a gate cannot see that
// call by name, so recomputeEach's own comment carries it and this does not
// claim to.
//
// A loop that opens its own transaction per id is NOT this defect and is not
// refused: locks it takes are released before the next one, so there is no pair
// to cycle. capturepurge.go and linkreconcile.go are both that shape.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	audienceRecomputeSingular = "RecomputeAudienceTx"
	audienceRecomputeTree     = "internal/compose"
)

func TestSeveralAudiencesAreRecomputedInOneOrder(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(audienceRecomputeTree)
	if err != nil {
		t.Fatalf("reading %s: %v", audienceRecomputeTree, err)
	}
	scanned, found := 0, []string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(audienceRecomputeTree, name)
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		scanned++
		found = append(found, loopedRecomputes(file, path)...)
	}
	// A census that read no files would report a clean tree. compose is not a
	// small package and never will be.
	if scanned < 50 {
		t.Fatalf("read %d files in %s and expected far more — this gate has stopped reaching "+
			"its corpus rather than the tree having given it up", scanned, audienceRecomputeTree)
	}
	if len(found) > 0 {
		t.Errorf("%d loop(s) call the singular recompute per id:\n\t%s\n"+
			"Pass the ids to RecomputeAudiencesTx instead, which sorts before it locks. Looping "+
			"the singular takes activity row locks in whatever order the query returned, and a "+
			"second caller that disagrees deadlocks against it.",
			len(found), strings.Join(found, "\n\t"))
	}
}

// loopedRecomputes reports every call to the singular that sits inside a loop
// AND inside the same transaction — a loop that opens a transaction of its own
// per id releases each lock before taking the next and cannot cycle.
func loopedRecomputes(file *ast.File, path string) []string {
	var found []string
	ast.Inspect(file, func(node ast.Node) bool {
		var body *ast.BlockStmt
		switch loop := node.(type) {
		case *ast.RangeStmt:
			body = loop.Body
		case *ast.ForStmt:
			body = loop.Body
		default:
			return true
		}
		ast.Inspect(body, func(inner ast.Node) bool {
			call, isCall := inner.(*ast.CallExpr)
			if !isCall {
				return true
			}
			if opensItsOwnTransaction(call) {
				return false
			}
			if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel &&
				sel.Sel.Name == audienceRecomputeSingular {
				found = append(found, path)
			}
			return true
		})
		return true
	})
	return found
}

// opensItsOwnTransaction recognises the per-id transaction shape, whose locks
// are released before the loop comes round again.
func opensItsOwnTransaction(call *ast.CallExpr) bool {
	sel, isSel := call.Fun.(*ast.SelectorExpr)
	return isSel && strings.HasSuffix(sel.Sel.Name, "WithWorkspaceTx")
}
