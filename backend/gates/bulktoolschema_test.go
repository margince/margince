// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// bulk_update_records advertises one schema for four modes. A client validates
// its arguments against it before calling, so the schema must require what each
// mode needs: a change names its record type, verb and items, and an undo names
// its batch. A schema that admitted {"mode":"undo"} would pass the client's own
// check and fail at the server.

import (
	"bytes"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/margince/margince/backend/internal/modules/agents"
)

func compiledBulkToolSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	registry := agents.NewRegistry(nil, nil)
	agents.RegisterBulkTool(registry, nil)
	spec, ok := registry.Spec("bulk_update_records")
	if !ok {
		t.Fatal("bulk_update_records is not registered")
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(spec.InputSchema))
	if err != nil {
		t.Fatalf("the input schema is not JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("bulk.json", doc); err != nil {
		t.Fatalf("adding the schema: %v", err)
	}
	schema, err := c.Compile("bulk.json")
	if err != nil {
		t.Fatalf("compiling the schema: %v", err)
	}
	return schema
}

func TestTheBulkToolSchemaRequiresWhatEachModeNeeds(t *testing.T) {
	t.Parallel()
	schema := compiledBulkToolSchema(t)
	const change = `"record_type":"contact","verb":"archive","items":[{"id":"019ff000-0000-7000-8000-000000000001","version":1}]`
	const batch = `"batch_id":"019ff000-0000-7000-8000-0000000000b1"`
	for _, tc := range []struct {
		args  string
		valid bool
	}{
		{`{"mode":"preview",` + change + `}`, true},
		{`{"mode":"execute",` + change + `}`, true},
		{`{"mode":"preview"}`, false},
		{`{"mode":"execute","record_type":"contact"}`, false},
		{`{"mode":"undo_preview",` + batch + `}`, true},
		{`{"mode":"undo",` + batch + `,"confirm_token":"t"}`, true},
		{`{"mode":"undo"}`, false},
		{`{"mode":"undo_preview"}`, false},
	} {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(tc.args)))
		if err != nil {
			t.Fatalf("%s is not JSON: %v", tc.args, err)
		}
		if err := schema.Validate(doc); (err == nil) != tc.valid {
			t.Errorf("%s: schema says valid=%v (%v), want %v", tc.args, err == nil, err, tc.valid)
		}
	}
}
