// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storedobjects

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// An undeclared kind is refused before anything is written.
//
// It reaches the sweep as a key nobody owns, and the sweep cannot adjudicate one — so
// a typo in a single writer strands every other writer's orphans. Refused here, the
// typo costs that writer its upload instead.
//
// A nil handle is what proves the order: the refusal arrives without a database, so
// the check runs before the insert rather than beside it. The other direction — every
// declared kind passing — asks the predicate instead, because passing the guard means
// proceeding to a database this test does not have.
func TestAnUndeclaredKindIsRefusedBeforeTheDatabase(t *testing.T) {
	t.Parallel()
	err := NewLedger(nil).Record(context.Background(), Kind("attachmnet"), "ws/attachmnet/typo")
	if err == nil {
		t.Fatal("an undeclared kind was accepted, and with no database behind it the insert " +
			"cannot have been what refused")
	}
	if !strings.Contains(err.Error(), "not a declared kind") {
		t.Errorf("the refusal says %q, which does not name the reason a writer has to act on", err)
	}
}

// Every declared kind passes that check, so the vocabulary and the guard agree.
//
// Without this the guard could refuse a kind the constants declare, and the writer
// that declared it correctly would be the one that breaks.
func TestEveryDeclaredKindPassesTheGuard(t *testing.T) {
	t.Parallel()
	for _, kind := range AllKinds() {
		if !declared(kind) {
			t.Errorf("%q is in AllKinds and the guard refuses it, so the writer that declared "+
				"it correctly is the one that breaks", kind)
		}
	}
}

// AllKinds names every kind, with no duplicates.
//
// It is what the sweep's owner map is held to, so a kind missing from it is a kind
// nothing requires an owner for — and a duplicate makes that check pass twice for one
// entry while another goes unasked.
func TestTheKindVocabularyIsCompleteAndDistinct(t *testing.T) {
	t.Parallel()
	seen := map[Kind]bool{}
	for _, kind := range AllKinds() {
		if seen[kind] {
			t.Errorf("%q appears twice in AllKinds", kind)
		}
		if strings.TrimSpace(string(kind)) == "" {
			t.Error("AllKinds contains an empty kind, which no writer can declare")
		}
		seen[kind] = true
	}
	for _, kind := range declaredKindConstants(t) {
		if !seen[kind] {
			t.Errorf("%q is a declared constant and AllKinds omits it, so nothing holds the "+
				"sweep to having an owner for it", kind)
		}
	}
}

// RecordTx refuses a kind nobody declared before it writes, for the reason
// Record does: an unowned key would stop the reaper's pass for the workspace.
func TestRecordTxRefusesAnUndeclaredKind(t *testing.T) {
	if err := RecordTx(context.Background(), nil, Kind("nobody's"), "ws/x/1"); err == nil {
		t.Fatal("an undeclared kind was accepted")
	}
}

// declaredKindConstants reads every Kind constant this package declares, so a new one is
// held to AllKinds without anybody adding it to a second list.
func declaredKindConstants(t *testing.T) []Kind {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "storedobjects.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing the kind vocabulary: %v", err)
	}
	var kinds []Kind
	for _, decl := range file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Values) != len(value.Names) {
				continue
			}
			if typed, ok := value.Type.(*ast.Ident); !ok || typed.Name != "Kind" {
				continue
			}
			for i, literal := range value.Values {
				text, ok := literal.(*ast.BasicLit)
				if !ok {
					t.Fatalf("Kind constant %s is not a string literal, so this test cannot read it", value.Names[i])
				}
				unquoted, err := strconv.Unquote(text.Value)
				if err != nil {
					t.Fatalf("Kind constant %s: %v", value.Names[i], err)
				}
				kinds = append(kinds, Kind(unquoted))
			}
		}
	}
	if len(kinds) < 2 {
		t.Fatalf("read %d Kind constants, so the walk is reading the wrong shape", len(kinds))
	}
	return kinds
}
