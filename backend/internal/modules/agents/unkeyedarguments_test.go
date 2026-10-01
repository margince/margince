// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// The declaration keeping a tool from every scheduled agent is a map, so a
// spec handed out must not share it: a caller writing through its copy would
// otherwise clear the declaration for every later reader of the registry.
func TestAnUnkeyedArgumentDeclarationIsNotSharedWithTheCaller(t *testing.T) {
	registry := NewRegistry(nil, nil)
	registry.Register(createRecord{})
	first := findSpec(t, registry, "create_record")
	if first.UnkeyedArguments["$.fields"] == "" {
		t.Fatalf("create_record does not declare its unkeyed fields: %v", first.UnkeyedArguments)
	}
	delete(first.UnkeyedArguments, "$.fields")

	if again := findSpec(t, registry, "create_record"); again.UnkeyedArguments["$.fields"] == "" {
		t.Fatal("clearing the declaration on one handed-out spec cleared it in the registry")
	}
	if (createRecord{}).Spec().UnkeyedArguments["$.fields"] == "" {
		t.Fatal("clearing the declaration on one handed-out spec cleared the tool's own")
	}
}

func findSpec(t *testing.T, registry *Registry, name string) mcp.ToolSpec {
	t.Helper()
	for _, spec := range registry.Specs() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("%s is not registered", name)
	return mcp.ToolSpec{}
}
