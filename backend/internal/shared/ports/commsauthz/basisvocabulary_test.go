// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

// The lawful-basis list is closed and names no country.
//
// The pack that grants a basis supplies its legal citation. A value that names
// a market belongs in that pack.

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
	"testing"
)

// basisValues is the value of every constant whose type is Basis, in any file
// of the package. The type checker decides, so a constant that takes its type
// from the block it sits in is read too.
func basisValues(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing the package: %v", err)
	}
	var files []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files = append(files, file)
	}
	// Imports are not resolved: only the constants of this package are read, and
	// a failed import cannot change the type of one of them.
	conf := types.Config{Error: func(error) {}}
	pkg, _ := conf.Check("commsauthz", fset, files, nil)
	var values []string
	for _, name := range pkg.Scope().Names() {
		c, ok := pkg.Scope().Lookup(name).(*types.Const)
		if !ok || c.Type().String() != "commsauthz.Basis" {
			continue
		}
		values = append(values, constant.StringVal(c.Val()))
	}
	return values
}

// namesACountry reports the word of a basis value that is a country code at
// either end, or a country name or adjective anywhere.
func namesACountry(value string) (string, bool) {
	words := strings.Split(value, "_")
	for i, word := range words {
		if countryWords[word] {
			return word, true
		}
		if (i == 0 || i == len(words)-1) && isoCountryCodes[word] {
			return word, true
		}
	}
	return "", false
}

func TestNoBasisConstantNamesACountry(t *testing.T) {
	t.Parallel()
	values := basisValues(t)
	if len(values) < 8 {
		t.Fatalf("read %d Basis constants, want at least the 8 the list holds: the reader missed some", len(values))
	}
	for _, value := range values {
		if word, bad := namesACountry(value); bad {
			t.Errorf("Basis %q holds %q, which names a country; core names no market, so the pack that grants it supplies the citation", value, word)
		}
	}
}

func TestTheCountryRuleSeesWhatItMustAndSparesTheRest(t *testing.T) {
	t.Parallel()
	for _, value := range []string{"vn_subject_agreement", "vietnam_subject_agreement", "agreement_de", "german_agreement"} {
		if _, bad := namesACountry(value); !bad {
			t.Errorf("%q names a country and the rule missed it", value)
		}
	}
	for _, value := range []string{"precontract_request", "vital_or_security_interest", "existing_customer_exception"} {
		if word, bad := namesACountry(value); bad {
			t.Errorf("%q is neutral and the rule read %q as a country", value, word)
		}
	}
}
