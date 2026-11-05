// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The schema refuses the workspace values boot refuses, so an editor flags
// them before a boot does.
func TestTheSchemaRefusesTheWorkspaceValuesBootRefuses(t *testing.T) {
	routing, err := routingDefs()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := schemaJSON(twoSettings(t), routing, fieldDocs{})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("margince.json", doc); err != nil {
		t.Fatal(err)
	}
	workspace, err := compiler.Compile("margince.json#/properties/workspace")
	if err != nil {
		t.Fatal(err)
	}
	for value, wantValid := range map[string]bool{
		`{"base_currency": "EUR"}`:  true,
		`{"base_currency": "EURO"}`: false,
		`{"base_currency": "eur"}`:  false,
		`{"base_language": "de"}`:   true,
		`{"base_language": "fr"}`:   false,
	} {
		instance, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(value)))
		if err != nil {
			t.Fatal(err)
		}
		if err := workspace.Validate(instance); (err == nil) != wantValid {
			t.Errorf("workspace %s: valid %v, want %v (%v)", value, err == nil, wantValid, err)
		}
	}
}
