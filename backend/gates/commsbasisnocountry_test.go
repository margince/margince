// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The communication basis vocabulary is closed and names no market. A pack
// supplies the legal citation for a basis; core never learns a jurisdiction one
// value at a time. The constants are read from commsauthz itself and the codes
// from the ISO 3166-1 list the address check already holds, so neither side is
// a second list kept here.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const commsauthzDecisionSource = "internal/shared/ports/commsauthz/decision.go"

// basisConstantValues returns the string value of every constant declared with
// type Basis, keyed by constant name.
func basisConstantValues(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := gatekit.ParseFile(path, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	values := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			if typ, ok := vs.Type.(*ast.Ident); !ok || typ.Name != "Basis" {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: unquoting %s: %v", path, lit.Value, err)
			}
			values[vs.Names[0].Name] = value
		}
	}
	return values
}

func TestNoCommunicationBasisNamesACountry(t *testing.T) {
	t.Parallel()
	bases := basisConstantValues(t, commsauthzDecisionSource)
	codes := map[string]bool{}
	for _, code := range contacts.AssignedCountryCodes() {
		codes[code] = true
	}
	// BasisConsent and BasisContract are two of the eight the vocabulary holds
	// today; fewer found means the scan stopped reading the declarations.
	if _, ok := bases["BasisConsent"]; !ok || len(bases) < 8 || len(codes) < 200 {
		t.Fatalf("found %d basis constant(s) and %d country code(s); a gate that reads a short census agrees with everything", len(bases), len(codes))
	}
	for name, value := range bases {
		parts := strings.Split(value, "_")
		for _, part := range []string{parts[0], parts[len(parts)-1]} {
			if len(parts) > 1 && codes[strings.ToUpper(part)] {
				t.Errorf("%s = %q starts or ends with the country code %q: core names no market, and the pack that grants a basis supplies the legal citation", name, value, strings.ToUpper(part))
			}
		}
	}
}
