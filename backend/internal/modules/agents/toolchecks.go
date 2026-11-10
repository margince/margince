// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// toolChecksJSON is what each verb's admin call was asked by the gates on its
// path, measured by TestTheToolRoleMatrixIsPublished and drift-gated there.
//
//go:embed toolchecks.json
var toolChecksJSON []byte

// ToolChecksFile is the generated table's shape.
type ToolChecksFile struct {
	Note  string                   `json:"note"`
	Verbs map[string]ToolChecksRow `json:"verbs"`
}

// ToolChecksRow is one verb's measured checks.
type ToolChecksRow struct {
	Checks []string `json:"checks"`
}

var requiredChecks = mustReadToolChecks(toolChecksJSON)

// EmbeddedToolChecks is the table this binary embeds, for the census that
// holds it to the served surface.
func EmbeddedToolChecks() (ToolChecksFile, error) {
	var file ToolChecksFile
	err := json.Unmarshal(toolChecksJSON, &file)
	return file, err
}

func mustReadToolChecks(raw []byte) map[string][]auth.Check {
	var file ToolChecksFile
	if err := json.Unmarshal(raw, &file); err != nil {
		//craft:ignore panic-in-domain the table is embedded at build time, and TestTheToolChecksTableReads reads it in every unit run
		panic(fmt.Sprintf("crmagents: toolchecks.json does not read: %v", err))
	}
	out := make(map[string][]auth.Check, len(file.Verbs))
	for verb, row := range file.Verbs {
		for _, name := range row.Checks {
			check, err := auth.ParseCheck(name)
			if err != nil {
				//craft:ignore panic-in-domain the table is embedded at build time, and TestTheToolChecksTableReads reads it in every unit run
				panic(fmt.Sprintf("crmagents: toolchecks.json, %s: %v", verb, err))
			}
			out[verb] = append(out[verb], check)
		}
	}
	return out
}

// grantedTo is spec as far as an agent's grants and its passport's work areas
// reach. A tool whose recorded checks the caller lacks, or whose areas are off,
// is not offered. A router offers only the actions whose verbs pass both. A
// verb with no recorded row stays offered: the
// gate still decides each call. Humans and the system are not narrowed, since
// their authority is enforced at the store and never listed. A self-describing
// tool is never narrowed, for the reason invocableByCaller keeps it.
func (r *Registry) grantedTo(ctx context.Context, spec mcp.ToolSpec) (mcp.ToolSpec, bool) {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalAgent || spec.SelfDescribing {
		return spec, true
	}
	if len(spec.Actions) == 0 {
		return spec, r.holdsChecks(p, spec.Name) && auth.AreasAdmit(p, spec)
	}
	kept := slices.DeleteFunc(slices.Clone(spec.Actions), func(a mcp.ToolAction) bool {
		return !r.holdsChecks(p, a.Verb) || !r.inAreas(p, a.Verb)
	})
	return r.withAvailableActions(spec, kept)
}

func (r *Registry) holdsChecks(p principal.Principal, verb string) bool {
	for _, check := range r.required[verb] {
		if !check.HeldBy(p) {
			return false
		}
	}
	return true
}

// narrowedRouterSchema is a router's served schema as the kept actions alone
// derive it, through routedSchema, which derives the registered one. An
// argument only hidden actions take is gone. The route enum, the choices named
// as taking an argument, a united enum and a bound note name only kept ones.
// A member the surface added, such as the retry key, stays.
func (r *Registry) narrowedRouterSchema(name string, served json.RawMessage, kept []mcp.ToolAction, hidden []string) (json.RawMessage, error) {
	r.mu.RLock()
	rt, ok := r.tools[name].(router)
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%s is not a registered router", name)
	}
	field, byField := routeFieldOf(rt)
	if !byField {
		return nil, fmt.Errorf("%s picks its action by the arguments named, so no enum narrows it", name)
	}
	every, err := r.routerSchemaFor(rt, field, slices.Sorted(maps.Keys(rt.routeTargets())))
	if err != nil {
		return nil, err
	}
	choices := make([]string, len(kept))
	for i, action := range kept {
		choices[i] = action.Action
	}
	narrowed, err := r.routerSchemaFor(rt, field, choices)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(served, &doc); err != nil {
		return nil, err
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(doc["properties"], &properties); err != nil {
		return nil, err
	}
	var required []string
	if raw, has := doc["required"]; has {
		if err := json.Unmarshal(raw, &required); err != nil {
			return nil, err
		}
	}
	for member := range every.Properties {
		delete(properties, member)
	}
	maps.Copy(properties, narrowed.Properties)
	if properties[field], err = withoutClausesNaming(properties[field], hidden); err != nil {
		return nil, err
	}
	required = slices.DeleteFunc(required, func(member string) bool { _, derived := every.Properties[member]; return derived })
	required = append(slices.Clone(narrowed.Required), required...)
	if doc["properties"], err = json.Marshal(properties); err != nil {
		return nil, err
	}
	if doc["required"], err = json.Marshal(required); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// routerSchemaFor is routedSchema over choices, read back member by member.
// Router wording for an argument no chosen target takes is left out, as that
// argument is.
func (r *Registry) routerSchemaFor(rt router, field string, choices []string) (routedObject, error) {
	targets := make([]mcp.ToolSpec, len(choices))
	r.mu.RLock()
	for i, choice := range choices {
		if target, ok := r.tools[rt.routeTargets()[choice]]; ok {
			targets[i] = target.Spec()
		}
	}
	r.mu.RUnlock()
	own, err := wordingFor(rt.Spec().InputSchema, field, targets)
	if err != nil {
		return routedObject{}, err
	}
	raw, err := routedSchema(rt.Spec().Name, field, choices, own, targets)
	if err != nil {
		return routedObject{}, err
	}
	var out routedObject
	if err := json.Unmarshal(raw, &out); err != nil {
		return routedObject{}, err
	}
	return out, nil
}

// wordingFor is a router's own argument wording less what no target takes.
func wordingFor(own json.RawMessage, field string, targets []mcp.ToolSpec) (json.RawMessage, error) {
	wording, err := readObjectSchema("router", own)
	if err != nil {
		return nil, err
	}
	for name := range wording.Properties {
		taken := name == field || slices.ContainsFunc(targets, func(target mcp.ToolSpec) bool {
			schema, err := readObjectSchema(target.Name, target.InputSchema)
			_, takes := schema.Properties[name]
			return err == nil && takes
		})
		if !taken {
			delete(wording.Properties, name)
		}
	}
	return json.Marshal(wording)
}
