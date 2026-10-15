// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A passport has the same functionality on both doors. So each path and query
// parameter of an agent route is an argument of its x-mcp-tool verb, top level
// or under filters. Paging is the one exception: MCP keeps smaller pages, and
// a cursor reaches the same records.
func TestEveryAgentRouteParameterIsAnArgumentOfItsVerb(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	arguments := map[string]map[string]bool{}
	for _, spec := range registry.Specs() {
		arguments[spec.Name] = argumentNames(t, spec.Name, spec.InputSchema)
	}
	params := routeParameters(t)
	var gaps []string
	checked := 0
	for route, policy := range agentPolicies {
		args, registered := arguments[policy.Tool]
		if policy.Access != accessTool || !registered {
			continue // TestEveryDeclaredToolVerbIsRegistered owns an unregistered verb.
		}
		names, known := params[route]
		if !known {
			t.Fatalf("the contract walk did not reach %s, which the policy table carries; the census is short", route)
		}
		for _, p := range names {
			checked++
			if !slices.ContainsFunc(argumentsFor(p, args), func(arg string) bool { return args[arg] }) {
				gaps = append(gaps, route+"#"+p.name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no agent route declares a parameter — this sweep checked nothing")
	}
	sort.Strings(gaps)
	for _, gap := range gaps {
		if slices.Contains(parameterGapsPendingParity, gap) {
			continue
		}
		route, name, _ := strings.Cut(gap, "#")
		t.Errorf("%s takes %q, and its verb %s has no argument for it, so a passport can ask REST what "+
			"MCP cannot. Add the argument to the verb (top level or under filters), or point the route at "+
			"a verb that takes it.", route, name, agentPolicies[route].Tool)
	}
	for _, waived := range parameterGapsPendingParity {
		if !slices.Contains(gaps, waived) {
			t.Errorf("%s is waived but no longer a gap; remove it from parameterGapsPendingParity", waived)
		}
	}
}

// A route is admitted against its verb's own spec (operationSpec). A router has
// no tier of its own to admit with, because it runs as the target a call picks.
// So a route names the verb that serves it, never the router in front.
func TestNoRouteNamesARouter(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	routers := 0
	for _, spec := range registry.Specs() {
		if _, isRouter := registry.RouteTargets(spec.Name); isRouter {
			routers++
		}
	}
	if routers == 0 {
		t.Fatal("the registry has no router — this sweep checked nothing")
	}
	for route, policy := range agentPolicies {
		if targets, isRouter := registry.RouteTargets(policy.Tool); isRouter {
			t.Errorf("%s declares x-mcp-tool verb %s, which is a router; name the verb it fronts that "+
				"serves this route (one of %v)", route, policy.Tool, targets)
		}
	}
}

// pagingParameters are the ones a verb may answer with its own page size.
var pagingParameters = []string{"cursor", "limit"}

// argumentsFor names the arguments a route parameter may be asked through. A
// query parameter keeps its name. A route's own {id} is the record it acts on,
// and every verb names that record through an id argument (list_id). A nested
// path parameter ({lineItemId}) must be an argument of its own name.
func argumentsFor(p routeParameter, args map[string]bool) []string {
	if p.in != "path" {
		return []string{p.name}
	}
	if p.name == "id" {
		var addresses []string
		for arg := range args {
			if arg == "id" || strings.HasSuffix(arg, "_id") {
				addresses = append(addresses, arg)
			}
		}
		return addresses
	}
	return []string{p.name, snakeCase(p.name)}
}

func snakeCase(name string) string {
	var b strings.Builder
	for i, r := range name {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// argumentNames reads the arguments a verb's input schema takes: its top-level
// properties and the properties of each top-level object (filters, selection).
func argumentNames(t *testing.T, tool string, schema json.RawMessage) map[string]bool {
	t.Helper()
	type properties map[string]json.RawMessage
	var parsed struct {
		Properties properties `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("tool %q publishes an input schema that is not JSON: %v", tool, err)
	}
	names := map[string]bool{}
	for name, raw := range parsed.Properties {
		names[name] = true
		var nested struct {
			Properties properties `json:"properties"`
		}
		if err := json.Unmarshal(raw, &nested); err != nil {
			t.Fatalf("tool %q publishes argument %q whose schema is not JSON: %v", tool, name, err)
		}
		for inner := range nested.Properties {
			names[inner] = true
		}
	}
	return names
}

// routeParameters reads the path and query parameter names of every operation
// in api/crm.yaml, keyed "METHOD /v1/route" as agentPolicies is, with paging
// left out. A parameter given by $ref is read from components.parameters.
type routeParameter struct{ name, in string }

func routeParameters(t *testing.T) map[string][]routeParameter {
	t.Helper()
	src, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	type parameter struct {
		Ref  string `yaml:"$ref"`
		Name string `yaml:"name"`
		In   string `yaml:"in"`
	}
	var doc struct {
		Paths      map[string]map[string]yaml.Node `yaml:"paths"`
		Components struct {
			Parameters map[string]parameter `yaml:"parameters"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	resolve := func(p parameter) parameter {
		if p.Ref == "" {
			return p
		}
		shared, ok := doc.Components.Parameters[strings.TrimPrefix(p.Ref, "#/components/parameters/")]
		if !ok {
			t.Fatalf("parameter $ref %s names no components.parameters entry", p.Ref)
		}
		return shared
	}
	byRoute := map[string][]routeParameter{}
	for path, item := range doc.Paths {
		var shared struct {
			Parameters []parameter `yaml:"parameters"`
		}
		if node, ok := item["parameters"]; ok {
			if err := node.Decode(&shared.Parameters); err != nil {
				t.Fatalf("%s parameters: %v", path, err)
			}
		}
		for _, key := range httpMethods {
			node, present := item[key]
			if !present {
				continue
			}
			var op struct {
				Parameters []parameter `yaml:"parameters"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", key, path, err)
			}
			route := strings.ToUpper(key) + " /v1" + path
			byRoute[route] = []routeParameter{}
			for _, p := range append(slices.Clone(shared.Parameters), op.Parameters...) {
				p = resolve(p)
				if (p.In == "path" || p.In == "query") && !slices.Contains(pagingParameters, p.Name) {
					byRoute[route] = append(byRoute[route], routeParameter{name: p.Name, in: p.In})
				}
			}
		}
	}
	return byRoute
}

// parameterGapsPendingParity are the route parameters no verb takes yet, as
// "METHOD /v1/route#parameter". The list only shrinks: an entry that stops
// being a gap fails the test above.
var parameterGapsPendingParity = []string{
	"DELETE /v1/companies/{id}/facts/{factKey}#factKey",
	"DELETE /v1/offers/{id}/line-items/{lineItemId}#lineItemId",
	"DELETE /v1/projects/{id}/companies/{company_id}#company_id",
	"DELETE /v1/projects/{id}/stakeholders/{contact_id}#contact_id",
	"GET /v1/activities#assignee_id",
	"GET /v1/activities#channel_provider",
	"GET /v1/activities#entity_id",
	"GET /v1/activities#entity_type",
	"GET /v1/activities#include_archived",
	"GET /v1/activities#kind",
	"GET /v1/activities#occurred_after",
	"GET /v1/activities#occurred_before",
	"GET /v1/activities#project_id",
	"GET /v1/activities#sort",
	"GET /v1/activities#thread_key",
	"GET /v1/activities#waiting_reply",
	"GET /v1/activities/{id}/email-presentation#thread_cursor",
	"GET /v1/activities/{id}/transcript-proposals/{readId}#readId",
	"GET /v1/analytics/editions/compare#left_id",
	"GET /v1/analytics/evaluate#end_at",
	"GET /v1/analytics/evaluate#scope_id",
	"GET /v1/analytics/evaluate#scope_kind",
	"GET /v1/analytics/evaluate#start_at",
	"GET /v1/analytics/evidence#end_at",
	"GET /v1/analytics/evidence#scope_id",
	"GET /v1/analytics/evidence#scope_kind",
	"GET /v1/analytics/evidence#start_at",
	"GET /v1/analytics/reports/{id}/evaluation#revision",
	"GET /v1/approvals#bundle_id",
	"GET /v1/approvals#target_entity_id",
	"GET /v1/approvals#target_entity_type",
	"GET /v1/commissions#deal_id",
	"GET /v1/companies#ai_written",
	"GET /v1/companies#captured_by_kind",
	"GET /v1/companies#id",
	"GET /v1/companies#include_anchor",
	"GET /v1/companies#include_archived",
	"GET /v1/companies#industry",
	"GET /v1/companies#list_id",
	"GET /v1/companies#owner_team_id",
	"GET /v1/companies#q",
	"GET /v1/companies#size_band",
	"GET /v1/companies#sort",
	"GET /v1/companies#unassigned",
	"GET /v1/companies/{id}/documents#category",
	"GET /v1/companies/{id}/documents#contract_id",
	"GET /v1/companies/{id}/documents#doc_state",
	"GET /v1/companies/{id}/documents#pinned_only",
	"GET /v1/companies/{id}/site-reads/{readId}#readId",
	"GET /v1/contacts#ai_written",
	"GET /v1/contacts#captured_by_kind",
	"GET /v1/contacts#include_archived",
	"GET /v1/contacts#list_id",
	"GET /v1/contacts#owner_team_id",
	"GET /v1/contacts#q",
	"GET /v1/contacts#sort",
	"GET /v1/contacts#unassigned",
	"GET /v1/custom-fields#object",
	"GET /v1/custom-fields#sort",
	"GET /v1/custom-fields#status",
	"GET /v1/deal-rooms#deal_id",
	"GET /v1/deal-rooms#include_archived",
	"GET /v1/deal-rooms#participant_email",
	"GET /v1/deal-rooms#sort",
	"GET /v1/deal-rooms#state",
	"GET /v1/deal-rooms/{id}/documents#id",
	"GET /v1/deal-rooms/{id}/participants#active_only",
	"GET /v1/deal-rooms/{id}/participants#id",
	"GET /v1/deal-rooms/{id}/threads#document_id",
	"GET /v1/deal-rooms/{id}/threads#id",
	"GET /v1/deals#include_archived",
	"GET /v1/deals#list_id",
	"GET /v1/deals#owner_team_id",
	"GET /v1/deals#q",
	"GET /v1/deals#sort",
	"GET /v1/deals#unassigned",
	"GET /v1/deals/{id}/documents#category",
	"GET /v1/deals/{id}/documents#include_hidden",
	"GET /v1/deals/{id}/offers#id",
	"GET /v1/deals/{id}/offers#status",
	"GET /v1/deals/{id}/status#facts_only",
	"GET /v1/deals/{id}/status#refresh",
	"GET /v1/imports/format#lang",
	"GET /v1/leads#ai_written",
	"GET /v1/leads#captured_by_kind",
	"GET /v1/leads#include_archived",
	"GET /v1/leads#list_id",
	"GET /v1/leads#owner_team_id",
	"GET /v1/leads#q",
	"GET /v1/leads#sla_state",
	"GET /v1/leads#sort",
	"GET /v1/leads#source",
	"GET /v1/leads#unassigned",
	"GET /v1/lists#include_archived",
	"GET /v1/lists#list_type",
	"GET /v1/lists#q",
	"GET /v1/lists/{id}/members#entity_id",
	"GET /v1/offer-templates#include_archived",
	"GET /v1/offer-templates#locale",
	"GET /v1/offer-templates#sort",
	"GET /v1/partners#cert_status",
	"GET /v1/partners#partner_role",
	"GET /v1/partners#sort",
	"GET /v1/pipelines#include_archived",
	"GET /v1/products#active",
	"GET /v1/products#include_archived",
	"GET /v1/products#sort",
	"GET /v1/projects#include_archived",
	"GET /v1/projects#owner_team_id",
	"GET /v1/projects#q",
	"GET /v1/projects#sort",
	"GET /v1/projects#unassigned",
	"GET /v1/records/{entity_type}/{entity_id}/tags#entity_id",
	"GET /v1/records/{entity_type}/{entity_id}/tags#entity_type",
	"GET /v1/search#per_type",
	"GET /v1/search#types",
	"GET /v1/search#with_employees",
	"GET /v1/tags#with_carried_by",
	"GET /v1/users#include_inactive",
	"GET /v1/users#include_invited",
	"PATCH /v1/companies/{id}/facts/{factKey}#factKey",
	"PATCH /v1/companies/{id}/profile-fields/{field}#field",
	"PATCH /v1/offers/{id}/line-items/{lineItemId}#lineItemId",
	"POST /v1/companies/{id}/facts/{factKey}/confirm#factKey",
	"POST /v1/companies/{id}/profile-fields/{field}/confirm#field",
	"POST /v1/deal-rooms/{id}/threads/{threadId}/comments#threadId",
}
