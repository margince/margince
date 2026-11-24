// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
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
	var asked askedKinds
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
		if v.AskUserFirst {
			asked |= askedKindOf(v.Method == http.MethodGet)
		}
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
		// An action a human approves anyway is not one the model asks about first.
		asks := spec.AskUserFirst && spec.Tier != mcp.TierConfirmationRequired
		if asks {
			asked |= askedKindOf(spec.ReadOnly())
		}
		actions = append(actions, routeActionShape{
			Action: args.Action, Does: spec.Description, Route: routerRunRoute,
			AskUserFirst: asks, Body: spec.InputSchema,
		})
	}
	slices.SortFunc(actions, func(a, b routeActionShape) int { return strings.Compare(a.Action, b.Action) })
	answer := map[string]any{"router": args.Router, "actions": actions}
	if asked != 0 {
		answer[askFirstMeansField] = "Where ask_user_first is true, the user's yes is needed, so tell them " +
			asked.consequence() + " and " + askFirstGetYes
	}
	return json.Marshal(answer)
}

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
