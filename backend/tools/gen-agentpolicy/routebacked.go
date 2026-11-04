// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"sort"
)

// routeServed is the served_by value of an operation whose MCP verb runs the
// route's own handler in process, so both doors are one engine.
const routeServed = "route"

// sendRouter is the one router whose route-backed actions may leave the
// installation: an outbound act belongs with the other sends.
const sendRouter = "send_outbound"

// enrichRouter is the one router whose route-backed actions spend the enrich scope.
const enrichRouter = "run_research"

// reasonDefect holds x-human-only-reason to the operations it can describe. A
// human-only operation a signed-in user reaches must say why an agent may not.
// An operation nobody signs in to reach has nobody whose agent it would be.
func reasonDefect(access, reason string, security *[]map[string][]string) string {
	switch {
	case access != "human-only" && reason != "":
		return "x-human-only-reason on an operation that is not x-agent-access: human-only"
	case access == "human-only" && reason == "" && reachedWithASession(security):
		return "human-only operation a signed-in user reaches declares no x-human-only-reason — " +
			"say whether security keeps it from agents, another tool covers it, or it is pending"
	}
	return ""
}

// reachedWithASession reports whether a signed-in user's cookie admits the
// operation. Absent security inherits the document's, which admits it.
func reachedWithASession(security *[]map[string][]string) bool {
	if security == nil {
		return true
	}
	for _, requirement := range *security {
		if _, ok := requirement["cookieAuth"]; ok {
			return true
		}
	}
	return false
}

// routeBackedDefects refuses route-backed rows the in-process door cannot
// serve. It runs only auto-executed calls, because it stages
// nothing, and it never spends the enrich cap, which buys a third party's data.
func routeBackedDefects(policies []policy, routers map[string]bool) []string {
	var defects []string
	actions := map[string]string{}
	scopes := map[string]policy{}
	for _, p := range policies {
		if p.Access != "tool" {
			continue
		}
		bad := func(format string, args ...any) {
			defects = append(defects, fmt.Sprintf("%s (%s): ", p.Route, p.Op)+fmt.Sprintf(format, args...))
		}
		if (p.Router == "") != (p.Action == "") {
			bad("x-mcp-tool names a router and an action together, or neither")
			continue
		}
		if (p.ServedBy == routeServed) != (p.Router != "") {
			bad("x-mcp-tool router and action belong to served_by: route, and served_by: route needs them")
			continue
		}
		if p.Router == "" {
			if p.AskFirst {
				bad("ask_user_first belongs to a route-backed action")
			}
			continue
		}
		if !routers[p.Router] {
			bad("router %s is no router the agents package registers, so no client could reach this action", p.Router)
		}
		key := p.Router + "." + p.Action
		if first, taken := actions[key]; taken {
			bad("action %s is already %s", key, first)
		}
		actions[key] = p.Route
		switch {
		case p.Tier != "auto_execute":
			bad("served_by: route runs at once and stages nothing, so its tier must be auto_execute, not %s", p.Tier)
		case p.Scope == "enrich" && p.Router != enrichRouter:
			bad("a route-backed enrich belongs to %s, not %s", enrichRouter, p.Router)
		case p.Scope == "send" && p.Router != sendRouter:
			bad("a route-backed send belongs to %s, not %s", sendRouter, p.Router)
		}
		if first, seen := scopes[p.Router]; !seen {
			scopes[p.Router] = p
		} else if first.Scope != p.Scope {
			bad("router %s spends %s, but %s spends %s — one router, one scope", p.Router, p.Scope, first.Op, first.Scope)
		}
	}
	return defects
}

// nonJSONBodies refuses a route-backed route whose body is not JSON. The verb
// passes its body argument on as JSON, so a file upload cannot run through it.
func nonJSONBodies(shapes map[string]routeShape) []string {
	var defects []string
	for route, shape := range shapes {
		if shape.body && shape.bodySchema == "" {
			defects = append(defects, route+": served_by: route needs a JSON body; a file upload goes through attach")
		}
	}
	sort.Strings(defects)
	return defects
}
