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
//
// The placeholder that SQL writes over the subject is held the same way. Consent
// reads it to tell the screen the case names no one.

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	consentSentinelFile = "internal/modules/consent/dsr.go"
	privacySentinelFile = "internal/modules/privacy/erasure_consent.go"
)

func TestBothErasureWritersLeaveTheSameWordInPlaceOfTheAnswer(t *testing.T) {
	t.Parallel()
	consentWord := consentSentinelConst(t, "erasedResolution")
	statement := "resolution = CASE WHEN resolution IS NULL THEN NULL ELSE '" + consentWord + "' END"
	if !strings.Contains(privacySentinelSource(t), statement) {
		t.Errorf("%s no longer writes %q in place of the answer, which is what consent's erasedResolution says an erasure leaves.\n"+
			"\tChange both or neither: consent's guard reads that word to tell a repeated erasure from a rewrite.",
			privacySentinelFile, consentWord)
	}
}

func TestConsentKnowsThePlaceholderTheErasureWritesOverTheSubject(t *testing.T) {
	t.Parallel()
	prefix := consentSentinelConst(t, "erasedSubjectPrefix")
	statement := "SET subject_ref = '" + prefix + "' || id,"
	if !strings.Contains(privacySentinelSource(t), statement) {
		t.Errorf("%s no longer writes %q over a retired case's subject, which is what consent's erasedSubjectPrefix reads.\n"+
			"\tChange both or neither: the request list would show the placeholder as the subject's name.",
			privacySentinelFile, statement)
	}
}

func privacySentinelSource(t *testing.T) string {
	t.Helper()
	source, err := gatekit.ReadFile(privacySentinelFile)
	if err != nil {
		t.Fatalf("reading the privacy erasure writer: %v", err)
	}
	return string(source)
}

// consentSentinelConst reads one of consent's constants from its source, so the
// gate holds the writer to the owner instead of keeping its own copy.
func consentSentinelConst(t *testing.T, name string) string {
	t.Helper()
	file, err := gatekit.ParseFile(consentSentinelFile, 0)
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
			if !isValue || len(value.Names) != 1 || value.Names[0].Name != name || len(value.Values) != 1 {
				continue
			}
			literal, isLiteral := value.Values[0].(*ast.BasicLit)
			if !isLiteral {
				break
			}
			word, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("%s is not a string literal: %v", name, err)
			}
			return word
		}
	}
	t.Fatalf("consent declares no %s constant, so this gate would agree with any privacy writer", name)
	return ""
}
