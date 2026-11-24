// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
)

// listedActions are the router actions whose arguments tools/list names. Every
// other action of a router the action catalog describes keeps its enum line,
// and a model reads its arguments from read_workspace topic=actions. The set is
// what the use-case scenarios require; a compose test holds the two together.
var listedActions = map[string][]string{
	"change_access":     {"share"},
	"change_activities": {"check_send", "log", "relink", "relink_many"},
	"change_attention":  {"acknowledge_company_view"},
	"change_reports":    {"record_decision"},
	"change_settings":   {"create_stage", "upload_company_logo"},
	"change_tags":       {"update"},
}

// ListedActions is listedActions, router by router.
func ListedActions() map[string][]string {
	out := maps.Clone(listedActions)
	for router, actions := range out {
		out[router] = slices.Clone(actions)
	}
	return out
}

// rareActions reports which of a router's actions tools/list lists without
// their arguments, and nil when the router lists every action's arguments. Only
// a router the action catalog answers for may leave one out, since the catalog
// is where a model reads them instead.
func (r *Registry) rareActions(routerName, field string) func(choice string) bool {
	r.mu.RLock()
	described := len(r.routeActions[routerName]) > 0
	r.mu.RUnlock()
	if field != actionArg || !described {
		return nil
	}
	listed := listedActions[routerName]
	return func(choice string) bool { return !slices.Contains(listed, choice) }
}

// listedUnkeyed keeps the unkeyed paths under an argument the router lists. A
// path under an argument only rare actions take is not in the listing at all.
func listedUnkeyed(unkeyed map[string]string, schema json.RawMessage) map[string]string {
	listed, err := readObjectSchema("router", schema)
	if err != nil || unkeyed == nil {
		return unkeyed
	}
	maps.DeleteFunc(unkeyed, func(path, _ string) bool {
		argument, _, _ := strings.Cut(strings.TrimPrefix(path, "$."), ".")
		_, kept := listed.Properties[argument]
		return !kept
	})
	if len(unkeyed) == 0 {
		return nil
	}
	return unkeyed
}

// routerChoices names every action of a router, and none for a tool that is
// not one.
func (r *Registry) routerChoices(routerName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rt, isRouter := r.tools[routerName].(router)
	if !isRouter {
		return nil
	}
	return slices.Sorted(maps.Keys(rt.routeTargets()))
}
