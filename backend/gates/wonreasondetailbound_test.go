// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The paperless-win detail's length bound is one number in three places.
// textLimits holds the deals constant to `api/crm.yaml`. This test holds the MCP
// tool schema to it, because an agent plans against that schema and an
// unforeseen refusal costs a whole turn.

import (
	"encoding/json"
	"go/ast"
	"go/token"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheWonReasonDetailBoundIsOneNumber(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var contract map[string]any
	if err := unmarshalYAML(raw, &contract); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	// The request schema, not Deal: Deal declares the field readOnly, and only
	// what a caller may send can be refused for its length.
	published := publishedMaxLength(t, contract,
		schemaPath("components", "schemas", "AdvanceDealRequest", "properties", "won_without_contract_detail"))
	if advertised := advertisedWonReasonDetailBound(t); advertised != published {
		t.Errorf("the advance_deal tool schema advertises maxLength %d and api/crm.yaml publishes %d — an agent plans against the tool schema, so it would spend a turn on a request the server refuses",
			advertised, published)
	}
}

// advertisedWonReasonDetailBound reads the tool schema the agents module writes
// into every deal-moving tool. It is a JSON fragment rather than a document —
// the properties are spliced into a larger object — so it is closed here into
// the object it becomes before being parsed, which is the same reading the MCP
// client gets.
func advertisedWonReasonDetailBound(t *testing.T) int {
	t.Helper()
	const declaration = "internal/modules/agents/tools_advance.go"
	file, err := gatekit.ParseFile(declaration, 0)
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
