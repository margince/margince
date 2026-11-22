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
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

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

// quotedTextValues returns the 'x'::text literals of a catalog line.
var quotedTextValues = regexp.MustCompile(`'([a-z_]+)'::text`)

// catalogCheckValues returns the value list of the named CHECK in the committed
// schema catalog, without the NULL arm.
func catalogCheckValues(t *testing.T, constraint string) map[string]bool {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the schema catalog: %v", err)
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.Contains(line, "."+constraint+" CHECK") {
			continue
		}
		values := map[string]bool{}
		for _, m := range quotedTextValues.FindAllStringSubmatch(line, -1) {
			values[m[1]] = true
		}
		return values
	}
	t.Fatalf("the schema catalog has no CHECK named %s", constraint)
	return nil
}

// apiBasisEnums returns every enum in crm.yaml that lists existing_customer_exception,
// which only a basis list does.
func apiBasisEnums(t *testing.T) []map[string]bool {
	t.Helper()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading crm.yaml: %v", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parsing crm.yaml: %v", err)
	}
	var found []map[string]bool
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == "enum" && n.Content[i+1].Kind == yaml.SequenceNode {
					values := map[string]bool{}
					for _, v := range n.Content[i+1].Content {
						values[v.Value] = true
					}
					if values["existing_customer_exception"] {
						found = append(found, values)
					}
				}
			}
		}
		for _, c := range n.Content {
			walk(c)
		}
	}
	walk(&root)
	return found
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// TestNoCommunicationBasisNamesACountry holds the vocabulary in all four places
// that own it. The relations are: the API enum is the Go set plus "none" (no
// basis recorded); the decision CHECK is the Go set; the basis-kind CHECK is the
// Go set without "consent", which lives in the consent tables.
func TestNoCommunicationBasisNamesACountry(t *testing.T) {
	t.Parallel()
	goValues := map[string]bool{}
	constants := basisConstantValues(t, commsauthzDecisionSource)
	for _, v := range constants {
		goValues[v] = true
	}
	decision := catalogCheckValues(t, "communication_decision_basis")
	kind := catalogCheckValues(t, "communication_basis_kind")
	enums := apiBasisEnums(t)
	codes := map[string]bool{}
	for _, code := range contacts.AssignedCountryCodes() {
		codes[code] = true
	}
	if _, ok := constants["BasisConsent"]; !ok || len(goValues) < 8 || len(codes) < 200 ||
		len(decision) < len(goValues) || len(kind) < len(goValues)-1 || len(enums) == 0 {
		t.Fatalf("short census: %d Go value(s), %d decision CHECK value(s), %d kind CHECK value(s), %d API enum(s), %d country code(s); a gate that reads less than exists agrees with everything",
			len(goValues), len(decision), len(kind), len(enums), len(codes))
	}

	wantKind := map[string]bool{}
	for v := range goValues {
		if v != "consent" {
			wantKind[v] = true
		}
	}
	wantEnum := map[string]bool{"none": true}
	for v := range goValues {
		wantEnum[v] = true
	}
	if !sameSet(decision, goValues) {
		t.Errorf("communication_decision_basis CHECK %v differs from the Go constants %v", decision, goValues)
	}
	if !sameSet(kind, wantKind) {
		t.Errorf("communication_basis_kind CHECK %v differs from the Go constants without consent %v", kind, wantKind)
	}
	for _, e := range enums {
		if !sameSet(e, wantEnum) {
			t.Errorf("a crm.yaml basis enum %v differs from the Go constants plus none %v", e, wantEnum)
		}
	}

	every := map[string]bool{}
	for _, set := range append([]map[string]bool{goValues, decision, kind}, enums...) {
		for v := range set {
			every[v] = true
		}
	}
	for value := range every {
		for _, part := range strings.Split(value, "_") {
			if codes[strings.ToUpper(part)] {
				t.Errorf("basis value %q contains the country code %q: core names no market, and the pack that grants a basis supplies the legal citation", value, strings.ToUpper(part))
			}
		}
	}
}
