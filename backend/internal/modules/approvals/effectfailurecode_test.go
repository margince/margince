// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEachFailureCodeStoresItsPinnedSentence(t *testing.T) {
	pinned := []effectFailure{
		{"approval_lapsed", "Approval expired before execution. Nothing changed. Request a new proposal to try again."},
		{"effect_not_run", "this was approved, but the work it released did not run"},
		{"effect_unconfirmed", "this was approved and sent on, but the system it went to never confirmed it, " +
			"so it may have happened. Check the record before you ask for it again"},
		{"follow_up_failed", "this was approved and the change was made, but the automation that asked for it " +
			"could not be marked finished"},
		{"window_not_widened", "the agent's window could not be widened, so the approval has not taken effect"},
	}
	if !slices.Equal(effectFailures, pinned) {
		t.Fatalf("failures = %q, want %q: a client words each code, so a code and its sentence change together", effectFailures, pinned)
	}
	for _, failure := range pinned {
		stored := failure.sentence
		if code := effectFailureCode(&stored); code == nil || *code != failure.code {
			t.Errorf("stored %q reads back as %v, want %s", stored, code, failure.code)
		}
	}
	unknown := "a sentence no failure stores"
	if effectFailureCode(&unknown) != nil || effectFailureCode(nil) != nil {
		t.Error("an unknown or absent sentence must carry no code")
	}
}

func TestAFailedEffectIsToldByItsCause(t *testing.T) {
	for err, want := range map[error]effectFailure{
		ErrEffectFollowUpFailed:     failureFollowUpFailed,
		ErrEffectUnconfirmed:        failureUnconfirmed,
		errors.New("executor down"): failureNotRun,
	} {
		if got := effectFailureOf(err); got != want {
			t.Errorf("effectFailureOf(%v) = %s, want %s", err, got.code, want.code)
		}
	}
}

func TestEveryDeclaredFailureIsOneTheReaderKnows(t *testing.T) {
	declared, listed := map[string]bool{}, map[string]bool{}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range value.Names {
					if i >= len(value.Values) {
						continue
					}
					lit, isLit := value.Values[i].(*ast.CompositeLit)
					if !isLit {
						continue
					}
					if ident, isIdent := lit.Type.(*ast.Ident); isIdent && ident.Name == "effectFailure" {
						declared[name.Name] = true
					}
					if name.Name == "effectFailures" {
						for _, elt := range lit.Elts {
							if ident, isIdent := elt.(*ast.Ident); isIdent {
								listed[ident.Name] = true
							}
						}
					}
				}
			}
		}
	}
	if len(declared) == 0 || len(listed) == 0 {
		t.Fatalf("read %d declared failures and %d listed ones; the reader of this package's source has stopped seeing them", len(declared), len(listed))
	}
	for name := range declared {
		if !listed[name] {
			t.Errorf("%s is a failure a writer can record, and effectFailures leaves it out, so its sentence reads back with no code", name)
		}
	}
}
