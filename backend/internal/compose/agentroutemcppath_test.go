// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// A passport has the same functionality on both doors. So each path and query
// parameter of an agent route is an argument of its x-mcp-tool verb, top level
// or under filters. Paging is the one exception: MCP keeps smaller pages, and
// a cursor reaches the same records.
func TestEveryAgentRouteParameterIsAnArgumentOfItsVerb(t *testing.T) {
	registry := NewRegistry(nil, SendPath{})
	arguments := map[string]map[string]bool{}
	var listSchema json.RawMessage
	for _, spec := range registry.Specs() {
		arguments[spec.Name] = argumentNames(t, spec.Name, spec.InputSchema)
		if spec.Name == "list_records" {
			listSchema = spec.InputSchema
		}
	}
	params := routeParameters(t)
	stores := NewProvider(nil)
	var gaps []string
	checked := 0
	respelled := map[string]bool{}
	for route, policy := range agentPolicies {
		args, registered := arguments[policy.Tool]
		if policy.Access != accessTool || !registered {
			continue // TestEveryDeclaredToolVerbIsRegistered owns an unregistered verb.
		}
		if policy.Tool == "list_records" && policy.RecordType != "" {
			args = listArguments(t, listSchema, stores.ListFilters(datasource.EntityType(policy.RecordType)))
		}
		if policy.Tool == "create_record" && policy.RecordType != "" {
			args = maps.Clone(args)
			for _, parent := range agents.CreateParents(string(policy.RecordType)) {
				args[parent] = true
			}
		}
		names, known := params[route]
		if !known {
			t.Fatalf("the contract walk did not reach %s, which the policy table carries; the census is short", route)
		}
		for _, p := range names {
			checked++
			candidates := argumentsFor(p, args)
			if spelled, ok := spelledOtherwiseByItsVerb[route+"#"+p.name]; ok {
				candidates = append(candidates, spelled)
				respelled[route+"#"+p.name] = args[spelled]
			}
			if !slices.ContainsFunc(candidates, func(arg string) bool { return args[arg] }) {
				gaps = append(gaps, route+"#"+p.name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no agent route declares a parameter — this sweep checked nothing")
	}
	sort.Strings(gaps)
	for _, gap := range gaps {
		if answeredByAnotherVerb.Waived(t, gap) || slices.Contains(parameterGapsPendingParity, gap) {
			continue
		}
		route, name, _ := strings.Cut(gap, "#")
		t.Errorf("%s takes %q, and its verb %s has no argument for it, so a passport can ask REST what "+
			"MCP cannot. Add the argument to the verb (top level or under filters), or point the route at "+
			"a verb that takes it.", route, name, agentPolicies[route].Tool)
	}
	answeredByAnotherVerb.AssertAllMatched(t)
	for param, spelled := range spelledOtherwiseByItsVerb {
		if !respelled[param] {
			t.Errorf("%s is spelled %q by its verb, which takes no such argument; correct or remove the entry", param, spelled)
		}
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

// listArguments narrows list_records to what one record type is listed by. Its
// filters object publishes every type's filters at once. Checked against all of
// them, one type's filter would hide another type's gap.
func listArguments(t *testing.T, schema json.RawMessage, filters []string) map[string]bool {
	t.Helper()
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("list_records publishes an input schema that is not JSON: %v", err)
	}
	narrowed := map[string]bool{}
	for arg := range parsed.Properties {
		narrowed[arg] = true
	}
	for _, filter := range filters {
		narrowed[filter] = true
	}
	return narrowed
}

// pagingParameters are the ones a verb may answer with its own page size.
var pagingParameters = []string{"cursor", "limit"}

// argumentsFor names the arguments a route parameter may be asked through. A
// query parameter keeps its name. A route's own {id} is the record it acts on,
// and every verb names that record through an id argument (list_id). A path's
// {entity_type}/{entity_id} is a record the tools call record_type/record_id.
// Any other nested path parameter ({lineItemId}) must be an argument of its own name.
func argumentsFor(p routeParameter, args map[string]bool) []string {
	if p.in != "path" {
		return []string{p.name}
	}
	if spelled, ok := recordPathParameters[p.name]; ok {
		return []string{p.name, spelled}
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

// recordPathParameters are the polymorphic record a path names, by the
// argument every tool takes it as.
//
// gatekit:fixture the argument names the tools give a route's record path parameters
var recordPathParameters = map[string]string{"entity_type": "record_type", "entity_id": "record_id"}

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

// argumentNames reads the arguments a verb's input schema takes, at any depth.
// A route's flat query parameter may be a member of an object argument.
func argumentNames(t *testing.T, tool string, schema json.RawMessage) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	if err := collectArgumentNames(schema, names); err != nil {
		t.Fatalf("tool %q publishes an input schema that is not JSON: %v", tool, err)
	}
	return names
}

func collectArgumentNames(schema json.RawMessage, names map[string]bool) error {
	var parsed struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Items      json.RawMessage            `json:"items"`
	}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return err
	}
	for name, raw := range parsed.Properties {
		names[name] = true
		if err := collectArgumentNames(raw, names); err != nil {
			return err
		}
	}
	if len(parsed.Items) > 0 {
		return collectArgumentNames(parsed.Items, names)
	}
	return nil
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

// spelledOtherwiseByItsVerb are route parameters the verb takes under its own
// name, as a member of an object argument or as the record's id.
//
// gatekit:fixture the argument each listed route parameter is spelled as by its verb
var spelledOtherwiseByItsVerb = map[string]string{
	"GET /v1/analytics/evaluate#scope_kind":      "kind",
	"GET /v1/analytics/evaluate#scope_id":        "id",
	"GET /v1/analytics/evidence#scope_kind":      "kind",
	"GET /v1/analytics/evidence#scope_id":        "id",
	"GET /v1/analytics/editions/compare#left_id": "id",
	"GET /v1/search#types":                       "record_types",
}

// answeredByAnotherVerb are route parameters whose question an agent asks
// through some other verb, so the route's own verb need not take them.
var answeredByAnotherVerb = gatekit.Waive(map[string]string{
	"GET /v1/companies#id": "a screen's batch read of companies it holds ids for; an agent reads each by id " +
		"through read_record (gates/seamfilters_test.go rules it out of list_records)",
})

// parameterGapsPendingParity are the route parameters no verb takes yet, as
// "METHOD /v1/route#parameter". The list only shrinks: an entry that stops
// being a gap fails the test above.
var parameterGapsPendingParity = []string{}
