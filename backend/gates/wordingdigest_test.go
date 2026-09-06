// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// One way to reduce a subject line and a body to a digest.
//
// Three places in consent ask the same question of the same two strings: the
// staging fingerprint, the transmit fingerprint, and the hash a published
// wording is identified by. They differ only in how they store the answer. A second
// hand-spelled sha256 over that pair is a second answer to one question, and
// the two drift silently — a fingerprint that stops matching reads as "the
// message changed", and a content hash that stops matching reads as "this
// wording was edited". Both are wrong, and neither says so.

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOneWordingDigest(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(moduleRoot(t), "internal", "modules", "consent")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the consent module: %v", err)
	}

	var examined int
	var offenders []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// The pin test hashes a RENDERED template, which is a different subject:
		// it proves the words in the catalog have not moved, not that two
		// writers agree. Named rather than pattern-matched so a second exempt
		// file has to be argued for.
		if name == "controllertemplates_test.go" || name == "textversion.go" {
			continue
		}
		examined++
		file := parseModuleFile(t, filepath.Join(dir, name))
		if hashesAPair(file) {
			offenders = append(offenders, name)
		}
	}
	// Under-recognition is the one way this must not fail: a walk that read
	// nothing would report PASS over a module full of second spellings.
	if examined < 10 {
		t.Fatalf("examined %d consent files, want at least 10: the census has stopped seeing "+
			"its subject", examined)
	}

	for _, name := range offenders {
		t.Errorf("%s spells sha256 over a subject-and-body pair itself. Call "+
			"consent.WordingDigest instead: the staging fingerprint, the transmit fingerprint "+
			"and a published wording's content hash are one question, and two spellings drift "+
			"until a fingerprint mismatch means nothing anybody can act on", name)
	}
}

// hashesAPair answers whether a file computes sha256 over a concatenation
// carrying the NUL separator this module joins a subject and a body with.
func hashesAPair(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if found {
			return false
		}
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Sum256" {
			return true
		}
		pkg, isPkg := sel.X.(*ast.Ident)
		if !isPkg || pkg.Name != "sha256" {
			return true
		}
		for _, arg := range call.Args {
			if joinsWithNUL(arg) {
				found = true
			}
		}
		return true
	})
	return found
}

// joinsWithNUL reports whether an expression concatenates with the "\x00"
// separator — the shape that makes a hash a SUBJECT-AND-BODY digest rather than
// a hash of something else.
func joinsWithNUL(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if found {
			return false
		}
		inner, isExpr := node.(ast.Expr)
		if !isExpr {
			return true
		}
		if text, isText := literalString(inner); isText && strings.Contains(text, "\x00") {
			found = true
			return false
		}
		return true
	})
	return found
}
