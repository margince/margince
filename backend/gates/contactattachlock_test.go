// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// A relationship carrying a contact is written under that contact's row lock.
//
// archiveContactRows archives the contact AND sweeps their relationships in one
// transaction. A writer that inserted a relationship without holding the contact
// row could commit between the archive's own probe and its sweep, leaving a
// LIVE relationship pointing at an ARCHIVED contact — the orphan the "block
// rather than orphan" rule exists to prevent (issue #1625).
//
// FOUR writers had to be found by hand to fix it, in four files, and the issue
// itself named one. That is the shape this census replaces: the next writer is
// the one nobody remembers, and the failure it causes is a row that looks fine
// in every sequential test.
//
// TWO SPELLINGS OF THE LOCK, because two shapes of writer need it:
//
//   - a writer that names ONE contact calls lockContactForAttach before its
//     insert, and LiveOnly is both the lock and the liveness check.
//   - a SET-BASED writer has no id to lock before the select that finds it, so
//     it takes the lock inside the statement — `FOR UPDATE OF p` over the
//     contact rows the insert is about to attach.
//
// Either satisfies this. What does not is an insert naming contact_id with
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
	singleContactLock = "lockContactForAttach"
	setBasedLock      = "FOR UPDATE OF p"
)

func TestEveryRelationshipCarryingAContactIsWrittenUnderItsLock(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "contacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the contacts module: %v", err)
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
		for _, fn := range contactRelationshipWriters(file) {
			writers++
			if !fn.locked {
				t.Errorf("%s: %s inserts a relationship naming contact_id and takes no contact lock.\n"+
					"\tAn archive in flight can then commit between its own probe and its sweep, "+
					"leaving a live relationship on an archived contact — which every sequential "+
					"test reads as fine.\n"+
					"\tCall %s before the insert, or take the lock in the statement with `%s` "+
					"if the contacts are found by the select rather than named.",
					name, fn.name, singleContactLock, setBasedLock)
			}
		}
	}
	// Four of these exist. A census that found none is one whose walk broke,
	// and it would report the same green as a tree where every writer locks.
	if writers < 4 {
		t.Fatalf("found %d writer(s) inserting a relationship with a contact_id, and there are at "+
			"least four — the walk is broken, or the statements changed shape", writers)
	}
}

type relationshipWriter struct {
	name   string
	locked bool
}

// contactRelationshipWriters finds each function whose body contains an INSERT
// into relationship naming contact_id, and whether it takes the lock in either
// of its two spellings.
//
// ONE hop, and only into a helper in the same file. A writer that reads the
// contacts it is about through a named helper is still locking them — the
// candidate read and the insert are one transaction either way — and refusing
// that would push the query back inline for no reason but this gate's reach.
// The hop stops there: a lock two calls away is one a reader of the writer
// cannot see, which is the same blindness the gate exists to close.
func contactRelationshipWriters(file *ast.File) []relationshipWriter {
	locking := lockingHelpers(file)
	var found []relationshipWriter
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		if !insertsContactRelationship(fn.Body) {
			continue
		}
		found = append(found, relationshipWriter{
			name:   fn.Name.Name,
			locked: takesLock(fn.Body, locking),
		})
	}
	return found
}

func insertsContactRelationship(body *ast.BlockStmt) bool {
	inserts := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		sql := gatekit.TextOf(lit)
		if strings.Contains(sql, "INSERT INTO relationship") && strings.Contains(sql, "contact_id") {
			inserts = true
		}
		return true
	})
	return inserts
}

// lockingHelpers names the functions in one file that take a contact lock
// themselves, so a writer calling one is recognised as locked.
func lockingHelpers(file *ast.File) map[string]bool {
	helpers := map[string]bool{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && takesLock(fn.Body, nil) {
			helpers[fn.Name.Name] = true
		}
	}
	return helpers
}

// takesLock reports whether a body locks the contacts it writes: in its own
// statement, by calling the single-contact helper, or by calling one of the
// same file's own locking helpers.
func takesLock(body *ast.BlockStmt, viaHelper map[string]bool) bool {
	locked := false
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind == token.STRING && strings.Contains(gatekit.TextOf(node), setBasedLock) {
				locked = true
			}
		case *ast.CallExpr:
			ident, ok := node.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			if ident.Name == singleContactLock || viaHelper[ident.Name] {
				locked = true
			}
		}
		return true
	})
	return locked
}

// The reader is asserted against planted files, because the failure this gate
// must never have is under-recognition: a walk that stopped matching reports
// the same green as a tree where every writer locks.
//
// The hop is asserted in both directions — a writer whose helper locks is
// clean, and one whose helper does not is not — since a hop that accepted any
// call at all would pass the whole module.
func TestTheWriterReaderSeesTheLockAndOnlyTheLock(t *testing.T) {
	t.Parallel()

	const insert = "`INSERT INTO relationship (kind, contact_id) VALUES ($1, $2)`"
	for _, tc := range []struct {
		name       string
		source     string
		wantWriter bool
		wantLocked bool
	}{
		{
			name:   "no insert at all",
			source: "package p\nfunc f() { q(`SELECT 1 FROM relationship`) }",
		},
		{
			name:       "an insert with no lock",
			source:     "package p\nfunc f() { q(" + insert + ") }",
			wantWriter: true,
		},
		{
			name:       "the single-contact helper",
			source:     "package p\nfunc f() { lockContactForAttach(ctx, tx, id); q(" + insert + ") }",
			wantWriter: true, wantLocked: true,
		},
		{
			name:       "the lock in the statement",
			source:     "package p\nfunc f() { q(`SELECT id FROM contact p FOR UPDATE OF p`); q(" + insert + ") }",
			wantWriter: true, wantLocked: true,
		},
		{
			name: "a same-file helper that locks",
			source: "package p\nfunc h() { q(`SELECT id FROM contact p FOR UPDATE OF p`) }\n" +
				"func f() { h(); q(" + insert + ") }",
			wantWriter: true, wantLocked: true,
		},
		{
			name: "a same-file helper that does NOT lock",
			source: "package p\nfunc h() { q(`SELECT id FROM contact`) }\n" +
				"func f() { h(); q(" + insert + ") }",
			wantWriter: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "planted.go", tc.source, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parsing the planted source: %v", err)
			}
			writers := contactRelationshipWriters(file)
			if got := len(writers) > 0; got != tc.wantWriter {
				t.Fatalf("read %d writer(s), want a writer: %t", len(writers), tc.wantWriter)
			}
			if tc.wantWriter && writers[0].locked != tc.wantLocked {
				t.Errorf("locked = %t, want %t", writers[0].locked, tc.wantLocked)
			}
		})
	}
}
