// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// listingPass is one operator's holding of roles, checked against the gate.
type listingPass struct {
	name string
	run  func(t *testing.T)
}

// A passport on each seeded role is offered the cells the published matrix
// allows that role, and the matrix names every seeded role. It runs on every
// PR, so a tools/list change shows against the committed matrix at once.
func TestToolsListOffersEachSeededRoleItsAllowedCells(t *testing.T) {
	d := toolRoleApp(t, "tool-role-cells")
	var keys []string
	for _, role := range apptest.SeededRoles(t, d.E) {
		keys = append(keys, role.Key)
	}
	if doc := publishedMatrix(t); !slices.Equal(doc.Roles, keys) {
		t.Errorf("%s publishes the roles %v and the database seeds %v", toolRolesFile, doc.Roles, keys)
	}
	holdCellsAreOffered(t, d, "tool-role-cells@fable.test")
}

// operatorListings are the holdings the test above checks, one pass each.
func operatorListings(t *testing.T, d *apptest.TwoDoors, recorded *toolCheckRecorder) []listingPass {
	t.Helper()
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	calls := toolRoleCalls(specs, routeShapes())
	calls = append(calls, kindVariants(t, calls, seedableKinds(t, d.E))...)
	createRole(t, d.E, "rep_tagger", `jsonb_set(jsonb_set(permissions,
		'{objects,tag}', coalesce(permissions->'objects'->'tag', '{}') || '{"create": true}'),
		'{objects,deal}', coalesce(permissions->'objects'->'deal', '{}') || '{"update": false}')`)
	createRole(t, d.E, "rep_path_bound", `jsonb_set(jsonb_set(permissions,
		'{objects,company}', coalesce(permissions->'objects'->'company', '{}') || '{"create": false}'),
		'{objects,installation_settings}', coalesce(permissions->'objects'->'installation_settings', '{}') || '{"read": false}')`)

	adminRows := publishedAdminAnswers(t)
	holdings := [][]string{{"rep_tagger"}, {"rep_path_bound"}, {"rep", "ops"}}
	seats := make([]*apptest.AppEnv, len(holdings))
	emails := make([]string, len(holdings))
	for i := range holdings {
		emails[i] = fmt.Sprintf("tool-role-listing-%d@fable.test", i)
		seats[i] = signedInAs(t, d.E, emails[i])
	}
	passes := make([]listingPass, len(holdings))
	for i, holding := range holdings {
		passes[i] = listingPass{name: "listing " + strings.Join(holding, "+"), run: func(t *testing.T) {
			e, email, human := seats[i], emails[i], apptest.SessionUserID(t, seats[i])
			// A gate refused the call when the recorder saw it refuse and the
			// call stopped on it.
			refused := func(verb string, got apptest.MCPResult) bool {
				run, _ := recorded.take(verb, human)
				return apptest.VerdictOf(got, run.Refused) == apptest.VerdictRefused
			}
			apptest.AssignRoleTo(t, e, email, "admin")
			clients := passportClients(t, e, strings.Join(holding, "+"), 1+len(calls)/toolRoleCallsPerPassport)
			controls := passportClients(t, e, "control "+strings.Join(holding, "+"), 1+len(calls)/toolRoleCallsPerPassport)
			pool := seedToolRoleRecords(t, e, calls, "")
			apptest.AssignRoleTo(t, e, email, holding...)
			offered := offeredCalls(t, clients[0].ListedTools(t), specs)
			answers := map[string]roleAnswer{}
			for n, call := range calls {
				pool.caller = clients[n/toolRoleCallsPerPassport].Bearer()
				got, args := callHolding(t, e, email, clients[n/toolRoleCallsPerPassport], call, pool, holding)
				roleRefused := refused(call.verb, got)
				// A refusal the admin meets too is about the record, not the role.
				// The published matrix says how the admin was answered; a kind
				// variant it does not hold is asked of the admin on the same records.
				adminAdmitted, published := adminRows[call.row]
				if !published {
					apptest.AssignRoleTo(t, e, email, "admin")
					adminAdmitted = !refused(call.verb, controls[n/toolRoleCallsPerPassport].Call(t, call.verb, args))
					apptest.AssignRoleTo(t, e, email, holding...)
				}
				answers[call.row] = roleAnswer{ran: !got.IsError, refused: roleRefused && adminAdmitted, text: got.Text}
			}
			for _, disagreement := range listingDisagreements(calls, specs, offered, answers) {
				t.Error(disagreement)
			}
			holdPathDependentTools(t, clients[0], offered, answers)
		}}
	}
	return passes
}

// callHolding makes call as the roles held, and answers its arguments too. A
// call that runs alone has its state seeded as it takes it, which only the
// admin may, and runs under the installation's lock.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
func callHolding(t *testing.T, e *apptest.AppEnv, email string, client *apptest.MCPClient, call toolRoleCall, pool *recordPool, holding []string) (apptest.MCPResult, map[string]any) {
	t.Helper()
	if !runsAlone(t, call) {
		args := call.args(pool)
		return client.Call(t, call.verb, args), args
	}
	installationSeeds.Lock()
	defer installationSeeds.Unlock()
	apptest.AssignRoleTo(t, e, email, "admin")
	args := call.args(pool)
	apptest.AssignRoleTo(t, e, email, holding...)
	return client.Call(t, call.verb, args), args
}

// publishedMatrix is docs/reference/mcp-tool-roles.json as committed.
func publishedMatrix(t *testing.T) toolRolesFileShape {
	t.Helper()
	raw, err := os.ReadFile(toolRolesFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc toolRolesFileShape
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s does not read: %v", toolRolesFile, err)
	}
	return doc
}

// publishedRow is a published row's name as the sweep's calls name it.
func publishedRow(row toolRoleRow) string {
	return strings.TrimSuffix(row.Tool+"."+row.Action, ".")
}

// publishedAdminAnswers is, per published row, whether the matrix has the
// admin running it.
func publishedAdminAnswers(t *testing.T) map[string]bool {
	t.Helper()
	doc := publishedMatrix(t)
	out := make(map[string]bool, len(doc.Rows))
	for _, row := range doc.Rows {
		out[publishedRow(row)] = slices.Contains(row.Allowed, "admin")
	}
	return out
}

// holdCellsAreOffered holds that a passport on each seeded role is offered the
// cells the published matrix allows that role. No refused call is offered, and
// no allowed one is hidden. It runs on the sweep's app once the sweep is done.
func holdCellsAreOffered(t *testing.T, d *apptest.TwoDoors, email string) {
	t.Helper()
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	doc := publishedMatrix(t)
	for _, role := range doc.Roles {
		apptest.AssignRoleTo(t, d.E, email, role)
		listed := d.MCP.ListedTools(t)
		offered := offeredCalls(t, listed, specs)
		for _, disagreement := range cellDisagreements(role, doc.Rows, specs, offered) {
			t.Errorf("%s: %s", role, disagreement)
		}
		for _, named := range hiddenActionsNamed(t, listed, specs) {
			t.Errorf("%s: %s", role, named)
		}
	}
	apptest.AssignRoleTo(t, d.E, email, "admin")
}

// cellDisagreements names each published row the listing offers role while
// the matrix refuses it, or hides while the matrix allows it. A router that
// picks its action by the arguments named has no enum to narrow. It is held as
// one tool, allowed where any of its rows is.
func cellDisagreements(role string, rows []toolRoleRow, specs []mcp.ToolSpec, offered map[string]bool) []string {
	byArguments := map[string]bool{}
	for _, spec := range specs {
		byArguments[spec.Name] = len(spec.Actions) > 0 && spec.ActionField == ""
	}
	allowed, listed := map[string]bool{}, map[string]bool{}
	for _, row := range rows {
		name := publishedRow(row)
		if byArguments[row.Tool] {
			name = row.Tool
		}
		allowed[name] = allowed[name] || slices.Contains(row.Allowed, role)
		listed[name] = listed[name] || offered[publishedRow(row)]
	}
	var out []string
	for _, name := range slices.Sorted(maps.Keys(allowed)) {
		switch {
		case listed[name] && !allowed[name]:
			out = append(out, name+" is offered and the matrix refuses it")
		case !listed[name] && allowed[name]:
			out = append(out, name+" is hidden and the matrix allows it")
		}
	}
	return out
}

// holdPathDependentTools holds what every role here may do, whatever one path
// of a tool asks. The record tools stay offered, a contact is made, and whoami
// answers.
func holdPathDependentTools(t *testing.T, client *apptest.MCPClient, offered map[string]bool, answers map[string]roleAnswer) {
	t.Helper()
	for _, row := range []string{"create_record", "update_record", "read_record", "whoami"} {
		if !offered[row] {
			t.Errorf("%s is hidden from a role that may use it on some kind of record", row)
		}
	}
	contact := client.Call(t, "create_record", map[string]any{
		"record_type": "contact", "fields": map[string]any{"full_name": "Path Bound", "source": "manual"},
	})
	if contact.IsError {
		t.Errorf("creating a contact was refused: %.300s", contact.Text)
	}
	if who := answers["whoami"]; !who.ran {
		t.Errorf("whoami was refused: %.300s", who.text)
	}
}

// A role change reaches the next tools/list of a passport already issued: the
// listing reads the grants afresh, as every call's admission does.
func TestARoleChangeReachesTheNextToolsList(t *testing.T) {
	d := apptest.SetupTwoDoors(t, "tool-role-change", nil, "read", "write")
	user := "tool-role-change@fable.test"
	apptest.AssignRoleTo(t, d.E, user, "rep")
	asRep := len(offeredActions(t, d.MCP.ListedTools(t)))
	apptest.AssignRoleTo(t, d.E, user, "admin")
	asAdmin := len(offeredActions(t, d.MCP.ListedTools(t)))
	if asAdmin <= asRep {
		t.Errorf("the same passport is offered %d tools and actions as an admin and %d as a rep; the role change did not reach the listing", asAdmin, asRep)
	}
}

// offeredActions names every tool and action word one listing offers.
func offeredActions(t *testing.T, listed []apptest.ListedTool) []string {
	t.Helper()
	var out []string
	for _, tool := range listed {
		out = append(out, tool.Name)
		for _, word := range routeWords(t, tool) {
			out = append(out, tool.Name+"."+word)
		}
	}
	return out
}

// routeWords lists the enum words a listed tool's top-level arguments admit.
func routeWords(t *testing.T, tool apptest.ListedTool) []string {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum []string `json:"enum"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatalf("%s's input schema does not read: %v", tool.Name, err)
	}
	var words []string
	for name, arg := range schema.Properties {
		for _, word := range arg.Enum {
			words = append(words, name+"="+word)
		}
	}
	return words
}

// offeredCalls is, per swept row, whether the listing offers it: a plain tool by
// name, a router's action by its route field's enum. A router that picks its
// target by the arguments named offers each action while it is listed at all.
func offeredCalls(t *testing.T, listed []apptest.ListedTool, specs []mcp.ToolSpec) map[string]bool {
	t.Helper()
	byName := map[string]apptest.ListedTool{}
	for _, tool := range listed {
		byName[tool.Name] = tool
	}
	offered := map[string]bool{}
	for _, spec := range specs {
		tool, listed := byName[spec.Name]
		if spec.Unlisted {
			continue
		}
		if len(spec.Actions) == 0 {
			offered[spec.Name] = listed
			continue
		}
		var words []string
		if listed {
			words = routeWords(t, tool)
		}
		for _, action := range spec.Actions {
			offered[spec.Name+"."+action.Action] = listed &&
				(spec.ActionField == "" || slices.Contains(words, spec.ActionField+"="+action.Action))
		}
	}
	return offered
}

// roleAnswer is what one call answered the role: it ran, or the gate refused
// the role where it admitted the admin, or neither.
type roleAnswer struct {
	ran, refused bool
	text         string
}

// listingDisagreements names each row the listing and the gate answer apart:
// an offered call the gate refuses the role, or a hidden one that ran. A router
// choosing by the arguments named is held as one tool. A call and its kind
// variants are held as one row. Offered, some path is admitted; hidden, none ran.
func listingDisagreements(calls []toolRoleCall, specs []mcp.ToolSpec, offered map[string]bool, answers map[string]roleAnswer) []string {
	chooses := map[string]bool{}
	for _, spec := range specs {
		chooses[spec.Name] = len(spec.Actions) > 0 && spec.ActionField == ""
	}
	groupOf := func(call toolRoleCall) string {
		if chooses[call.tool] {
			return call.tool
		}
		return baseRow(call.row)
	}
	listed, members := map[string]bool{}, map[string]int{}
	someAdmitted, someRan := map[string]bool{}, map[string]bool{}
	for _, call := range calls {
		group, got := groupOf(call), answers[call.row]
		listed[group] = listed[group] || offered[baseRow(call.row)]
		members[group]++
		someAdmitted[group] = someAdmitted[group] || !got.refused
		someRan[group] = someRan[group] || got.ran
	}
	var out []string
	for _, group := range slices.Sorted(maps.Keys(members)) {
		switch {
		case listed[group] && !someAdmitted[group]:
			out = append(out, fmt.Sprintf("%s is offered and the gate refuses every path of it: %.200s", group, answers[group].text))
		case !listed[group] && someRan[group]:
			out = append(out, fmt.Sprintf("%s is hidden and some path of it ran", group))
		}
	}
	return out
}

// createRole adds an operator's role whose grants are rep's as changed by edit,
// an SQL expression over rep's permissions document.
func createRole(t *testing.T, e *apptest.AppEnv, key, edit string) {
	t.Helper()
	if _, err := e.Owner.Exec(t.Context(),
		`INSERT INTO role (key, name, permissions) SELECT $1, $1, `+edit+` FROM role WHERE key = 'rep' AND is_system`,
		key); err != nil {
		t.Fatalf("creating role %s: %v", key, err)
	}
	// The database outlives this test, and a later sweep reads every role.
	t.Cleanup(func() {
		if _, err := e.Owner.Exec(context.Background(), `DELETE FROM role WHERE key = $1`, key); err != nil {
			t.Errorf("removing role %s: %v", key, err)
		}
	})
}

// hiddenActionsNamed names each router whose listed schema speaks of an action
// its route enum hides, as an argument's name, description or enum word. In
// prose only an action's own spelling with an underscore counts, as a plain
// word such as "member" is English first. The route field's own description is
// the router's prose about every action, and the tool's description answers it.
func hiddenActionsNamed(t *testing.T, listed []apptest.ListedTool, specs []mcp.ToolSpec) []string {
	t.Helper()
	byName := map[string]mcp.ToolSpec{}
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	var out []string
	for _, tool := range listed {
		spec := byName[tool.Name]
		if spec.ActionField == "" {
			continue
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s's input schema does not read: %v", tool.Name, err)
		}
		var route struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(schema.Properties[spec.ActionField], &route); err != nil {
			t.Fatalf("%s's route field does not read: %v", tool.Name, err)
		}
		hidden := map[string]bool{}
		for _, action := range spec.Actions {
			hidden[action.Action] = !slices.Contains(route.Enum, action.Action)
		}
		delete(schema.Properties, spec.ActionField)
		for name, arg := range schema.Properties {
			for _, word := range append(schemaWords(t, arg), name) {
				if hidden[word] && (strings.Contains(word, "_") || !proseOnly(t, arg, word)) {
					out = append(out, fmt.Sprintf("%s's argument %s names the hidden action %s", tool.Name, name, word))
				}
			}
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// schemaWords lists the words of the strings in one argument's schema, at any
// depth: its descriptions, enums and the names of its members.
//
//craft:ignore naked-any a schema member is whichever JSON value its keyword takes
func schemaWords(t *testing.T, arg json.RawMessage) []string {
	t.Helper()
	var doc any
	if err := json.Unmarshal(arg, &doc); err != nil {
		t.Fatalf("an argument does not read: %v", err)
	}
	var words []string
	var walk func(any)
	walk = func(node any) {
		switch value := node.(type) {
		case string:
			words = append(words, actionWord.FindAllString(value, -1)...)
		case []any:
			for _, item := range value {
				walk(item)
			}
		case map[string]any:
			for key, item := range value {
				words = append(words, key)
				walk(item)
			}
		}
	}
	walk(doc)
	return words
}

// proseOnly reports whether word appears in arg only inside a description.
//
//craft:ignore naked-any an enum word is whichever JSON value the schema lists
func proseOnly(t *testing.T, arg json.RawMessage, word string) bool {
	t.Helper()
	var shape struct {
		Enum  []any                      `json:"enum"`
		Items struct{ Enum []any }       `json:"items"`
		Props map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(arg, &shape); err != nil {
		t.Fatalf("an argument does not read: %v", err)
	}
	named := slices.Concat(shape.Enum, shape.Items.Enum)
	_, isMember := shape.Props[word]
	return !isMember && !slices.Contains(named, any(word))
}

// actionWord matches a word an action can be named by.
var actionWord = regexp.MustCompile(`[a-z][a-z_]*`)
