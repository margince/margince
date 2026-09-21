// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The paperless-win detail's length bound is ONE number, in three places that
// each need it.
//
// `api/crm.yaml` publishes it, so every generated client truncates to it. The
// deals module applies it, because the generated server does not enforce a
// string length and the column is plain `text` — without that the published
// bound is a promise the product does not keep. The MCP tool schema advertises
// it, because an agent plans against the tool schema and a refusal it could not
// have predicted costs a whole turn.
//
// Three readers agreeing with each other is not the property that matters here:
// they can only be compared to the CONTRACT, which is the one of them a client
// outside this repository can see. So each is read separately and each is
// compared to `api/crm.yaml`, and the gate fails whichever one moves.
//
// The failure it exists for is silent on both sides. Raise the contract alone
// and the server refuses a value the schema says is fine; raise the server
// alone and a client goes on truncating to a bound nobody applies.

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTheWonReasonDetailBoundIsOneNumber(t *testing.T) {
	t.Parallel()

	published := publishedWonReasonDetailBound(t)
	if published <= 0 {
		t.Fatalf("api/crm.yaml publishes maxLength %d for won_without_contract_detail — a non-positive bound is not one this gate can hold anything to", published)
	}

	if applied := appliedWonReasonDetailBound(t); applied != published {
		t.Errorf("deals applies a bound of %d and api/crm.yaml publishes %d — one of them refuses a value the other calls valid, and nothing else says which",
			applied, published)
	}
	if advertised := advertisedWonReasonDetailBound(t); advertised != published {
		t.Errorf("the advance_deal tool schema advertises maxLength %d and api/crm.yaml publishes %d — an agent plans against the tool schema, so it would spend a turn on a request the server refuses",
			advertised, published)
	}
}

// publishedWonReasonDetailBound is the contract's own number, read from the
// schema node rather than from a generated artifact: `maxLength` is a
// validation keyword the Go generator does not carry into a type, so the YAML
// is the only place it survives.
func publishedWonReasonDetailBound(t *testing.T) int {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					MaxLength int `yaml:"maxLength"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	// The REQUEST schema, not Deal. Deal declares the field readOnly, which
	// is the value coming back and needs no bound; what a caller may send is
	// the only side a length can refuse, and the only side the server applies
	// one to.
	request, ok := doc.Components.Schemas["AdvanceDealRequest"]
	if !ok {
		t.Fatal("api/crm.yaml declares no AdvanceDealRequest schema — this gate's subject moved and it is now asking about nothing")
	}
	property, ok := request.Properties["won_without_contract_detail"]
	if !ok {
		t.Fatal("AdvanceDealRequest declares no won_without_contract_detail — retire this gate, or repoint it at whatever replaced the field")
	}
	return property.MaxLength
}

// appliedWonReasonDetailBound reads the module's constant from source rather
// than importing it. The constant is unexported on purpose — a length nobody
// outside deals should be quoting — and exporting it to satisfy a gate would
// widen the surface to make the check convenient, which is backwards.
func appliedWonReasonDetailBound(t *testing.T) int {
	t.Helper()
	const declaration = "internal/modules/deals/win_evidence.go"
	file, err := parser.ParseFile(token.NewFileSet(), declaration, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", declaration, err)
	}
	for _, decl := range file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != "maxWonReasonDetail" {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.INT {
				t.Fatalf("maxWonReasonDetail is not an integer literal — this gate can only read one, so give it one or teach it the new shape")
			}
			n, err := strconv.Atoi(literal.Value)
			if err != nil {
				t.Fatalf("maxWonReasonDetail = %q: %v", literal.Value, err)
			}
			return n
		}
	}
	t.Fatalf("%s declares no maxWonReasonDetail — the bound the contract publishes is applied by nothing, so the server takes whatever a caller sends", declaration)
	return 0
}

// advertisedWonReasonDetailBound reads the tool schema the agents module writes
// into every deal-moving tool. It is a JSON fragment rather than a document —
// the properties are spliced into a larger object — so it is closed here into
// the object it becomes before being parsed, which is the same reading the MCP
// client gets.
func advertisedWonReasonDetailBound(t *testing.T) int {
	t.Helper()
	const declaration = "internal/modules/agents/tools_advance.go"
	file, err := parser.ParseFile(token.NewFileSet(), declaration, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", declaration, err)
	}
	fragment := constStringValue(t, file, "winEvidenceProperties", declaration)

	var properties map[string]struct {
		MaxLength int `json:"maxLength"`
	}
	if err := json.Unmarshal([]byte("{\"_\":null"+fragment+"}"), &properties); err != nil {
		t.Fatalf("winEvidenceProperties does not close into a JSON object: %v — the tool schema it is spliced into would be malformed too", err)
	}
	property, ok := properties["won_without_contract_detail"]
	if !ok {
		t.Fatal("winEvidenceProperties advertises no won_without_contract_detail — an agent cannot see the bound at all, so it learns it by being refused")
	}
	return property.MaxLength
}

// constStringValue is the unquoted value of a named string constant.
func constStringValue(t *testing.T, file *ast.File, name, where string) string {
	t.Helper()
	for _, decl := range file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != name {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				t.Fatalf("%s in %s is not a string literal", name, where)
			}
			unquoted, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatalf("unquoting %s in %s: %v", name, where, err)
			}
			return unquoted
		}
	}
	t.Fatalf("%s declares no %s", where, name)
	return ""
}
