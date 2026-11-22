// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// The lawful-basis list is closed and names no country.
//
// The pack that grants a basis supplies its legal citation, so a value that
// names a market belongs in that pack and not in this list.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"testing"
)

// countryPrefixed matches a value that starts with a two-letter code and an
// underscore, such as `de_` or `vn_`.
var countryPrefixed = regexp.MustCompile(`^[a-z]{2}_`)

// basisValues reads the string value of every constant declared with the type
// Basis, so a constant added later is checked without anyone listing it.
func basisValues(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "decision.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing decision.go: %v", err)
	}
	var values []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Values) != 1 {
				continue
			}
			if ident, ok := vs.Type.(*ast.Ident); !ok || ident.Name != "Basis" {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok {
				t.Fatalf("Basis constant %s is not a string literal", vs.Names[0].Name)
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("reading Basis constant %s: %v", vs.Names[0].Name, err)
			}
			values = append(values, value)
		}
	}
	return values
}

func TestNoBasisConstantNamesACountry(t *testing.T) {
	t.Parallel()
	values := basisValues(t)
	if len(values) < 8 {
		t.Fatalf("read %d Basis constants, want at least the 8 the list holds: the reader missed some", len(values))
	}
	for _, value := range values {
		if countryPrefixed.MatchString(value) {
			t.Errorf("Basis %q starts with a country code; core names no market, so the pack that grants it supplies the citation", value)
		}
	}
}

func TestTheCountryPrefixRuleSeesACountryValue(t *testing.T) {
	t.Parallel()
	if !countryPrefixed.MatchString("vn_subject_agreement") {
		t.Fatal("the rule does not match a value that starts with a country code")
	}
	if countryPrefixed.MatchString("precontract_request") {
		t.Fatal("the rule matches a neutral value")
	}
}
