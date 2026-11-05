// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package toolrole holds the MCP role sweep and the tests that hold tools/list
// to its answers. The sweep calls every tool and action as every seeded role,
// which takes minutes. It is a test binary of its own, so that time counts
// against its own per-package timeout in the integration lane.
package toolrole

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

const (
	toolRolesFile = "../../../../../docs/reference/mcp-tool-roles.json"
	toolRoleUser  = "tool-role-matrix@fable.test"
	// toolRoleCallsPerPassport stays under a passport's per-window call budget.
	toolRoleCallsPerPassport = 80
)

// TestTheToolRoleMatrixIsPublished measures, for every seeded role, which tools
// a passport on that role is refused, and holds docs/reference/mcp-tool-roles.json
// to the answer. The roles come from the database and the tools from the registry,
// so neither is a second list. A cell holds what the gate says, not what the seed
// document claims.
func TestTheToolRoleMatrixIsPublished(t *testing.T) {
	recorded := newToolCheckRecorder()
	d := toolRoleApp(t, "tool-role-matrix", compose.WithToolChecksRecordedForTest(recorded.note))
	calls := toolRoleCalls(compose.NewRegistry(nil, compose.SendPath{}).Specs(), routeShapes())
	if len(calls) == 0 {
		t.Fatal("the registry serves no tool, so this measured nothing")
	}
	holdEveryFixtureNamesACall(t, calls)
	roles := apptest.SeededRoles(t, d.E)
	if len(roles) == 0 {
		t.Fatal("the database seeds no role, so this measured nothing")
	}

	verdicts := map[string]toolRoleVerdicts{}
	measured := newToolChecksMeasure()
	elsewhere := measuredElsewhere(t, roles)
	for verb, row := range elsewhere {
		measured.control[verb] = agents.ToolChecksRow{Checks: row.Checks}
	}
	for _, call := range calls {
		if row, ok := elsewhere[call.verb]; ok {
			verdicts[call.row] = row.toolRoleVerdicts
		}
	}
	seats := roleSeats(t, d.E, roles)
	listings := operatorListings(t, d, recorded)
	swept := make([][]sweptCall, len(seats))
	eachSeat(t, "roles", seats, func(t *testing.T, i int, seat roleSeat) {
		swept[i] = sweepSeat(t, seat, recorded, calls, elsewhere)
	}, listings...)
	if t.Failed() {
		return
	}
	// Merged in the roles' order, so the files read the same however the
	// passes interleaved.
	for i, seat := range seats {
		for _, c := range swept[i] {
			if c.ran {
				measured.add(c.call.verb, seat.role.Key == "admin", c.run, c.got)
			}
			row := verdicts[c.call.row]
			if len(c.releaseRefused) > 0 {
				row.Refused = append(row.Refused, seat.role.Key)
			} else {
				row.record(seat.role.Key, c.got, c.run.Refused)
			}
			verdicts[c.call.row] = row
		}
	}
	measureEveryKind(t, seats, recorded, measured, kindVariants(t, withoutVerbs(calls, elsewhere), seedableKinds(t, d.E)), verdicts)
	colleague := crossOwnerPass(t, d.E, seats, calls)
	if t.Failed() {
		return
	}

	holdTheMeasurementHasTeeth(t, roles, len(calls), verdicts)
	doc, open := toolRolesDocument(roles, calls, verdicts, colleague)
	if len(open) > 0 {
		t.Errorf("%d cells reached no verdict. Each must run, or be refused by a gate:\n%s", len(open), strings.Join(open, "\n"))
	}
	rows, partial := measured.rows()
	holdTheToolChecksTable(t, provenBy(rows, roles, calls, verdicts), partial)
	rendered, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rendered = append(rendered, '\n')
	if os.Getenv("UPDATE_MCP_TOOL_ROLES") != "" {
		if err := os.WriteFile(toolRolesFile, rendered, 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	committed, err := os.ReadFile(toolRolesFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(committed) != string(rendered) {
		t.Errorf("%s is not what the roles are refused. Run: UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/toolrole RUN=TestTheToolRoleMatrixIsPublished, then go test ./gates -run TestTheMCPParityCatalogIsPublished -update-mcp-parity", toolRolesFile)
	}
	// The listing reads the checks this binary embeds, which the sweep above
	// held to what it measured.
	t.Run("cells", func(t *testing.T) { holdCellsAreOffered(t, d, toolRoleUser) })
}

// extensionRoleFile holds the verbs that need a composed extension unit. They
// are measured in a test binary of their own. A registered unit holds for the
// whole binary, and this one runs every other suite in the package.
const extensionRoleFile = "../extensionrole/measured.json"

// elsewhereRow is one verb's cells and checks as measured.json holds them.
type elsewhereRow struct {
	toolRoleVerdicts
	Checks []string `json:"checks"`
}

// measuredElsewhere reads the verbs another binary measured. Its roles must be
// this sweep's, or its cells would answer for roles nobody seeded.
func measuredElsewhere(t *testing.T, roles []apptest.SeededRole) map[string]elsewhereRow {
	t.Helper()
	raw, err := os.ReadFile(extensionRoleFile)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Roles []string                `json:"roles"`
		Verbs map[string]elsewhereRow `json:"verbs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s does not read: %v", extensionRoleFile, err)
	}
	keys := make([]string, len(roles))
	for i, role := range roles {
		keys[i] = role.Key
	}
	if !slices.Equal(doc.Roles, keys) {
		t.Fatalf("%s measured the roles %v and this database seeds %v. Run: UPDATE_MCP_TOOL_ROLES=1 make test-it DIR=backend/internal/compose/integration/extensionrole", extensionRoleFile, doc.Roles, keys)
	}
	return doc.Verbs
}

// withoutVerbs is calls less those of the verbs another binary measured.
func withoutVerbs(calls []toolRoleCall, elsewhere map[string]elsewhereRow) []toolRoleCall {
	return slices.DeleteFunc(slices.Clone(calls), func(call toolRoleCall) bool {
		_, measured := elsewhere[call.verb]
		return measured
	})
}

// holdTheMeasurementHasTeeth fails a sweep whose calls never reach a gate: such
// a sweep would publish no role as refused anything.
func holdTheMeasurementHasTeeth(t *testing.T, roles []apptest.SeededRole, calls int, verdicts map[string]toolRoleVerdicts) {
	t.Helper()
	perRole := map[string]int{}
	for _, row := range verdicts {
		for _, key := range row.Refused {
			perRole[key]++
		}
	}
	for _, role := range roles {
		if role.Key != "admin" && perRole[role.Key] == 0 {
			t.Errorf("role %s is refused none of %d calls, so the sweep did not reach the gates", role.Key, calls)
		}
	}
}

// holdEveryFixtureNamesACall fails a fixture whose verb the sweep no longer
// calls, which would otherwise stay in the table and fill nothing.
func holdEveryFixtureNamesACall(t *testing.T, calls []toolRoleCall) {
	t.Helper()
	inTable := map[string]int{}
	for _, fixtures := range fixtureTables() {
		for verb := range fixtures {
			inTable[verb]++
			if !slices.ContainsFunc(calls, func(call toolRoleCall) bool { return call.verb == verb }) {
				t.Errorf("the fixtures have %s, which no swept call names; remove it or rename it to the verb it was for", verb)
			}
		}
	}
	for verb, n := range inTable {
		if n > 1 {
			t.Errorf("%s has a fixture in %d tables, so all but one fill nothing", verb, n)
		}
	}
}

// firstLine is the start of an answer, enough to name why a call stopped.
func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	if len(line) > 240 {
		line = line[:240]
	}
	return line
}

func isBudgetRefusal(text string) bool {
	return strings.HasPrefix(text, "Too many") || strings.Contains(text, "call ceiling") || strings.Contains(text, "ceiling")
}

func passportClients(t *testing.T, e *apptest.AppEnv, role string, n int) []*apptest.MCPClient {
	t.Helper()
	clients := make([]*apptest.MCPClient, n)
	for i := range clients {
		bearer, _ := apptest.PassportWithID(t, e, fmt.Sprintf("tool-role %s %d", role, i), "read", "write", "send", "draft", "enrich")
		clients[i] = apptest.NewMCPClient(e, strings.TrimPrefix(bearer["Authorization"], "Bearer "))
	}
	return clients
}

// toolRoleRoute is the part of a route verb's route a call fills in.
type toolRoleRoute struct {
	Body    json.RawMessage
	Pattern string
}

// toolRoleCall is one call the sweep makes: a verb, and the published row it is.
// A kind variant names kindArg's further kind, and whether the sweep seeds one.
type toolRoleCall struct {
	row, tool, action, verb string
	input                   json.RawMessage
	route                   toolRoleRoute
	kindArg, kind           string
	kindSeeded              bool
}

// routeShapes is each route verb's body schema and path by verb.
func routeShapes() map[string]toolRoleRoute {
	shapes := map[string]toolRoleRoute{}
	for _, verb := range compose.RouteVerbs() {
		shapes[verb.Verb] = toolRoleRoute{Body: verb.BodySchema, Pattern: verb.Pattern}
	}
	return shapes
}

// toolRoleCalls is one call per listed tool, or per action of a listed router.
func toolRoleCalls(specs []mcp.ToolSpec, shapes map[string]toolRoleRoute) []toolRoleCall {
	byName := map[string]mcp.ToolSpec{}
	for _, spec := range specs {
		byName[spec.Name] = spec
	}
	var calls []toolRoleCall
	for _, spec := range specs {
		if spec.Unlisted {
			continue
		}
		if len(spec.Actions) == 0 {
			calls = append(calls, toolRoleCall{row: spec.Name, tool: spec.Name, verb: spec.Name, input: spec.InputSchema, route: shapes[spec.Name]})
			continue
		}
		for _, action := range spec.Actions {
			calls = append(calls, toolRoleCall{
				row: spec.Name + "." + action.Action, tool: spec.Name, action: action.Action, verb: action.Verb,
				input: byName[action.Verb].InputSchema, route: shapes[action.Verb],
			})
		}
	}
	slices.SortFunc(calls, func(a, b toolRoleCall) int { return strings.Compare(a.row, b.row) })
	return calls
}

// toolRoleVerdicts sorts the roles by what one call answered them. open is
// each role the call stopped for before a gate decided it, and why; the sweep
// fails on any.
type toolRoleVerdicts struct {
	Allowed []string `json:"allowed_for"`
	Refused []string `json:"refused_for"`
	open    map[string]string
}

// unrecordedRefusal is a 403 no gate in platform/auth recorded: a store's own
// refusal, which no listing can tell before the call.
const unrecordedRefusal = "a 403 no gate recorded; make it an auth decision"

// record sorts one answer as apptest.VerdictOf does, and names why an open
// one stopped.
func (v *toolRoleVerdicts) record(role string, got apptest.MCPResult, refusedBy []string) {
	switch apptest.VerdictOf(got, refusedBy) {
	case apptest.VerdictAllowed:
		v.Allowed = append(v.Allowed, role)
	case apptest.VerdictRefused:
		v.Refused = append(v.Refused, role)
	case apptest.VerdictOpen:
		if v.open == nil {
			v.open = map[string]string{}
		}
		reason := stopReason(got.Text)
		if reason == stopRefused {
			reason = unrecordedRefusal
		}
		v.open[role] = reason + " (" + firstLine(got.Text) + ")"
	}
}

// Why a call stopped before a gate decided it, read from the answer the agent
// is given. Each reason is a kind of work left, such as a value the filler
// cannot make or a record the sweep does not seed. The colleague pass reports
// its answers in the same words.
const (
	stopValidation  = "validation"
	stopNotFound    = "not_found"
	stopConflict    = "conflict"
	stopUnsupported = "unsupported"
	stopInternal    = "internal"
	// stopRefused is a refusal the admin meets too: about the call's record,
	// such as a relation the sweep's user lacks, and never about a role.
	stopRefused = "refused"
	stopOther   = "other"
)

// stopReasonMeanings is what each reason says, which the page prints beside
// the colleague pass.
//
// gatekit:fixture the published meaning of each stop reason: expected data, not a waived cost
var stopReasonMeanings = map[string]string{
	stopValidation:  "An argument was refused: a value the filler cannot make.",
	stopNotFound:    "No such record: a kind the sweep does not seed, or a record it does not relate.",
	stopConflict:    "The record is not in the state the call needs.",
	stopUnsupported: "The installation does not offer the service the call needs.",
	stopInternal:    "The server failed, or answered 503 for a service the test stack does not start.",
	stopRefused:     "The admin was refused too, so the refusal is about the record, not the role.",
	stopOther:       "Any other answer.",
}

func stopReason(text string) string {
	switch {
	case apptest.AnswerNotFound(text):
		return stopNotFound
	case apptest.AnswerRefused(text):
		return stopRefused
	case strings.Contains(text, "does not offer what this call needs"), strings.Contains(text, "cannot serve this tool"):
		return stopUnsupported
	case strings.Contains(text, "failed for an internal reason"):
		return stopInternal
	case strings.Contains(text, "(conflict"):
		return stopConflict
	case strings.Contains(text, "arguments were rejected"), strings.Contains(text, "refused as issued"):
		return stopValidation
	}
	return stopOther
}

type toolRoleRow struct {
	Tool   string `json:"tool"`
	Action string `json:"action,omitempty"`
	Verb   string `json:"verb"`
	toolRoleVerdicts
	// ColleagueRecord is what each scoped role was answered on a record a
	// colleague owns, for a call that takes one.
	ColleagueRecord map[string]string `json:"colleague_record,omitempty"`
}

type toolRolesFileShape struct {
	Note    string            `json:"note"`
	Reasons map[string]string `json:"reasons"`
	Roles   []string          `json:"roles"`
	Rows    []toolRoleRow     `json:"rows"`
}

// toolRolesNote is what the JSON says the sweep does, and the page prints it.
const toolRolesNote = "Generated by TestTheToolRoleMatrixIsPublished (backend/internal/compose/integration/toolrole). " +
	"A passport on each seeded role calls every tool and action once. Each record a call names is one the sweep " +
	"made through the app's own routes, owned by the passport's own user, so the role's grants alone decide. A " +
	"refusal counts only where a gate was recorded refusing that role in that call, whether it answered 403 or hid " +
	"a row as not found. Every other answer fails the test, so every cell is one or the other. A tool that picks " +
	"its record by a kind argument is allowed where the call on some kind ran. Each role whose row scope is " +
	"narrower than all then calls every tool that takes a company, contact, deal, lead or project on one a " +
	"colleague owns, and reasons names its answers. Do not edit by hand."

// toolRolesDocument is the published file, and each cell no gate decided.
func toolRolesDocument(roles []apptest.SeededRole, calls []toolRoleCall, verdicts map[string]toolRoleVerdicts, colleague map[string]map[string]string) (toolRolesFileShape, []string) {
	doc := toolRolesFileShape{Note: toolRolesNote, Reasons: stopReasonMeanings}
	for _, role := range roles {
		doc.Roles = append(doc.Roles, role.Key)
	}
	var open []string
	for _, call := range calls {
		row := verdicts[call.row]
		for _, role := range slices.Sorted(maps.Keys(row.open)) {
			open = append(open, fmt.Sprintf("%s as %s: %s", call.row, role, row.open[role]))
		}
		published := toolRoleRow{Tool: call.tool, Action: call.action, Verb: call.verb, ColleagueRecord: colleague[call.row]}
		published.Allowed, published.Refused = sortedRoles(row.Allowed), sortedRoles(row.Refused)
		doc.Rows = append(doc.Rows, published)
	}
	return doc, open
}

func sortedRoles(keys []string) []string {
	sorted := append([]string{}, keys...)
	slices.Sort(sorted)
	return sorted
}
