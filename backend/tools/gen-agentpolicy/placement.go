// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// placement is where the contract's own placed operations say an unannotated
// one belongs on MCP: the x-mcp-tool line to write, and the signal behind it.
type placement struct {
	line, because string
}

// unannotatedDefect refuses an operation that names no agent policy, and
// suggests its placement. It only suggests: tier and scope are security
// decisions, so the author writes the annotation.
func unannotatedDefect(target policy, placed []policy, tags map[string][]string) string {
	method, path := routeParts(target.Route)
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s (%s): carries neither x-mcp-tool nor x-agent-access. "+
		"An operation is open to agents unless security is the reason to close it.", method, path, target.Op)
	if suggested, found := suggestPlacement(target, placed, tags); found {
		fmt.Fprintf(&b, "\n    Suggested: x-mcp-tool: %s\n    because: %s", suggested.line, suggested.because)
		if methodScope(method) != "read" {
			b.WriteString("\n    The scope follows the method: make it send if the act delivers to the other party, " +
				"or enrich if it fetches from a third party.")
		}
	} else {
		b.WriteString("\n    No sibling, tag or record type places it: name the verb that serves it on MCP.")
	}
	b.WriteString("\n    If security is the reason an agent must never do this, mark it x-agent-access: human-only " +
		"and give x-human-only-reason: security.")
	return b.String()
}

// suggestPlacement tries each signal in turn, the most specific first.
func suggestPlacement(target policy, placed []policy, tags map[string][]string) (placement, bool) {
	method, path := routeParts(target.Route)
	tools := make([]policy, 0, len(placed))
	for _, p := range placed {
		if p.Access == "tool" {
			tools = append(tools, p)
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Route < tools[j].Route })
	verb := snakeCase(target.Op)
	if found, ok := recordPlacement(method, path, tools); ok {
		return found, true
	}
	if found, ok := siblingPlacement(method, path, verb, tools); ok {
		return found, true
	}
	if found, ok := tagPlacement(method, verb, tags[target.Route], tools, tags); ok {
		return found, true
	}
	if method == "GET" {
		return placement{
			line:    fmt.Sprintf("{ verb: %s, tier: auto_execute, scope: read }", readViewVerb),
			because: fmt.Sprintf("no sibling or tag places this read, and %s serves every read with no verb of its own.", readViewVerb),
		}, true
	}
	return placement{}, false
}

// recordPlacement places a create, change, archive or read of a record type:
// a sibling in the record's collection names the type, and the verb is the one
// the contract most often gives that method on that shape of record path.
func recordPlacement(method, path string, tools []policy) (placement, bool) {
	collection, item := recordShape(path)
	recordType, source := "", ""
	for _, p := range tools {
		if otherCollection, _ := recordShape(routePath(p.Route)); p.RecordType != "" && otherCollection == collection {
			recordType, source = p.RecordType, p.Route
			break
		}
	}
	if recordType == "" {
		return placement{}, false
	}
	verbs := map[string]int{}
	for _, p := range tools {
		otherMethod, otherPath := routeParts(p.Route)
		if _, otherItem := recordShape(otherPath); p.RecordType != "" && otherMethod == method && otherItem == item {
			verbs[p.Tool]++
		}
	}
	verb, uses := mostUsed(verbs)
	if verb == "" {
		return placement{}, false
	}
	shape := map[bool]string{true: "item", false: "collection"}[item]
	return placement{
		line: fmt.Sprintf("{ verb: %s, record_type: %s, tier: auto_execute, scope: %s }", verb, recordType, methodScope(method)),
		because: fmt.Sprintf("%s declares record_type %s, and %d %s operations on a record %s use %s.",
			routeDisplay(source), recordType, uses, method, shape, verb),
	}, true
}

// siblingPlacement places an operation beside a routed one on its own path:
// in the sibling's router when it spends the same scope, else in the router
// the contract most often pairs with it on a shared path.
func siblingPlacement(method, path, verb string, tools []policy) (placement, bool) {
	scope := methodScope(method)
	var sibling *policy
	for i, p := range tools {
		if routePath(p.Route) != path || p.Router == "" {
			continue
		}
		if p.Scope == scope {
			return routedPlacement(verb, p.Router, scope,
				fmt.Sprintf("%s is served by %s.", routeDisplay(p.Route), p.Router)), true
		}
		if sibling == nil {
			sibling = &tools[i]
		}
	}
	if sibling == nil {
		return placement{}, false
	}
	routers := map[string]int{}
	for _, shared := range pathsServedBy(sibling.Router, tools) {
		for _, p := range tools {
			if routePath(p.Route) == shared && p.Router != "" && p.Scope == scope {
				routers[p.Router]++
			}
		}
	}
	router, uses := mostUsed(routers)
	if router == "" {
		return placement{}, false
	}
	because := fmt.Sprintf("%s is served by %s, and %s serves the %s side on %d of the paths %s serves.",
		routeDisplay(sibling.Route), sibling.Router, router, scope, uses, sibling.Router)
	return routedPlacement(verb, router, scope, because), true
}

// tagPlacement places an operation in the router its tag most often uses for
// the same method.
func tagPlacement(method, verb string, targetTags []string, tools []policy, tags map[string][]string) (placement, bool) {
	scope := methodScope(method)
	counts := map[string]int{}
	for _, p := range tools {
		if otherMethod, _ := routeParts(p.Route); p.Router == "" || otherMethod != method || p.Scope != scope {
			continue
		}
		for _, tag := range tags[p.Route] {
			if contains(targetTags, tag) {
				counts[tag+" "+p.Router]++
			}
		}
	}
	key, uses := mostUsed(counts)
	if key == "" {
		return placement{}, false
	}
	tag, router, _ := strings.Cut(key, " ")
	return routedPlacement(verb, router, scope,
		fmt.Sprintf("%d other %s operations in the %s tag use %s.", uses, method, tag, router)), true
}

// routedPlacement writes a route-backed line. A read router's action drops the
// verb's leading word (get_contact_brief is contact_brief), as most do.
func routedPlacement(verb, router, scope, because string) placement {
	action := verb
	if _, rest, cut := strings.Cut(verb, "_"); cut && scope == "read" {
		action = rest
	}
	return placement{
		line: fmt.Sprintf("{ verb: %s, router: %s, action: %s, tier: auto_execute, scope: %s, served_by: %s }",
			verb, router, action, scope, routeServed),
		because: because,
	}
}

// pathsServedBy lists the paths any of whose operations the router serves.
func pathsServedBy(router string, tools []policy) []string {
	var paths []string
	for _, p := range tools {
		if path := routePath(p.Route); p.Router == router && !contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// mostUsed picks the key with the highest count, the first by name on a tie,
// so the suggestion does not depend on map order.
func mostUsed(counts map[string]int) (string, int) {
	best, uses := "", 0
	for key, n := range counts {
		if n > uses || (n == uses && key < best) {
			best, uses = key, n
		}
	}
	return best, uses
}

// methodScope is the scope the method suggests; the author still chooses send
// or enrich for an act that delivers or fetches.
func methodScope(method string) string {
	if method == "GET" {
		return "read"
	}
	return "write"
}

// recordShape names a path's record collection and whether the path is one
// item of it: /companies/{id} is an item of /companies.
func recordShape(path string) (string, bool) {
	parent, last, found := cutLast(path, "/")
	if found && strings.HasPrefix(last, "{") {
		return parent, true
	}
	return path, false
}

func cutLast(s, sep string) (string, string, bool) {
	if i := strings.LastIndex(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, "", false
}

// routeParts splits a policy route into its method and its contract path.
func routeParts(route string) (string, string) {
	method, path, _ := strings.Cut(route, " ")
	return method, strings.TrimPrefix(path, "/v1")
}

func routePath(route string) string {
	_, path := routeParts(route)
	return path
}

func routeDisplay(route string) string {
	method, path := routeParts(route)
	return method + " " + path
}

// snakeCase spells an operationId as a verb: updateContactCaseSettings is
// update_contact_case_settings, and importVCards is import_vcards.
func snakeCase(operationID string) string {
	var b strings.Builder
	runes := []rune(operationID)
	for i, r := range runes {
		if unicode.IsUpper(r) && i > 0 && (unicode.IsLower(runes[i-1]) || unicode.IsDigit(runes[i-1])) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
