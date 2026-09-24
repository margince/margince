// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Every sentence the receipt emits has a word for it, in the client that draws
// it and in the catalog that translates it.
//
// GET /magic sends a KEY, never a sentence: the product ships three languages
// and prose composed in Go reaches a German reader in English. That decision
// puts the two halves of one invariant on opposite sides of a wire, with
// nothing between them — the backend mints `magic.action.advance_stage` as a
// string literal, and whether any catalog carries that key is a fact no
// compiler on either side can see.
//
// It went the way that always goes. Nine keys shipped unresolvable: every line
// the lanes could draw named a word that existed nowhere, and nothing failed,
// because a missing translation renders as its own key and only a reader
// notices.
//
// THE STALE DIRECTION IS THE OTHER HALF. A key retired from the Go vocabulary
// leaves a catalog entry behind that reads exactly like coverage, and the next
// author greps, finds it, and stops looking. Both directions fail here.
//
// LOCALE PARITY IS NOT THIS GATE'S BUSINESS, deliberately. That de.ts and vi.ts
// carry the same keys as en.ts is held by frontend/src/i18n/i18n.test.ts and by
// the compiler, through catalogs' own Record type. Restating it here would be a
// second writer of somebody else's invariant, and the two would drift.

import (
	"go/ast"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	magicKeyRegistry = "../frontend/src/screens/magic.keys.ts"
	magicKeyCatalog  = "../frontend/src/i18n/en.ts"
	// magicKeyFloor guards against a vacuous pass. The lanes mint around thirty
	// keys today; the floor sits low enough that retiring a vocabulary does not
	// drag it along, and high enough that a walk resolving nothing is reported
	// rather than read as three sides that happen to agree about nothing.
	magicKeyFloor = 20
)

// magicKeyPrefixes are the two families the receipt sends. A key outside them
// is ordinary screen copy, which the orphan gate in i18n.test.ts already holds.
var magicKeyPrefixes = []string{"magic.action.", "magic.consequence."}

// magicKeyLiteral matches a quoted key in TypeScript. Both families in one
// pattern, so a family added on one side cannot be read as absent on the other.
var magicKeyLiteral = regexp.MustCompile(`["'` + "`" + `](magic\.(?:action|consequence)\.[A-Za-z0-9_]+)["'` + "`" + `]`)

// TestEverySentenceTheReceiptEmitsHasAWord holds the three sides together.
func TestEverySentenceTheReceiptEmitsHasAWord(t *testing.T) {
	t.Parallel()

	emitted := magicKeysEmitted(t)
	if len(emitted) < magicKeyFloor {
		t.Fatalf("resolved only %d magic sentence key(s) from the Go vocabulary, expected at least %d — a walk this short would agree with any catalog",
			len(emitted), magicKeyFloor)
	}
	registry := magicKeysIn(t, magicKeyRegistry)
	catalog := magicKeysIn(t, magicKeyCatalog)

	for _, key := range absentFrom(emitted, registry) {
		t.Errorf("the receipt emits %q and %s does not carry it: the client cannot narrow the key, so the line draws without its sentence",
			key, magicKeyRegistry)
	}
	for _, key := range absentFrom(registry, emitted) {
		t.Errorf("%s carries %q and no lane emits it: a retired key that still reads as coverage",
			magicKeyRegistry, key)
	}
	for _, key := range absentFrom(emitted, catalog) {
		t.Errorf("the receipt emits %q and %s has no word for it: the reader is shown the key itself",
			key, magicKeyCatalog)
	}
	for _, key := range absentFrom(catalog, emitted) {
		t.Errorf("%s translates %q and no lane emits it: a retired key that still reads as coverage",
			magicKeyCatalog, key)
	}
}

// magicKeysEmitted reads the vocabulary out of the package that owns it.
//
// The Scope sweep is what makes the single root a claim rather than an
// assumption: a key minted anywhere else in the module is reported as outside
// the gate's reach instead of going unseen, which is the one way a census fails
// — it reads a smaller tree, says PASS, and has no failing assertion to notice.
func magicKeysEmitted(t *testing.T) map[string]bool {
	t.Helper()
	scope := gatekit.Scope{
		Roots:   []string{"internal/compose/magic"},
		Subject: fileMintsAMagicKey,
	}
	keys := map[string]bool{}
	for _, parsed := range scope.Files(t) {
		if strings.HasSuffix(parsed.Path, "_test.go") {
			continue
		}
		for _, key := range magicKeysWithin(parsed.File) {
			keys[key] = true
		}
	}
	return keys
}

// fileMintsAMagicKey is the sweep's predicate, asked the same inside the root
// and outside it.
func fileMintsAMagicKey(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	return len(magicKeysWithin(file)) > 0
}

// magicKeysWithin collects the key literals one file spells.
func magicKeysWithin(file *ast.File) []string {
	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind.String() != "STRING" {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		for _, prefix := range magicKeyPrefixes {
			if strings.HasPrefix(value, prefix) && len(value) > len(prefix) {
				found = append(found, value)
			}
		}
		return true
	})
	return found
}

// magicKeysIn reads one side of the wire.
//
// Prose is stripped first: a key NAMED in a comment is not a key the client
// carries, and counting one would fail this gate over a sentence somebody wrote
// about the vocabulary rather than over the vocabulary. Stripping can only ever
// remove a key that was never real, so it cannot make this side read short.
func magicKeysIn(t *testing.T, path string) map[string]bool {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	keys := map[string]bool{}
	for _, match := range magicKeyLiteral.FindAllStringSubmatch(withoutProse(string(source)), -1) {
		keys[match[1]] = true
	}
	return keys
}

// magicKeyProse matches a line comment and a block comment in TypeScript.
var magicKeyProse = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

// withoutProse blanks the comments in a TypeScript source.
func withoutProse(source string) string {
	return magicKeyProse.ReplaceAllString(source, "")
}

// absentFrom names what the first side has and the second does not, in a
// deterministic order so a failing run reads the same twice.
func absentFrom(have, want map[string]bool) []string {
	var absent []string
	for key := range have {
		if !want[key] {
			absent = append(absent, key)
		}
	}
	sort.Strings(absent)
	return absent
}
