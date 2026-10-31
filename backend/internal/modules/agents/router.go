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
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// router is a tool that runs as one of several Unlisted tools. The tool name is
// the approval kind, the tier-floor key and the retry key. A routed call is
// therefore served under the target's name from the first check on, and the
// router itself never handles, stages or is admitted. Every router picks its
// target either by one argument (fieldRouter) or by which arguments a call
// names (targetChooser).
type router interface {
	mcp.Tool
	routeTargets() map[string]string
}

// fieldRouter picks its target by the value of one argument.
type fieldRouter interface {
	routeField() string
}

// routeFieldOf is the argument a router picks by, and false for a router that
// picks by which arguments a call names.
func routeFieldOf(rt router) (string, bool) {
	fr, ok := rt.(fieldRouter)
	if !ok {
		return "", false
	}
	return fr.routeField(), true
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
	spec.RequiredScope, spec.Tier, spec.TierResolver = first.RequiredScope, first.Tier, nil
	spec.Actions = make([]mcp.ToolAction, 0, len(targets))
	for i, target := range targets {
		if target.RequiredScope != first.RequiredScope {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s fronts %s and %s, which differ in scope — one tool spends one scope",
				spec.Name, first.Name, target.Name))
		}
		// Each action is admitted at its own verb's tier once routed. The
		// router is listed as decided per call when its actions differ.
		spec.Actions = append(spec.Actions, mcp.ToolAction{
			Action: choices[i], Verb: target.Name, Tier: target.Tier, AskUserFirst: target.AskUserFirst,
		})
		if target.Tier != first.Tier {
			spec.Tier = mcp.TierDynamic
		}
		spec.MaxArgsBytes = max(spec.MaxArgsBytes, argsLimit(target))
		spec.Egress = spec.Egress || target.Egress
		// The router advertises the target's arguments at the same paths, so
		// what a target says about an object's keys holds for the router too.
		for path, why := range target.UnkeyedArguments {
			if spec.UnkeyedArguments == nil {
				spec.UnkeyedArguments = map[string]string{}
			}
			spec.UnkeyedArguments[path] = why
		}
	}
	_, chooses := rt.(targetChooser)
	field, picksByField := routeFieldOf(rt)
	if chooses == picksByField {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: router %s must pick its target either by one argument or by the arguments named", spec.Name))
	}
	spec.ActionField = field
	spec.InputSchema = routedSchema(spec.Name, field, choices, spec.InputSchema, targets)
	spec.OutputSchema = nil
	if spec.UI != nil {
		spec.OutputSchema = targetsOutput(spec.Name, targets)
	}
	return spec
}

// routeTarget is one registered target's own spec, refused unless a router may
// front it. A dynamic target keeps its resolver: a call is routed before it is
// admitted. The last three are refused because the listing reads them off the
// router.
func (r *Registry) routeTarget(routerName, name string) mcp.ToolSpec {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()
	// The target's own spec: the registered one carries the members the
	// surface adds, such as the retry key, which the router gains by itself.
	var spec mcp.ToolSpec
	if ok {
		spec = t.Spec()
	}
	var why string
	switch _, isRouter := t.(router); {
	case !ok:
		why = "is not registered (register targets first)"
	case isRouter:
		why = "is itself a router"
	case !spec.Unlisted:
		why = "is still offered, so fronting it would list one capability twice"
	case spec.HumanOnly, spec.SelfDescribing, spec.ConfirmsInConversation:
		why = "is offered or confirmed in a way a router does not carry"
	}
	if why != "" {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: router %s cannot front %s: it %s", routerName, name, why))
	}
	return spec
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
	schemaMinimum     = "minimum"
	schemaMaximum     = "maximum"
	schemaItems       = "items"
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
	schemas := make([]objectSchema, len(targets))
	for i, target := range targets {
		schemas[i] = readObjectSchema(target.Name, target.InputSchema)
	}
	properties, required, takers := unitedArguments(routerName, field, choices, targets, schemas)
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
	for name, choicesTaking := range takers {
		if _, worded := routerOwn.Properties[name]; !worded && field != "" && len(choicesTaking) < len(choices) {
			properties[name][schemaDescription] = scopedDescription(choicesTaking, properties[name][schemaDescription])
		}
	}
	for name, arg := range properties {
		if note := boundNotes(name, choices, schemas); note != "" {
			arg[schemaDescription] = noted(arg[schemaDescription], note)
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
func unitedArguments(routerName, field string, choices []string, targets []mcp.ToolSpec, schemas []objectSchema) (map[string]argument, []string, map[string][]string) {
	properties := map[string]argument{}
	takers := map[string][]string{}
	var required []string
	for i, target := range targets {
		schema := schemas[i]
		for name, arg := range schema.Properties {
			takers[name] = append(takers[name], choices[i])
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
	return properties, required, takers
}

// scopedDescription names the choices that take an argument, so a caller
// learns before the target refuses it that another choice does not.
func scopedDescription(choicesTaking []string, description json.RawMessage) json.RawMessage {
	var text string
	if description != nil && json.Unmarshal(description, &text) != nil {
		return description
	}
	scoped := strings.Join(slices.Sorted(slices.Values(choicesTaking)), ", ") + " only"
	if text != "" {
		scoped += ": " + text
	}
	return mustMarshal(scoped)
}

// boundNotes names the choices whose own bound on an argument is tighter than
// the loosest one the router advertises. Without it the schema promises a value
// those choices refuse. A choice that sets no bound is the loosest of all, so
// every bounded choice is then named. It is empty when all choices share a bound.
func boundNotes(name string, choices []string, schemas []objectSchema) string {
	var notes []string
	for _, b := range []struct {
		member, phrase string
		sign           float64
	}{{schemaMaximum, "at most", 1}, {schemaMinimum, "at least", -1}} {
		byBound := map[float64][]string{}
		loosest := math.Inf(-1)
		for i, schema := range schemas {
			arg, takes := schema.Properties[name]
			if !takes {
				continue
			}
			bound := math.Inf(1)
			if raw := arg[b.member]; raw != nil {
				if err := json.Unmarshal(raw, &bound); err != nil {
					continue
				}
				bound *= b.sign
				byBound[bound] = append(byBound[bound], choices[i])
			}
			loosest = max(loosest, bound)
		}
		for _, bound := range slices.Sorted(maps.Keys(byBound)) {
			if bound < loosest {
				notes = append(notes, fmt.Sprintf("%s %s for %s", b.phrase, strconv.FormatFloat(bound*b.sign, 'f', -1, 64),
					strings.Join(slices.Sorted(slices.Values(byBound[bound])), ", ")))
			}
		}
	}
	return strings.Join(notes, "; ")
}

// noted appends a note to an argument's description, which may be absent.
func noted(description json.RawMessage, note string) json.RawMessage {
	var text string
	if description != nil && json.Unmarshal(description, &text) != nil {
		return description
	}
	if text = strings.TrimRight(text, "."); text != "" {
		note = text + "; " + note
	}
	return mustMarshal(note + ".")
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
		delete(out, schemaMinimum)
		delete(out, schemaMaximum)
		delete(out, schemaItems)
		return out
	}
	if !maps.EqualFunc(shape(seen), shape(next), func(a, b json.RawMessage) bool { return string(a) == string(b) }) {
		return nil, false
	}
	united := maps.Clone(seen)
	// A list's items unite by the same rule, so two lists of words admit both.
	if seen[schemaItems] != nil || next[schemaItems] != nil {
		var seenItems, nextItems argument
		if json.Unmarshal(seen[schemaItems], &seenItems) != nil || json.Unmarshal(next[schemaItems], &nextItems) != nil {
			return nil, false
		}
		items, ok := uniteArgument(seenItems, nextItems)
		if !ok {
			return nil, false
		}
		united[schemaItems] = mustMarshal(items)
	}
	// A bound is the loosest the targets set, so a value any target takes is
	// advertised; the target refuses its own excess by name. A target that sets
	// no bound takes any value, so the router sets none either.
	loosest(united, next, schemaMinimum, func(a, b float64) float64 { return min(a, b) })
	loosest(united, next, schemaMaximum, func(a, b float64) float64 { return max(a, b) })
	var words, more []string
	if json.Unmarshal(seen[schemaEnum], &words) != nil || json.Unmarshal(next[schemaEnum], &more) != nil {
		return united, string(seen[schemaEnum]) == string(next[schemaEnum])
	}
	united[schemaEnum] = mustMarshal(slices.Compact(slices.Sorted(slices.Values(append(words, more...)))))
	return united, true
}

// loosest sets a numeric bound to the looser of two targets' values, and drops
// it when either target sets none.
func loosest(united, next argument, bound string, looser func(float64, float64) float64) {
	var mine, theirs float64
	hasMine := united[bound] != nil && json.Unmarshal(united[bound], &mine) == nil
	hasTheirs := next[bound] != nil && json.Unmarshal(next[bound], &theirs) == nil
	if hasMine && hasTheirs {
		united[bound] = mustMarshal(looser(mine, theirs))
		return
	}
	delete(united, bound)
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
	if field, ok := routeFieldOf(rt); ok {
		delete(members, field)
	}
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
	field, _ := routeFieldOf(rt)
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

// dropNullMembers treats a null argument as absent, as the surface does
// everywhere, and drops it: the target refuses an argument it does not take.
func dropNullMembers(members map[string]json.RawMessage, names ...string) {
	for _, name := range names {
		if raw, ok := members[name]; ok && isNull(raw) {
			delete(members, name)
		}
	}
}
