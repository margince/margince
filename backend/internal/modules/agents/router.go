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
// key. A routed call is therefore served under the target's name from the first
// check on, and the router itself never handles, stages or is admitted.
type router interface {
	mcp.Tool
	routeField() string
	routeTargets() map[string]string
}

// Served is one call's answer and the tool that produced it. When the call
// named a router, Tool is the target and Router is the name it was called by.
type Served struct {
	Tool    string
	Router  string
	Output  json.RawMessage
	Records int
}

// routedSpec is what a router is registered and offered as. Everything but its
// name, title, copy and argument wording is derived from the targets. The router
// cannot advertise an argument, scope or tier that its targets lack.
func (r *Registry) routedSpec(rt router) mcp.ToolSpec {
	spec := rt.Spec()
	choices := slices.Sorted(maps.Keys(rt.routeTargets()))
	targets := make([]mcp.ToolSpec, 0, len(choices))
	for _, choice := range choices {
		targets = append(targets, r.routeTarget(spec.Name, rt.routeTargets()[choice]))
	}
	first := targets[0]
	spec.RequiredScope, spec.Tier = first.RequiredScope, first.Tier
	for _, target := range targets {
		if target.RequiredScope != first.RequiredScope || target.Tier != first.Tier {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s fronts %s and %s, which differ in scope or tier — one tool spends one of each",
				spec.Name, first.Name, target.Name))
		}
		spec.MaxArgsBytes = max(spec.MaxArgsBytes, argsLimit(target))
		spec.Egress = spec.Egress || target.Egress
	}
	spec.InputSchema = routedSchema(spec.Name, rt.routeField(), choices, spec.InputSchema, targets)
	spec.OutputSchema = nil
	return spec
}

// argsLimit is the bound withinArgsBound holds a tool's arguments to.
func argsLimit(spec mcp.ToolSpec) int64 {
	if spec.MaxArgsBytes == 0 {
		return httperr.MaxBodyBytes
	}
	return spec.MaxArgsBytes
}

// routeTarget is one registered target's own spec, refused unless a router may
// front it. A dynamic tier is refused because its resolver belongs to the
// target. A router has no call to resolve it on until it has routed. The
// last three are refused because the listing reads them off the router.
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
	case t.Spec().HumanOnly, t.Spec().SelfDescribing, t.Spec().ConfirmsInConversation:
		why = "is offered or confirmed in a way a router does not carry"
	}
	if why != "" {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: router %s cannot front %s: it %s", routerName, name, why))
	}
	return t.Spec()
}

type routeChoice struct {
	Type        string          `json:"type"`
	Enum        []string        `json:"enum"`
	Description json.RawMessage `json:"description,omitempty"`
}

type routedObject struct {
	Type       string                     `json:"type"`
	Required   []string                   `json:"required,omitempty"`
	Properties map[string]json.RawMessage `json:"properties"`
}

// The schema members a router reads off an argument.
const (
	schemaDescription = "description"
	schemaEnum        = "enum"
)

// argument is one property of a JSON schema, member by member.
type argument map[string]json.RawMessage

type objectSchema struct {
	Required   []string            `json:"required,omitempty"`
	Properties map[string]argument `json:"properties"`
}

func readObjectSchema(owner string, raw json.RawMessage) objectSchema {
	var schema objectSchema
	if raw == nil {
		return schema
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: cannot read %s's input schema: %v", owner, err))
	}
	return schema
}

// routedSchema is the targets' arguments plus the route field. An argument is
// required only when every target requires it; the target's own check still
// refuses a call missing one only it needs. The router may only word an
// argument for every action at once, never restate its shape.
func routedSchema(routerName, field string, choices []string, own json.RawMessage, targets []mcp.ToolSpec) json.RawMessage {
	properties, required := unitedArguments(routerName, field, targets)
	routerOwn := readObjectSchema(routerName, own)
	for name, wording := range routerOwn.Properties {
		_, known := properties[name]
		if _, extra := wording[schemaDescription]; len(wording) != 1 || !extra || (!known && name != field) {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s may only describe an argument its targets take, and %s is not that", routerName, name))
		}
		if known {
			properties[name][schemaDescription] = wording[schemaDescription]
		}
	}
	out := map[string]json.RawMessage{}
	for name, arg := range properties {
		out[name] = mustMarshal(arg)
	}
	if field != "" {
		out[field] = mustMarshal(routeChoice{Type: schemaString, Enum: choices, Description: routerOwn.Properties[field][schemaDescription]})
		required = append([]string{field}, required...)
	}
	return mustMarshal(routedObject{Type: schemaObject, Required: required, Properties: out})
}

// unitedArguments unites the arguments of a router's targets. Two targets
// naming one argument must agree on its shape. Where they differ only in the
// words an enum admits, the router admits all of them. Neither target's call is
// then refused by a client holding the advertised schema.
func unitedArguments(routerName, field string, targets []mcp.ToolSpec) (map[string]argument, []string) {
	properties := map[string]argument{}
	var required []string
	for i, target := range targets {
		schema := readObjectSchema(target.Name, target.InputSchema)
		for name, arg := range schema.Properties {
			united, ok := uniteArgument(properties[name], arg)
			if name == field || !ok {
				//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
				panic(fmt.Sprintf("crmagents: router %s cannot advertise %s's argument %s as one shape", routerName, target.Name, name))
			}
			properties[name] = united
		}
		if i == 0 {
			required = schema.Required
		} else {
			required = slices.DeleteFunc(required, func(name string) bool { return !slices.Contains(schema.Required, name) })
		}
	}
	return properties, required
}

// uniteArgument is one argument as two targets declare it, ignoring wording.
func uniteArgument(seen, next argument) (argument, bool) {
	if seen == nil {
		return maps.Clone(next), true
	}
	shape := func(arg argument) argument {
		out := maps.Clone(arg)
		delete(out, schemaDescription)
		delete(out, schemaEnum)
		return out
	}
	if !maps.EqualFunc(shape(seen), shape(next), func(a, b json.RawMessage) bool { return string(a) == string(b) }) {
		return nil, false
	}
	var words, more []string
	if json.Unmarshal(seen[schemaEnum], &words) != nil || json.Unmarshal(next[schemaEnum], &more) != nil {
		return seen, string(seen[schemaEnum]) == string(next[schemaEnum])
	}
	united := maps.Clone(seen)
	united[schemaEnum] = mustMarshal(slices.Compact(slices.Sorted(slices.Values(append(words, more...)))))
	return united, true
}

func mustMarshal[T any](v T) json.RawMessage {
	out, err := json.Marshal(v)
	if err != nil {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(err)
	}
	return out
}

// resolve names the tool a call runs as and the arguments it runs with. An
// ordinary tool runs as the call itself; a router runs as the chosen target
// without the route field.
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
	return "", &BadArgsError{
		Field: field,
		Cause: fmt.Errorf("%s must be one of %s", field, strings.Join(choices, ", ")),
	}
}

// routed gives a router the Handle every tool must have. Registry.resolve
// sends every call on to a target first, so this answer means a router was
// called around the registry.
type routed struct{}

func (routed) Handle(context.Context, json.RawMessage) (json.RawMessage, error) {
	return nil, errRouterHandled
}

var errRouterHandled = errors.New("crmagents: a router was handled instead of resolved")
