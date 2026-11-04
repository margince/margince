// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// RouteVerb is one REST operation an agent reaches as a router's action. The
// verb runs that operation's own handler, so the answer is the route's.
type RouteVerb struct {
	// Verb is the hidden tool's name: the approval kind and retry key.
	Verb string
	// Router and Action are the listed tool that fronts the verb and the
	// choice that picks it.
	Router, Action string
	// Op and Summary are the operation's id and its one-line summary.
	Op, Summary string
	// Method and Pattern are the route, path parameters in braces.
	Method, Pattern string
	Scope           principal.Scope
	PathParams      []string
	Query           []ViewQuery
	// IfMatch is true when the route takes the record's version.
	IfMatch bool
	// Body is true when the route takes a JSON body, whose schema BodySchema
	// is, $refs resolved.
	Body       bool
	BodySchema json.RawMessage
	// AnswerSchema is the route's 2xx answer schema, as gen-agentpolicy
	// derives it: the {status} a body-less answer becomes, where it has none.
	AnswerSchema json.RawMessage
	// AskUserFirst is true when the app confirms the act before it runs.
	AskUserFirst bool
}

// RouteCall is one route-backed call as its route takes it.
type RouteCall struct {
	Verb    RouteVerb
	Path    map[string]string
	Query   url.Values
	Version string
	Body    json.RawMessage
}

// RouteRunner runs one call through its route as the caller.
type RouteRunner func(ctx context.Context, call RouteCall) (json.RawMessage, error)

// The arguments a route verb takes beside its route's own parameters.
const (
	routeBodyArg    = "request_body"
	routeVersionArg = "if_version"
)

// RouteAnswerTruncated is the member a write's answer carries when its body was
// too long to return. The write has committed, so the caller must not retry.
const RouteAnswerTruncated = "truncated"

// writeAnswerSchema admits the two members of a write's cut answer beside the
// route's own, and so can require none of the route's.
func writeAnswerSchema(answer json.RawMessage) json.RawMessage {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(answer, &shape); err != nil {
		return answer
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(shape["properties"], &properties); err != nil || properties == nil {
		properties = map[string]json.RawMessage{}
	}
	properties["status"] = json.RawMessage(`{"type":"integer"}`)
	properties[RouteAnswerTruncated] = json.RawMessage(`{"type":"boolean"}`)
	shape["properties"] = mustMarshal(properties)
	delete(shape, "required")
	return mustMarshal(shape)
}

// RegisterRouteVerbs registers every route-backed verb, unlisted, and records
// which router fronts each. It runs before the routers register.
func RegisterRouteVerbs(r *Registry, verbs []RouteVerb, run RouteRunner) {
	if len(verbs) == 0 {
		return
	}
	r.Register(describeRouterActions{verbs: verbs})
	r.mu.Lock()
	r.routeActions[readWorkspaceName] = map[string]string{routeActionsTopic: describeRouterActionsName}
	r.mu.Unlock()
	for _, v := range verbs {
		r.Register(routeVerbTool{verb: v, run: run})
		r.mu.Lock()
		if r.routeActions[v.Router] == nil {
			r.routeActions[v.Router] = map[string]string{}
		}
		r.routeActions[v.Router][v.Action] = v.Verb
		r.mu.Unlock()
	}
}

type routeVerbTool struct {
	verb RouteVerb
	run  RouteRunner
}

func (t routeVerbTool) Spec() mcp.ToolSpec {
	v := t.verb
	properties := map[string]string{}
	for _, p := range v.PathParams {
		properties[p] = `{"type":"string"}`
	}
	// A router refuses to unite two actions that give one name two shapes. A list
	// published here fails at boot if another action takes that name as one value.
	for _, q := range v.Query {
		properties[q.Name] = `{"type":` + strconv.Quote(q.Type) + `}`
		if q.Type == schemaArray {
			properties[q.Name] = `{"type":"array","items":{"type":"string"}}`
		}
	}
	if v.IfMatch {
		properties[routeVersionArg] = `{"type":"integer","description":"The version the caller read; the write is refused as skew if the record moved since"}`
	}
	var unkeyed map[string]string
	if v.Body {
		properties[routeBodyArg] = `{"type":"object","description":"The request body of ` + v.Op + `"}`
		unkeyed = map[string]string{"$." + routeBodyArg: "the body of " + v.Method + " " + v.Pattern + ", as its operation " + v.Op + " defines it"}
	}
	members := make([]string, 0, len(properties))
	for _, name := range slices.Sorted(maps.Keys(properties)) {
		members = append(members, strconv.Quote(name)+":"+properties[name])
	}
	required := `[]`
	if len(v.PathParams) > 0 {
		required = mustJSON(v.PathParams)
	}
	egress := v.Scope.Egresses()
	answer := v.AnswerSchema
	if v.Method != http.MethodGet {
		answer = writeAnswerSchema(answer)
	}
	return mcp.ToolSpec{
		Name: v.Verb, Title: v.Summary, Version: toolVersionV1,
		Description:   v.Summary + " (" + v.Method + " " + v.Pattern + ", as the app does it.)",
		RequiredScope: v.Scope, Tier: mcp.TierAutoExecute, Egress: egress, AskUserFirst: v.AskUserFirst,
		Unlisted:         true,
		OpenAPIOp:        v.Op,
		UnkeyedArguments: unkeyed,
		InputSchema: schema(`{"type":"object","required":` + required + `,"properties":{` +
			strings.Join(members, ",") + `},"additionalProperties":false}`),
		OutputSchema: answer,
	}
}

func (t routeVerbTool) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args map[string]json.RawMessage
	if err := json.Unmarshal(in, &args); err != nil {
		return nil, &BadArgsError{Cause: fmt.Errorf("arguments must be a JSON object: %w", err)}
	}
	v := t.verb
	call := RouteCall{Verb: v, Path: map[string]string{}, Query: url.Values{}}
	for key, raw := range args {
		if err := v.bind(key, raw, &call); err != nil {
			return nil, err
		}
	}
	for _, param := range v.PathParams {
		if call.Path[param] == "" {
			return nil, &BadArgsError{Field: param, Cause: fmt.Errorf("%s needs %s", v.Verb, param)}
		}
	}
	return t.run(ctx, call)
}

// bind puts one argument where the verb's route takes it, and refuses one the
// verb does not publish rather than dropping it.
func (v RouteVerb) bind(key string, raw json.RawMessage, call *RouteCall) error {
	switch {
	case key == routeBodyArg && v.Body:
		call.Body = raw
		return nil
	case key == routeVersionArg && v.IfMatch:
		version, err := operand(raw)
		if err != nil {
			return &BadArgsError{Field: key, Cause: err}
		}
		call.Version = version
		return nil
	case slices.Contains(v.PathParams, key):
		value, err := operand(raw)
		if err != nil {
			return &BadArgsError{Field: key, Cause: err}
		}
		call.Path[key] = value
		return nil
	}
	idx := slices.IndexFunc(v.Query, func(q ViewQuery) bool { return q.Name == key })
	if idx < 0 {
		return &BadArgsError{Field: key, Cause: fmt.Errorf("%s takes %s", v.Verb, strings.Join(v.arguments(), ", "))}
	}
	var members []json.RawMessage
	if v.Query[idx].Type == schemaArray && json.Unmarshal(raw, &members) == nil {
		for _, member := range members {
			value, err := operand(member)
			if err != nil {
				return &BadArgsError{Field: key, Cause: err}
			}
			call.Query.Add(key, value)
		}
		return nil
	}
	value, err := operand(raw)
	if err != nil {
		return &BadArgsError{Field: key, Cause: err}
	}
	call.Query.Set(key, value)
	return nil
}

// arguments names what the verb publishes, for a refusal of anything else.
func (v RouteVerb) arguments() []string {
	names := slices.Clone(v.PathParams)
	for _, q := range v.Query {
		names = append(names, q.Name)
	}
	if v.IfMatch {
		names = append(names, routeVersionArg)
	}
	if v.Body {
		names = append(names, routeBodyArg)
	}
	if len(names) == 0 {
		return []string{"no arguments"}
	}
	return names
}

// contractRouter is a listed router whose every action the contract declares
// as a route-backed verb.
type contractRouter struct {
	routed
	spec mcp.ToolSpec
}

func (c contractRouter) Spec() mcp.ToolSpec { return c.spec }

func (contractRouter) routeField() string { return routeActionField }

func (contractRouter) routeTargets() map[string]string { return map[string]string{} }

// routeActionField is the argument a contract router picks its action by.
const routeActionField = "action"

// withRouteActions is a router with the contract's actions for it added to its
// own. A choice spelled both ways is a composition defect.
//
//nolint:ireturn // a router is served behind this interface, whichever way it picks its target.
func (r *Registry) withRouteActions(rt router) router {
	r.mu.RLock()
	extra := r.routeActions[rt.Spec().Name]
	r.mu.RUnlock()
	if len(extra) == 0 {
		return rt
	}
	if rt.Spec().UI != nil {
		//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
		panic(fmt.Sprintf("crmagents: router %s has a view, whose published result unites its targets' shapes, so it cannot take contract actions", rt.Spec().Name))
	}
	targets := maps.Clone(rt.routeTargets())
	for choice, verb := range extra {
		if _, taken := targets[choice]; taken {
			//craft:ignore panic-in-domain composition-time registration assertion — fires only while cmd wiring runs, never on a request path
			panic(fmt.Sprintf("crmagents: router %s already has action %s", rt.Spec().Name, choice))
		}
		targets[choice] = verb
	}
	if chooser, chooses := rt.(targetChooser); chooses {
		return extendedChooser{router: rt, targetChooser: chooser, targets: targets}
	}
	field, _ := routeFieldOf(rt)
	return extendedRouter{router: rt, targets: targets, field: field}
}

// extendedRouter serves a router's own targets and the contract's.
type extendedRouter struct {
	router
	targets map[string]string
	field   string
}

func (e extendedRouter) routeTargets() map[string]string { return e.targets }

func (e extendedRouter) routeField() string { return e.field }

// extendedChooser is the same for a router that picks by the arguments named.
type extendedChooser struct {
	router
	targetChooser
	targets map[string]string
}

func (e extendedChooser) routeTargets() map[string]string { return e.targets }

// The read_workspace topic that lists each route-backed action's body.
const (
	routeActionsTopic         = "actions"
	describeRouterActionsName = "describe_router_actions"
)

// describeRouterActions answers what each action of one router takes, so a
// router's own listing stays one enum and one body object.
type describeRouterActions struct{ verbs []RouteVerb }

func (d describeRouterActions) routers() []string {
	var names []string
	for _, v := range d.verbs {
		names = append(names, v.Router)
	}
	return slices.Compact(slices.Sorted(slices.Values(names)))
}

func (d describeRouterActions) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: describeRouterActionsName, Title: "Describe a router's actions", Version: toolVersionV1,
		Description: "Lists what each action of one router takes: its path ids, if_version, and the " +
			"JSON schema of its body, as the app's own route defines them.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: "describeRouterActions",
		OutputSchema: schema(`{"type":"object","required":["router","actions"],"properties":{` +
			`"router":{"type":"string"},"actions":{"type":"array","items":{"type":"object",` +
			`"required":["action","does","route"],"properties":{"action":{"type":"string"},` +
			`"does":{"type":"string"},"route":{"type":"string"},"ids":{"type":"array","items":{"type":"string"}},` +
			`"query":{"type":"array","items":{"type":"string"}},"if_version":{"type":"boolean"},` +
			`"ask_user_first":{"type":"boolean"},"body":{"type":"object"}}}}}}`),
		InputSchema: schema(`{"type":"object","required":["router"],"properties":{"router":{"type":"string","enum":` +
			mustJSON(d.routers()) + `}},"additionalProperties":false}`),
	}
}

type routeActionShape struct {
	Action       string          `json:"action"`
	Does         string          `json:"does"`
	Route        string          `json:"route"`
	IDs          []string        `json:"ids,omitempty"`
	Query        []string        `json:"query,omitempty"`
	IfVersion    bool            `json:"if_version,omitempty"`
	AskUserFirst bool            `json:"ask_user_first,omitempty"`
	Body         json.RawMessage `json:"body,omitempty"`
}

func (d describeRouterActions) Handle(_ context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Router string `json:"router"`
	}
	if err := json.Unmarshal(in, &args); err != nil || !slices.Contains(d.routers(), args.Router) {
		return nil, &BadArgsError{Field: "router", Cause: fmt.Errorf("router must be one of %s", strings.Join(d.routers(), ", "))}
	}
	actions := []routeActionShape{}
	for _, v := range d.verbs {
		if v.Router != args.Router {
			continue
		}
		shape := routeActionShape{
			Action: v.Action, Does: v.Summary, Route: v.Method + " " + v.Pattern, IDs: v.PathParams,
			IfVersion: v.IfMatch, AskUserFirst: v.AskUserFirst,
		}
		for _, q := range v.Query {
			shape.Query = append(shape.Query, q.Name)
		}
		if v.Body && len(v.BodySchema) > 0 {
			shape.Body = v.BodySchema
		}
		actions = append(actions, shape)
	}
	slices.SortFunc(actions, func(a, b routeActionShape) int { return strings.Compare(a.Action, b.Action) })
	return json.Marshal(map[string]any{"router": args.Router, "actions": actions})
}
