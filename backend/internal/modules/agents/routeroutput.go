// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// targetsOutput is the result shape of a router with a view: one object
// schema every target's answer satisfies. A view's answer must be described
// for the clients that do not render it, and this surface refuses `anyOf`, so
// the targets' shapes are merged instead. A member some targets lack is not
// required, and a member two targets share must agree on its type.
func targetsOutput(routerName string, targets []mcp.ToolSpec) json.RawMessage {
	var merged map[string]any
	for _, target := range targets {
		if target.OutputSchema == nil {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s has a view, and its target %s declares no result shape", routerName, target.Name))
		}
		var shape map[string]any
		if err := json.Unmarshal(target.OutputSchema, &shape); err != nil {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s: %s's result shape is not JSON: %v", routerName, target.Name, err))
		}
		if merged == nil {
			merged = shape
			continue
		}
		var err error
		if merged, err = mergeShapes(merged, shape); err != nil {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s: %s: %v", routerName, target.Name, err))
		}
	}
	out, err := json.Marshal(merged)
	if err != nil {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: router %s: encoding its result shape: %v", routerName, err))
	}
	return out
}

// mergeShapes is the narrowest schema both a and b satisfy, for the shapes
// schemaFor writes: objects, arrays and scalars, never a composition.
func mergeShapes(a, b map[string]any) (map[string]any, error) {
	ta, err := json.Marshal(a["type"])
	if err != nil {
		return nil, err
	}
	tb, err := json.Marshal(b["type"])
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(ta, tb) {
		return nil, fmt.Errorf("one member is %s in one target and %s in another", ta, tb)
	}
	out := maps.Clone(a)
	if items := objectAt(b, "items"); items != nil {
		if out["items"], err = mergeOptional(objectAt(a, "items"), items); err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
	}
	pa, pb := objectAt(a, "properties"), objectAt(b, "properties")
	if pa == nil && pb == nil {
		return out, nil
	}
	properties := maps.Clone(pa)
	if properties == nil {
		properties = map[string]any{}
	}
	for name := range pb {
		if properties[name], err = mergeOptional(objectAt(properties, name), objectAt(pb, name)); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	out["properties"] = properties
	out["required"] = sharedRequired(a, b)
	return out, nil
}

// mergeOptional merges two member shapes where either may be absent.
func mergeOptional(a, b map[string]any) (map[string]any, error) {
	if a == nil {
		return b, nil
	}
	if b == nil {
		return a, nil
	}
	return mergeShapes(a, b)
}

// sharedRequired is what both shapes require, in a stable order.
func sharedRequired(a, b map[string]any) []string {
	names := func(list any) map[string]bool {
		out := map[string]bool{}
		items, isList := list.([]any)
		if !isList {
			return out
		}
		for _, item := range items {
			if name, ok := item.(string); ok {
				out[name] = true
			}
		}
		return out
	}
	inA, inB := names(a["required"]), names(b["required"])
	shared := []string{}
	for name := range inA {
		if inB[name] {
			shared = append(shared, name)
		}
	}
	slices.Sort(shared)
	return shared
}

// objectAt is the schema object under key, or nil where there is none.
func objectAt(shape map[string]any, key string) map[string]any {
	member, ok := shape[key].(map[string]any)
	if !ok {
		return nil
	}
	return member
}
