// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A relationship carrying a person is written under that person's row lock.
//
// archivePersonRows archives the person AND sweeps their relationships in one
// transaction. A writer that inserted a relationship without holding the person
// row could commit between the archive's own probe and its sweep, leaving a
// LIVE relationship pointing at an ARCHIVED person — the orphan the "block
// rather than orphan" rule exists to prevent (issue #1625).
//
// FOUR writers had to be found by hand to fix it, in four files, and the issue
// itself named one. That is the shape this census replaces: the next writer is
// the one nobody remembers, and the failure it causes is a row that looks fine
// in every sequential test.
//
// TWO SPELLINGS OF THE LOCK, because two shapes of writer need it:
//
//   - a writer that names ONE person calls lockPersonForAttach before its
//     insert, and LiveOnly is both the lock and the liveness check.
//   - a SET-BASED writer has no id to lock before the select that finds it, so
//     it takes the lock inside the statement — `FOR UPDATE OF p` over the
//     person rows the insert is about to attach.
//
// Either satisfies this. What does not is an insert naming person_id with
// neither, which is the state every one of them was in.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The two ways a writer takes the lock, one per shape.
const (
	singlePersonLock = "lockPersonForAttach"
	setBasedLock     = "FOR UPDATE OF p"
)

func TestEveryRelationshipCarryingAPersonIsWrittenUnderItsLock(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "people")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the people module: %v", err)
	}

	writers := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, fn := range personRelationshipWriters(file) {
			writers++
			if !fn.locked {
				t.Errorf("%s: %s inserts a relationship naming person_id and takes no person lock.\n"+
					"\tAn archive in flight can then commit between its own probe and its sweep, "+
					"leaving a live relationship on an archived person — which every sequential "+
					"test reads as fine.\n"+
					"\tCall %s before the insert, or take the lock in the statement with `%s` "+
					"if the people are found by the select rather than named.",
					name, fn.name, singlePersonLock, setBasedLock)
			}
		}
	}
	// Four of these exist. A census that found none is one whose walk broke,
	// and it would report the same green as a tree where every writer locks.
	if writers < 4 {
		t.Fatalf("found %d writer(s) inserting a relationship with a person_id, and there are at "+
			"least four — the walk is broken, or the statements changed shape", writers)
	}
}

type relationshipWriter struct {
	name   string
	locked bool
}

// personRelationshipWriters finds each function whose body contains an INSERT
// into relationship naming person_id, and whether that same function takes the
// lock in either of its two spellings.
func personRelationshipWriters(file *ast.File) []relationshipWriter {
	var found []relationshipWriter
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		inserts, statementLock := false, false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			sql := gatekit.TextOf(lit)
			if strings.Contains(sql, "INSERT INTO relationship") && strings.Contains(sql, "person_id") {
				inserts = true
			}
			if strings.Contains(sql, setBasedLock) {
				statementLock = true
			}
			return true
		})
		if !inserts {
			continue
		}
		found = append(found, relationshipWriter{
			name:   fn.Name.Name,
			locked: statementLock || callsLock(fn.Body),
		})
	}
	return found
}

func callsLock(body *ast.BlockStmt) bool {
	called := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == singlePersonLock {
			called = true
		}
		return true
	})
	return called
}
