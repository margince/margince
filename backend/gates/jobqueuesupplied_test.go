// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A job's queue is supplied from api/jobs.yaml, never written at the insert.
//
// The declaration is what the fleet surfaces publish and what jobqueues.go
// sizes a worker pool against. For an insert built by hand it was also, until
// jobs.QueuedAs, documentation the runtime never read: moving comms_send_email
// onto its own queue changed the declaration, the merge gate went green, and
// every send carried on landing on `default` — because an InsertOpts naming no
// queue is River's own default and nothing compared the two.
//
// So a river.InsertOpts that names a Queue is the defect, wherever it is
// written. Nothing softer works: the value the declaration wants and the value
// the insert uses are both plain strings, both plausible, and the one that
// wins is invisible until an operator asks why a sized pool is empty.

import (
	"go/ast"
	"go/parser"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// insertOptsFloor guards against a vacuous pass. The tree builds around forty
// river.InsertOpts today; the floor sits low enough that retiring a handful of
// jobs does not drag it along, and high enough that a walk which has stopped
// recognising the literal — a rename, a parser change, a wrong root — is
// reported rather than read as a clean tree.
//
// Recognising the literal is the whole subject: the queue field is the thing
// this refuses, so a scan finding NO literals would refuse nothing and say so
// in exactly the same words as a tree that writes none.
const insertOptsFloor = 20

func TestNoInsertOptsNamesItsOwnQueue(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	paths, err := goFilesUnder(filepath.Join(root, "internal"))
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	named, seen := 0, 0
	for _, path := range paths {
		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", rel, err)
		}
		literals, queueLines := insertOptsQueueLines(file)
		seen += literals
		for _, line := range queueLines {
			named++
			t.Errorf("%s:%d writes river.InsertOpts.Queue. Wrap the options in jobs.QueuedAs[Args] "+
				"(or jobs.QueuedAsKind for a kind known only as a string) and delete the field — "+
				"api/jobs.yaml is what names the queue, and a second spelling here is the one that "+
				"silently wins.", rel, line)
		}
	}
	// Under-recognition is the one way this must not break: a walk that stops
	// recognising the literal reports a clean tree while every insert names its
	// own queue again.
	if seen < insertOptsFloor {
		t.Fatalf("the walk found %d river.InsertOpts literal(s) and expects at least %d: "+
			"this scan no longer sees its own subject", seen, insertOptsFloor)
	}
}

// insertOptsQueueLines answers how many river.InsertOpts literals this file
// builds, and the line of every `Queue:` field among them. The TYPE is what it
// matches on: river.QueueConfig carries a Queue-keyed map and jobhealth reads a
// queue off a spec, and neither is an insert deciding where its row lands.
func insertOptsQueueLines(file *ast.File) (literals int, lines []int) {
	ast.Inspect(file, func(node ast.Node) bool {
		lit, isLit := node.(*ast.CompositeLit)
		if !isLit || !isRiverInsertOpts(lit.Type) {
			return true
		}
		literals++
		for _, elt := range lit.Elts {
			kv, isKV := elt.(*ast.KeyValueExpr)
			if !isKV {
				continue
			}
			if key, isIdent := kv.Key.(*ast.Ident); isIdent && key.Name == "Queue" {
				lines = append(lines, gatekit.SourceFileSet().Position(kv.Pos()).Line)
			}
		}
		return true
	})
	return literals, lines
}

func isRiverInsertOpts(expr ast.Expr) bool {
	sel, isSel := expr.(*ast.SelectorExpr)
	if !isSel || sel.Sel.Name != "InsertOpts" {
		return false
	}
	pkg, isIdent := sel.X.(*ast.Ident)
	return isIdent && pkg.Name == "river"
}
