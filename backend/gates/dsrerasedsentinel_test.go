// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The word an erasure leaves in place of a request's answer has two writers in
// two modules, and a module cannot import its sibling. consent names it once,
// and its patch guard reads it to tell a repeated erasure from a rewrite. The
// privacy engine writes the same word into the SQL that retires the subject's
// other cases. If they diverged, a retired case would hold a word the guard no
// longer knows, and a rewrite of a closed request's answer would pass.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

const (
	consentSentinelFile = "internal/modules/consent/dsr.go"
	privacySentinelFile = "internal/modules/privacy/erasure_consent.go"
)

func TestBothErasureWritersLeaveTheSameWordInPlaceOfTheAnswer(t *testing.T) {
	t.Parallel()
	consentWord := erasedResolutionConst(t)
	source, err := os.ReadFile(privacySentinelFile)
	if err != nil {
		t.Fatalf("reading the privacy erasure writer: %v", err)
	}
	statement := "resolution = CASE WHEN resolution IS NULL THEN NULL ELSE '" + consentWord + "' END"
	if !strings.Contains(string(source), statement) {
		t.Errorf("%s no longer writes %q in place of the answer, which is what consent's erasedResolution says an erasure leaves.\n"+
			"\tChange both or neither: consent's guard reads that word to tell a repeated erasure from a rewrite.",
			privacySentinelFile, consentWord)
	}
}

// erasedResolutionConst reads consent's constant from its source, so the gate
// holds the writer to the owner instead of keeping its own copy of the word.
func erasedResolutionConst(t *testing.T) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), consentSentinelFile, nil, 0)
	if err != nil {
		t.Fatalf("parsing consent's request file: %v", err)
	}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) != 1 || value.Names[0].Name != "erasedResolution" || len(value.Values) != 1 {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.BasicLit)
			if !isLiteral {
				break
			}
			word, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("erasedResolution is not a string literal: %v", err)
			}
			return word
		}
	}
	t.Fatal("consent declares no erasedResolution constant, so this gate would agree with any privacy writer")
	return ""
}
