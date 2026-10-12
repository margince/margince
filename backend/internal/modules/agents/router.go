// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// router is a tool that runs as one of several Unlisted tools, picked by one
// argument. The tool name is the approval kind, the tier-floor key and the retry
// key, so a routed call is served under the target's name from the first check
// on: the router itself never handles, stages or is admitted.
type router interface {
	mcp.Tool
	routeField() string
	routeTargets() map[string]string
}

// Served is one call's answer and the tool that produced it: the target, when
// the call named a router.
type Served struct {
	Tool    string
	Output  json.RawMessage
	Records int
}

// routedSpec is what a router is registered and offered as. Everything but its
// name, title and copy is derived from the targets, so the router cannot
// advertise an argument, scope or tier that the tool it runs as does not have.
func (r *Registry) routedSpec(rt router) mcp.ToolSpec {
	spec := rt.Spec()
	choices := slices.Sorted(maps.Keys(rt.routeTargets()))
	targets := make([]mcp.ToolSpec, 0, len(choices))
	for _, choice := range choices {
		targets = append(targets, r.routeTarget(spec.Name, rt.routeTargets()[choice]))
	}
	first := targets[0]
	for _, target := range targets[1:] {
		if target.RequiredScope != first.RequiredScope || target.Tier != first.Tier {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s fronts %s and %s, which differ in scope or tier — one tool spends one of each",
				spec.Name, first.Name, target.Name))
		}
		spec.MaxArgsBytes = max(spec.MaxArgsBytes, target.MaxArgsBytes)
		spec.Egress = spec.Egress || target.Egress
	}
	spec.MaxArgsBytes = max(spec.MaxArgsBytes, first.MaxArgsBytes)
	spec.Egress = spec.Egress || first.Egress
	spec.RequiredScope, spec.Tier = first.RequiredScope, first.Tier
	spec.InputSchema = routedSchema(rt.routeField(), choices, spec.InputSchema, targets)
	spec.OutputSchema = nil
	return spec
}

// routeTarget is one registered target's own spec, refused unless a router may
// front it. A dynamic tier is refused because its resolver belongs to the
// target and a router has no call to resolve it on until it has routed.
func (r *Registry) routeTarget(routerName, name string) mcp.ToolSpec {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	var why string
	switch _, isRouter := t.(router); {
	case !ok:
		why = "is not registered (register targets first)"
	case isRouter:
		why = "is itself a router"
	case !t.Spec().Unlisted:
		why = "is still offered, so fronting it would list one capability twice"
	case t.Spec().Tier == mcp.TierDynamic:
		why = "has a dynamic tier"
	}
	if why != "" {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: router %s cannot front %s: it %s", routerName, name, why))
	}
	return t.Spec()
}

// routedSchema is the targets' arguments plus the route field. An argument is
// required only when every target requires it; the target's own check still
// refuses a call missing one only it needs. An argument the router describes
// itself keeps the router's wording, written for every action at once.
func routedSchema(field string, choices []string, own json.RawMessage, targets []mcp.ToolSpec) json.RawMessage {
	properties := map[string]json.RawMessage{}
	if own != nil {
		var described struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(own, &described); err != nil {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: cannot read a router's own input schema: %v", err))
		}
		maps.Copy(properties, described.Properties)
	}
	var required []string
	for i, target := range targets {
		var own struct {
			Required   []string                   `json:"required"`
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(target.InputSchema, &own); err != nil {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: cannot read %s's input schema: %v", target.Name, err))
		}
		for name, schema := range own.Properties {
			if _, seen := properties[name]; !seen {
				properties[name] = schema
			}
		}
		if i == 0 {
			required = own.Required
		} else {
			required = slices.DeleteFunc(required, func(name string) bool { return !slices.Contains(own.Required, name) })
		}
	}
	if field != "" {
		enum, err := json.Marshal(map[string]any{"type": "string", "enum": choices})
		if err != nil {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(err)
		}
		properties[field] = enum
		required = append([]string{field}, required...)
	}
	out, err := json.Marshal(map[string]any{"type": "object", "required": required, "properties": properties})
	if err != nil {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(err)
	}
	return out
}

// resolve names the tool a call runs as and the arguments it runs with: the
// call itself for an ordinary tool, the chosen target without the route field
// for a router.
func (r *Registry) resolve(name string, in json.RawMessage) (string, json.RawMessage, error) {
	r.mu.RLock()
	rt, isRouter := r.tools[name].(router)
	spec := r.specs[name]
	r.mu.RUnlock()
	if !isRouter {
		return name, in, nil
	}
	if err := withinArgsBound(spec, in); err != nil {
		return "", nil, err
	}
	// Checked before the decode for splitReserved's reason: the decode turns
	// invalid bytes into U+FFFD, and the target would then see a valid call.
	if !utf8.Valid(in) {
		return "", nil, &BadArgsError{Cause: errArgumentsNotUTF8}
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(in, &members); err != nil {
		safe, withheld := httperr.SafeDecodeError(err)
		if withheld {
			slog.Warn("unnameable tool-argument decode failure", "tool", name, "err", err)
		}
		return "", nil, &BadArgsError{Cause: safe}
	}
	target, err := pickTarget(rt, members)
	if err != nil {
		return "", nil, err
	}
	delete(members, rt.routeField())
	out, err := json.Marshal(members)
	if err != nil {
		return "", nil, errors.Join(errRouteArguments, err)
	}
	return target, out, nil
}

var errRouteArguments = errors.New("crmagents: re-encoding routed arguments")

// targetChooser is a router whose target follows from which arguments a call
// names, so it has no route field of its own.
type targetChooser interface {
	chooseTarget(members map[string]json.RawMessage) (string, error)
}

func pickTarget(rt router, members map[string]json.RawMessage) (string, error) {
	if chooser, ok := rt.(targetChooser); ok {
		return chooser.chooseTarget(members)
	}
	field := rt.routeField()
	var choice string
	if json.Unmarshal(members[field], &choice) == nil {
		if target, ok := rt.routeTargets()[choice]; ok {
			return target, nil
		}
	}
	choices := slices.Sorted(maps.Keys(rt.routeTargets()))
	return "", &BadArgsError{Field: field,
		Cause: fmt.Errorf("%s must be one of %s", field, strings.Join(choices, ", "))}
}

// routed gives a router the Handle every tool must have. Registry.resolve
// sends every call on to a target first, so this answer means a router was
// called around the registry.
type routed struct{}

func (routed) Handle(context.Context, json.RawMessage) (json.RawMessage, error) {
	return nil, errRouterHandled
}

var errRouterHandled = errors.New("crmagents: a router was handled instead of resolved")
