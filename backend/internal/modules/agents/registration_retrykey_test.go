// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// declaredProperties reads back what a schema lets a caller pass.
func declaredProperties(t *testing.T, schema json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var shape struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &shape); err != nil {
		t.Fatalf("schema is not readable: %v", err)
	}
	return shape.Properties
}

func registeredSpec(t *testing.T, spec mcp.ToolSpec) mcp.ToolSpec {
	t.Helper()
	r := NewRegistry(nil, nil)
	r.Register(&fakeTool{spec: spec})
	registered, ok := r.Spec(spec.Name)
	if !ok {
		t.Fatalf("%s did not register", spec.Name)
	}
	return registered
}

func mutatingSpec(name string) mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: name, Title: name, Version: testToolVersion, Description: describedForRegistration,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		InputSchema: json.RawMessage(`{"type":"object","properties":{"deal_id":{"type":"string","format":"uuid"}},` +
			`"required":["deal_id"],"additionalProperties":false}`),
	}
}

// readOnlySpec is the same tool the retry-key splice never touches, which is
// what makes it the right subject for a check the registration door owes every
// spec rather than only the mutating ones.
func readOnlySpec(name string) mcp.ToolSpec {
	spec := mutatingSpec(name)
	spec.RequiredScope = principal.ScopeRead
	return spec
}

func TestAMutatingToolIsAdvertisedWithTheRetryKey(t *testing.T) {
	registered := registeredSpec(t, mutatingSpec("archive_record"))
	props := declaredProperties(t, registered.InputSchema)
	if _, advertised := props[idempotencyKeyArg]; !advertised {
		t.Fatalf("a mutating tool's schema does not advertise %s: %s", idempotencyKeyArg, registered.InputSchema)
	}
	if _, kept := props["deal_id"]; !kept {
		t.Error("the splice dropped the tool's own argument")
	}
	// The rest of the schema is the tool's, untouched: a splice that quietly
	// widened `additionalProperties` would let through everything the strict
	// decode then refuses.
	if !strings.Contains(string(registered.InputSchema), `"additionalProperties":false`) {
		t.Errorf("the splice lost additionalProperties:false: %s", registered.InputSchema)
	}
	if !strings.Contains(string(registered.InputSchema), `"required":["deal_id"]`) {
		t.Errorf("the splice lost the tool's `required`: %s", registered.InputSchema)
	}
}

func TestAReadOnlyToolIsNotAdvertisedWithTheRetryKey(t *testing.T) {
	spec := mutatingSpec("read_record")
	spec.RequiredScope = principal.ScopeRead
	registered := registeredSpec(t, spec)
	if _, advertised := declaredProperties(t, registered.InputSchema)[idempotencyKeyArg]; advertised {
		t.Fatalf("a read tool advertises a key that would protect nothing: %s", registered.InputSchema)
	}
}

// unitTool is a fakeTool an extension unit shipped: the same tool, plus the one
// fact mcp.UnitScopedTool carries. A core tool cannot answer that question at
// all, which is what the surface reads it by.
type unitTool struct {
	*fakeTool
	unit string
}

func (u unitTool) OwningUnit() string { return u.unit }

// An extension's records never enter the datasource seam a replay re-proves its
// evidence through, so its recorded result could never pass the replay gate.
// Advertising the key there would promise a recovery the surface cannot perform
// — and a caller that believes it is protected repeats an irreversible call.
func TestAMutatingExtensionToolIsNotAdvertisedWithTheRetryKey(t *testing.T) {
	spec := mutatingSpec("notes_create_note")
	r := NewRegistry(nil, nil)
	r.Register(unitTool{fakeTool: &fakeTool{spec: spec}, unit: "notes"})
	registered, ok := r.Spec(spec.Name)
	if !ok {
		t.Fatalf("%s did not register", spec.Name)
	}
	props := declaredProperties(t, registered.InputSchema)
	if _, advertised := props[idempotencyKeyArg]; advertised {
		t.Fatalf("an extension tool advertises `%s`, whose promise its records cannot keep: %s",
			idempotencyKeyArg, registered.InputSchema)
	}
	// The exclusion withholds ONE member and touches nothing else the unit
	// declared: a tool that lost its own arguments to it would be unusable
	// rather than merely unprotected.
	if _, kept := props["deal_id"]; !kept {
		t.Errorf("the exclusion dropped the tool's own argument: %s", registered.InputSchema)
	}
}

// The other half of the same rule, so neither can move without the other: the
// exclusion is about WHO OWNS THE RECORDS, not about mutation, and a core
// mutating tool's replay machinery is unchanged.
func TestAMutatingCoreToolKeepsTheRetryKeyBesideAnExtensionsTool(t *testing.T) {
	r := NewRegistry(nil, nil)
	r.Register(&fakeTool{spec: mutatingSpec("archive_record")})
	r.Register(unitTool{fakeTool: &fakeTool{spec: mutatingSpec("notes_create_note")}, unit: "notes"})
	core, ok := r.Spec("archive_record")
	if !ok {
		t.Fatal("the core tool did not register")
	}
	if _, advertised := declaredProperties(t, core.InputSchema)[idempotencyKeyArg]; !advertised {
		t.Fatalf("a core mutating tool lost `%s`: %s", idempotencyKeyArg, core.InputSchema)
	}
}

// A mutating tool that takes no arguments still gets the key: having arguments
// says nothing about whether repeating the call is safe.
func TestAToolWithNoPropertiesStillGetsTheRetryKey(t *testing.T) {
	spec := mutatingSpec("sweep")
	spec.InputSchema = json.RawMessage(`{"type":"object"}`)
	registered := registeredSpec(t, spec)
	if _, advertised := declaredProperties(t, registered.InputSchema)[idempotencyKeyArg]; !advertised {
		t.Fatalf("an argument-less mutation was not offered the key: %s", registered.InputSchema)
	}
}

func TestSpliceRetryKeyRefusesSchemasItCannotRead(t *testing.T) {
	for _, tc := range []struct{ name, schema string }{
		{name: "not an object", schema: `["nope"]`},
		{name: "properties is not an object", schema: `{"type":"object","properties":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := spliceRetryKey(json.RawMessage(tc.schema)); err == nil {
				t.Fatalf("%s was spliced", tc.schema)
			}
		})
	}
}

// The advertised property is what the surface enforces. A schema promising a
// bound the pop does not hold to — or holding to one it does not promise —
// is the mismatch A4 exists to close, one axis over.
func TestTheAdvertisedKeyBoundIsTheOneTheSurfaceEnforces(t *testing.T) {
	var declared struct {
		Type string `json:"type"`
		// A POINTER, so "declared 0" and "declared nothing" stay different
		// answers — the second is the one that would leave the surface
		// enforcing a bound it never published.
		MaxLength *int `json:"maxLength"` //nolint:tagliatelle // JSON Schema's keyword, not ours to case
	}
	if err := json.Unmarshal([]byte(retryKeyProperty), &declared); err != nil {
		t.Fatalf("the advertised property is not readable JSON: %v", err)
	}
	if declared.Type != "string" {
		t.Errorf("advertised type = %q, want string", declared.Type)
	}
	if declared.MaxLength == nil {
		t.Fatal("the advertised property declares no maxLength, so the bound the surface enforces is unpublished")
	}
	if *declared.MaxLength != maxRetryKeyLen {
		t.Errorf("advertised maxLength = %d, but the surface refuses past %d", *declared.MaxLength, maxRetryKeyLen)
	}
}

// A composed schema cannot be spliced by adding one top-level member: a closed
// branch inside `allOf` still rejects the key the surface just advertised, so a
// schema-aware client would be told to send an argument its own validator
// refuses.
func TestAComposedInputSchemaIsRefusedAtBoot(t *testing.T) {
	for _, keyword := range composingKeywords {
		t.Run(keyword, func(t *testing.T) {
			spec := mutatingSpec("compose_probe")
			spec.InputSchema = json.RawMessage(`{"type":"object","properties":{},"` + keyword + `":[]}`)
			mustPanic(t, "a composed schema was spliced anyway", func() {
				NewRegistry(nil, nil).Register(&fakeTool{spec: spec})
			})
		})
	}
}

// ANY tool declaring `idempotency_key` at its own root is refused at boot —
// read-only and extension-owned included, not just the mutating core tools
// spliceRetryKey walks.
//
// The argument is not declarable by anyone: splitReserved pops the name from
// every call before a handler sees it, and refuseUnkeyableCall then refuses it
// outright on a read-only or unit-owned tool. So declaring it advertises an
// argument the tool can never receive.
//
// It also removes a divergence the equivalence gate could not see. withRetryKey
// splices iff mutating AND core; the runner's compaction strips the member's
// description iff mutating. Those disagree for an extension's mutating tool, and
// the gate compares the listing against the same compaction, so both sides moved
// together. With no tool able to declare the member, the only root
// `idempotency_key` is the surface's own.
func TestAnyToolDeclaringTheRetryKeyItselfIsRefusedAtBoot(t *testing.T) {
	declared := `{"type":"object","properties":{"` + idempotencyKeyArg +
		`":{"type":"string","description":"my own key"}},"additionalProperties":false}`
	for _, tc := range []struct {
		name string
		spec mcp.ToolSpec
	}{
		{"mutating core", mutatingSpec("declares_own_key")},
		{"read-only", readOnlySpec("readonly_declares_own_key")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.spec.InputSchema = json.RawMessage(declared)
			mustPanic(t, "a tool declaring the surface's own argument was registered", func() {
				NewRegistry(nil, nil).Register(&fakeTool{spec: tc.spec})
			})
		})
	}
	// An extension unit's mutating tool, which withRetryKey skips entirely — the
	// case the divergence was about.
	t.Run("extension-owned mutating", func(t *testing.T) {
		spec := mutatingSpec("unit_declares_own_key")
		spec.InputSchema = json.RawMessage(declared)
		mustPanic(t, "an extension tool declaring the surface's own argument was registered", func() {
			NewRegistry(nil, nil).Register(unitTool{fakeTool: &fakeTool{spec: spec}, unit: "probe-unit"})
		})
	})
	// `approval_id` STAYS declarable: several tools name it with their own
	// per-tool meaning, which is why the compaction leaves it alone.
	ownApproval := mutatingSpec("declares_approval_id")
	ownApproval.InputSchema = json.RawMessage(`{"type":"object","properties":{"` + approvalIDArg +
		`":{"type":"string","description":"Set on retry after a human approved."}},` +
		`"additionalProperties":false}`)
	NewRegistry(nil, nil).Register(&fakeTool{spec: ownApproval})
}

// A composed schema is refused AT ANY DEPTH, and on a READ-ONLY tool too.
//
// A root-only check holds neither thing that depends on it. The runner's listing
// renderer walks `properties` and `items` and no other key, so a branch spelled
// `properties.foo.allOf` passes a root check and is then copied through
// verbatim — keeping its closed form, with the published headroom larger than
// the real one and nothing failing. And the refusal used to live in the retry-key
// splice, which read-only tools never reach.
//
// The clean case at the end is what stops this passing by the refusal having
// become indiscriminate.
func TestAComposedBranchIsRefusedAtEveryDepthAndOnReadOnlyTools(t *testing.T) {
	for _, tc := range []struct{ name, schema string }{
		{"under properties", `{"type":"object","properties":{"a":{"allOf":[{"additionalProperties":false}]}}}`},
		{"under items", `{"type":"object","properties":{"a":{"type":"array","items":{"$ref":"#/x"}}}}`},
		{"two levels down", `{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"oneOf":[{}]}}}}}`},
		{"in an output schema", `{"type":"object","properties":{"data":{"anyOf":[{}]}}}`},
		// The third nesting place. An earlier pass walked `properties` and
		// `items` only, so a composed branch under `additionalProperties` passed
		// boot — and qualify_lead's own output schema nests a `properties` tree
		// under one, which is what makes this reachable.
		{"under additionalProperties", `{"type":"object","additionalProperties":{"oneOf":[{}]}}`},
		// The TUPLE form of `items` is refused outright, not skipped: neither
		// walk descends into an array of schemas, so anything nested there is
		// invisible to both the refusal and the listing renderer.
		{"tuple-form items", `{"type":"object","properties":{"pair":{"type":"array",` +
			`"items":[{"type":"string"},{"type":"object","additionalProperties":false}]}}}`},
		{"deep under additionalProperties", `{"type":"object","properties":{"filled":{"type":"object",` +
			`"additionalProperties":{"type":"object","properties":{"v":{"$ref":"#/x"}}}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// READ-ONLY, so the retry-key splice never runs and only the
			// registration door can refuse this.
			spec := readOnlySpec("nested_compose_probe")
			if tc.name == "in an output schema" {
				spec.OutputSchema = json.RawMessage(tc.schema)
			} else {
				spec.InputSchema = json.RawMessage(tc.schema)
			}
			mustPanic(t, "a composed branch below the root was served", func() {
				NewRegistry(nil, nil).Register(&fakeTool{spec: spec})
			})
		})
	}
	// A deeply nested schema that composes NOWHERE is served. Without this the
	// test above passes for a refusal that rejects everything.
	clean := readOnlySpec("nested_clean_probe")
	clean.InputSchema = json.RawMessage(`{"type":"object","properties":{` +
		`"rows":{"type":"array","items":{"type":"object","properties":{` +
		`"ref":{"type":"string"}},"additionalProperties":false}}},"additionalProperties":false}`)
	NewRegistry(nil, nil).Register(&fakeTool{spec: clean})
}
