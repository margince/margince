// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// This file sorts after toolrolematrix, so the sweep runs first. The settings
// rows the filler names outlive a test, and a second maker meets a conflict.
const toolListingUser = "tool-role-listing@fable.test"

// A passport is offered a tool or action when, and only when, the gate admits
// its user to it. The roles are an operator's own. One is a copy of rep that
// may create tags and may not update deals. The other member holds rep and
// ops, whose grants merge to the widest.
func TestToolsListAgreesWithTheGateForAnOperatorsRoles(t *testing.T) {
	d := toolRoleApp(t, "tool-role-listing")
	specs := compose.NewRegistry(nil, compose.SendPath{}).Specs()
	calls := toolRoleCalls(specs, routeShapes())
	createRole(t, d.e, "rep_tagger", `jsonb_set(jsonb_set(permissions,
		'{objects,tag}', coalesce(permissions->'objects'->'tag', '{}') || '{"create": true}'),
		'{objects,deal}', coalesce(permissions->'objects'->'deal', '{}') || '{"update": false}')`)

	for _, holding := range [][]string{{"rep_tagger"}, {"rep", "ops"}} {
		t.Run(strings.Join(holding, "+"), func(t *testing.T) {
			holdRoles(t, d.e, "admin")
			clients := passportClients(t, d.e, strings.Join(holding, "+"), 1+len(calls)/toolRoleCallsPerPassport)
			controls := passportClients(t, d.e, "control "+strings.Join(holding, "+"), 1+len(calls)/toolRoleCallsPerPassport)
			pool := seedToolRoleRecords(t, d.e, calls, "")
			holdRoles(t, d.e, holding...)
			offered := offeredCalls(t, clients[0].ListedTools(t), specs)
			answers := map[string]roleAnswer{}
			for n, call := range calls {
				// One set of records for both. The role calls first, so a call it
				// was refused leaves the records as the admin control finds them.
				args := call.args(pool)
				holdRoles(t, d.e, holding...)
				got := clients[n/toolRoleCallsPerPassport].Call(t, call.verb, args)
				holdRoles(t, d.e, "admin")
				control := controls[n/toolRoleCallsPerPassport].Call(t, call.verb, args)
				// A refusal the admin meets too is about the record, not the role.
				answers[call.row] = roleAnswer{
					ran: !got.IsError, refused: refusedAGrant(got) && !isRefusal(control), text: got.Text,
				}
			}
			for _, disagreement := range listingDisagreements(calls, specs, offered, answers) {
				t.Error(disagreement)
			}
		})
	}
	holdRoles(t, d.e, "admin")
}

// A role change reaches the next tools/list of a passport already issued: the
// listing reads the grants afresh, as every call's admission does.
func TestARoleChangeReachesTheNextToolsList(t *testing.T) {
	d := newTwoDoors(t, "tool-role-change", "read", "write")
	user := "tool-role-change@fable.test"
	assignRoleTo(t, d.e, user, "rep")
	asRep := len(offeredActions(t, d.mcp.ListedTools(t)))
	assignRoleTo(t, d.e, user, "admin")
	asAdmin := len(offeredActions(t, d.mcp.ListedTools(t)))
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
// choosing by the arguments named is held as one tool.
func listingDisagreements(calls []toolRoleCall, specs []mcp.ToolSpec, offered map[string]bool, answers map[string]roleAnswer) []string {
	chooses := map[string]bool{}
	for _, spec := range specs {
		chooses[spec.Name] = len(spec.Actions) > 0 && spec.ActionField == ""
	}
	var out []string
	someAdmitted, someRan := map[string]bool{}, map[string]bool{}
	for _, call := range calls {
		got := answers[call.row]
		if chooses[call.tool] {
			someAdmitted[call.tool] = someAdmitted[call.tool] || !got.refused
			someRan[call.tool] = someRan[call.tool] || got.ran
			continue
		}
		switch {
		case offered[call.row] && got.refused:
			out = append(out, fmt.Sprintf("%s is offered and the gate refuses it: %.200s", call.row, got.text))
		case !offered[call.row] && got.ran:
			out = append(out, fmt.Sprintf("%s is hidden and it ran", call.row))
		}
	}
	for tool := range chooses {
		listed := slices.ContainsFunc(calls, func(call toolRoleCall) bool { return call.tool == tool && offered[call.row] })
		if chooses[tool] && (listed && !someAdmitted[tool] || !listed && someRan[tool]) {
			out = append(out, fmt.Sprintf("%s is offered=%v, and the gate admits some action=%v and ran some=%v",
				tool, listed, someAdmitted[tool], someRan[tool]))
		}
	}
	return out
}

func isRefusal(got apptest.MCPResult) bool {
	return got.IsError && strings.Contains(got.Text, "permission denied")
}

// namedCheckRefusal matches a refusal that names the check the listing reads. A
// bare "permission denied" is a gate on one record, such as its row scope. No
// listing can know it before the call names the record.
var namedCheckRefusal = regexp.MustCompile(`(([a-z_]+|\*)\.(create|read|update|delete)(\|(create|read|update|delete))*|scope:[a-z]+|admin-only operation): permission denied`)

func refusedAGrant(got apptest.MCPResult) bool {
	return isRefusal(got) && namedCheckRefusal.MatchString(got.Text)
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

// holdRoles gives the listing test's user these workspace roles and no others.
func holdRoles(t *testing.T, e *apptest.AppEnv, keys ...string) {
	t.Helper()
	email := toolListingUser
	if _, err := e.Owner.Exec(t.Context(), `
		DELETE FROM role_assignment WHERE team_id IS NULL AND user_id = (SELECT id FROM app_user WHERE email = $1)`,
		email); err != nil {
		t.Fatalf("clearing the roles of %s: %v", email, err)
	}
	if _, err := e.Owner.Exec(t.Context(), `
		INSERT INTO role_assignment (user_id, role_id)
		SELECT (SELECT id FROM app_user WHERE email = $1), id FROM role WHERE key = ANY($2)`, email, keys); err != nil {
		t.Fatalf("giving %s the roles %v: %v", email, keys, err)
	}
}
