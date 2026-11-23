// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
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
	return describe.catalog(router, "")
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

// catalog is the answer for one router, or for one of its actions.
func (d describeRouterActions) catalog(router, action string) (json.RawMessage, error) {
	args := struct{ Router, Action string }{router, action}
	if !slices.Contains(d.routers(), args.Router) {
		return nil, &BadArgsError{Field: catalogRouterArg, Cause: fmt.Errorf("router must be one of %s", strings.Join(d.routers(), ", "))}
	}
	actions := []routeActionShape{}
	for _, v := range d.verbs {
		if v.Router != args.Router || (args.Action != "" && v.Action != args.Action) {
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
		shape.UploadsAFile = v.UploadField != ""
		actions = append(actions, shape)
	}
	if args.Action != "" && len(actions) == 0 {
		// A client that reads tool schemas from a file has no inputSchema at hand.
		// An action the router runs itself is described from its own spec.
		spec, runs := d.runs(args.Router, args.Action)
		if !runs {
			return nil, &BadArgsError{Field: "action", Cause: fmt.Errorf(
				"router %s has no action %s; read_workspace with topic=actions and this router lists the ones it has",
				args.Router, args.Action)}
		}
		actions = append(actions, routeActionShape{
			Action: args.Action, Does: spec.Description, Route: routerRunRoute,
			AskUserFirst: spec.AskUserFirst, Body: spec.InputSchema,
		})
	}
	slices.SortFunc(actions, func(a, b routeActionShape) int { return strings.Compare(a.Action, b.Action) })
	answer := map[string]any{"router": args.Router, "actions": actions}
	if slices.ContainsFunc(actions, func(a routeActionShape) bool { return a.AskUserFirst }) {
		answer[askFirstMeansField] = "Where ask_user_first is true, the user's yes is needed, so tell them what the " +
			"call will change and " + askFirstGetYes
	}
	return json.Marshal(answer)
}

// askFirstMeansField says what ask_user_first asks of the model, beside the
// actions, because a model reads the flag bare otherwise and drops its exception.
const askFirstMeansField = "ask_user_first_means"
