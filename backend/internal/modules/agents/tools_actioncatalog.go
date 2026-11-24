// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// catalogRouterArg names the router whose actions the catalog describes.
const catalogRouterArg = "router"

// ActionCatalog is what read_workspace topic=actions answers for router, by the
// describe tool's own catalog. docs/reference/mcp-info.json publishes it, and
// the MCP end-to-end stamps hash it per router.
func (r *Registry) ActionCatalog(router string) (json.RawMessage, error) {
	r.mu.RLock()
	describe, ok := r.tools[describeRouterActionsName].(describeRouterActions)
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no route verb is registered, so %s is not served", describeRouterActionsName)
	}
	return describe.catalog(router, "", nil)
}

// offeredActions says which of a router's actions this caller's tools/list
// offers, and false when it offers the router not at all.
func (r *Registry) offeredActions(ctx context.Context, router string) (func(action string) bool, bool) {
	spec, ok := r.offeredOne(ctx, router)
	if !ok {
		return nil, false
	}
	return func(action string) bool {
		return slices.ContainsFunc(spec.Actions, func(a mcp.ToolAction) bool { return a.Action == action })
	}, true
}

// ActionCatalogRouters names the routers ActionCatalog answers for.
func (r *Registry) ActionCatalogRouters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if describe, ok := r.tools[describeRouterActionsName].(describeRouterActions); ok {
		return describe.routers()
	}
	return nil
}

// catalog is the answer for one router, or for one of its actions. seen, when
// set, keeps only the actions a caller is offered.
func (d describeRouterActions) catalog(router, action string, seen func(string) bool) (json.RawMessage, error) {
	args := struct{ Router, Action string }{router, action}
	if !slices.Contains(d.routers(), args.Router) {
		return nil, &BadArgsError{Field: catalogRouterArg, Cause: fmt.Errorf("router must be one of %s", strings.Join(d.routers(), ", "))}
	}
	actions := []routeActionShape{}
	var asked askedKinds
	for _, v := range d.verbs {
		if v.Router != args.Router || (args.Action != "" && v.Action != args.Action) || (seen != nil && !seen(v.Action)) {
			continue
		}
		shape := routeActionShape{
			Action: v.Action, Does: v.Summary, Route: v.Method + " " + v.Pattern, IDs: v.PathParams,
			IfVersion: v.IfMatch, AskUserFirst: v.AskUserFirst, Query: v.Query,
		}
		if v.Body && len(v.BodySchema) > 0 {
			shape.Body = v.BodySchema
		}
		shape.UploadsAFile = v.UploadField != ""
		if v.AskUserFirst {
			asked |= askedKindOf(v.Method == http.MethodGet)
		}
		actions = append(actions, shape)
	}
	// An action the router runs itself has no route, so it is described from its
	// own spec. Its arguments are named here even where tools/list leaves them out.
	ranInProcess, err := d.inProcess(args.Router, args.Action, seen, actions, &asked)
	if err != nil {
		return nil, err
	}
	actions = append(actions, ranInProcess...)
	slices.SortFunc(actions, func(a, b routeActionShape) int { return strings.Compare(a.Action, b.Action) })
	answer := map[string]any{"router": args.Router, "actions": actions, howToCallField: howToCall}
	if asked != 0 {
		answer[askFirstMeansField] = "Where ask_user_first is true, the user's yes is needed, so tell them " +
			asked.consequence() + " and " + askFirstGetYes
	}
	return json.Marshal(answer)
}

// inProcess describes the actions router runs itself, or the one named. It
// refuses a named action the router has under neither kind.
func (d describeRouterActions) inProcess(router, action string, seen func(string) bool, routed []routeActionShape, asked *askedKinds) ([]routeActionShape, error) {
	choices := []string{action}
	if action == "" {
		choices = nil
		if d.choices != nil {
			choices = d.choices(router)
		}
	}
	var out []routeActionShape
	for _, choice := range choices {
		if slices.ContainsFunc(routed, func(a routeActionShape) bool { return a.Action == choice }) {
			continue
		}
		spec, runs := d.runs(router, choice)
		runs = runs && (seen == nil || seen(choice))
		if !runs && action != "" {
			return nil, &BadArgsError{Field: "action", Cause: fmt.Errorf(
				"router %s has no action %s; read_workspace with topic=actions and this router lists the ones it has",
				router, action)}
		}
		if !runs {
			continue
		}
		// An action a human approves anyway is not one the model asks about first.
		asks := spec.AskUserFirst && spec.Tier != mcp.TierConfirmationRequired
		if asks {
			*asked |= askedKindOf(spec.ReadOnly())
		}
		out = append(out, routeActionShape{
			Action: choice, Does: spec.Description, Route: routerRunRoute,
			AskUserFirst: asks, Body: spec.InputSchema,
		})
	}
	return out, nil
}

// howToCall says where each part of an action's arguments goes in the router
// call, once per answer, since a model nests a top-level argument otherwise.
const (
	howToCallField = "how_to_call"
	howToCall      = "Call the router with action and the action's arguments as top-level members. ids, query " +
		"and if_version are top-level members too. Only a routed action's body goes inside request_body; " +
		"an in-process action's body members are top-level members."
)

// askedKinds is which ask-first actions a router has: ones that write, ones
// that only read, or both.
type askedKinds uint8

const (
	askedWrites askedKinds = 1 << iota
	askedReads
)

func askedKindOf(readOnly bool) askedKinds {
	if readOnly {
		return askedReads
	}
	return askedWrites
}

// consequence is what the user is told before the yes.
func (k askedKinds) consequence() string {
	switch k {
	case askedReads:
		return askConsequenceRead
	case askedWrites | askedReads:
		return "what the call will change or surface"
	}
	return askConsequenceWrite
}

// askFirstMeansField says what ask_user_first asks of the model, beside the
// actions, because a model reads the flag bare otherwise and drops its exception.
const askFirstMeansField = "ask_user_first_means"
