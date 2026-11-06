// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// docs/reference/mcp-parity.{json,md} match the contract, the web app and the handlers' grants, and give each operation one place.
//
// The pages answer how much of the web app a user's agent can do through MCP. They
// sort that into the groups a later product step can ask a user to enable. A gate
// that read fewer operations would report a wrong percentage. So each walk has a
// floor, and a second reading of the raw contract checks the operation count.

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/types"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var updateMCPParity = flag.Bool("update-mcp-parity", false,
	"rewrite docs/reference/mcp-parity.{json,md} and the work-area table from the contract, the web app and the seeded grants")

const (
	mcpParityCommand = "go test ./gates -run TestTheMCPParityCatalogIsPublished -update-mcp-parity"
	mcpParityJSON    = "../docs/reference/mcp-parity.json"
	mcpParityPage    = "../docs/reference/mcp-parity.md"
	toolRolesJSON    = "../docs/reference/mcp-tool-roles.json"
	// repRole is the role a plain user starts with, so a setting it passes is one
	// a non-admin reaches.
	repRole = "rep"
	// unitRoutePrefix marks a route a unit adds. It is outside crm.yaml, so a
	// call to one is counted apart and never fails the contract check.
	unitRoutePrefix = "/ext/"

	// Each floor sits under today's count. It fails a walk that stopped finding its
	// subject, so a broken scan cannot pass as a small tree.
	parityOperationFloor  = 900
	paritySignedInFloor   = 850
	parityWebFileFloor    = 1000
	parityWebCallFloor    = 800
	paritySettingsFloor   = 200
	parityRoleObjectFloor = 50
)

// The states an operation can be in, for the agent of a user who reaches it.
const (
	stateOpen      = "open"
	statePending   = "pending"
	stateSecurity  = "security"
	stateCovered   = "covered"
	stateSignIn    = "sign_in"
	stateNoSession = "no_signed_in_user"
)

// How a settings operation's answer for the rep role was reached.
const (
	decidedMeasured = "measured"
	decidedChecks   = "grant_checks"
	decidedNoCheck  = "no_check"
	decidedDeclared = "declared"

	audienceMember = "member"
	audienceAdmin  = "admin"
)

type parityOperation struct {
	Method      string   `json:"method"`
	Path        string   `json:"path"`
	OperationID string   `json:"operation_id"`
	Tags        []string `json:"tags"`
	Group       string   `json:"group"`
	Area        string   `json:"area"`
	State       string   `json:"state"`
	AskFirst    bool     `json:"ask_user_first,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Verb        string   `json:"verb,omitempty"`
	Router      string   `json:"router,omitempty"`
	Action      string   `json:"action,omitempty"`
	// WebApp says how the scan knows the web app calls this: direct, inferred or
	// url. Empty when no call was found.
	WebApp string `json:"web_app,omitempty"`
	// The last three appear on a settings operation, where they decide the group.
	// RefusedBy is the first grant a refused rep lacks. The whole list of checks is
	// left out, because any store a handler reaches can add to it.
	RepPasses *bool  `json:"rep_passes,omitempty"`
	DecidedBy string `json:"decided_by,omitempty"`
	RefusedBy string `json:"refused_by,omitempty"`
}

type parityCounts struct {
	Operations  int     `json:"operations"`
	Open        int     `json:"open"`
	AskFirst    int     `json:"ask_first"`
	Pending     int     `json:"pending"`
	Security    int     `json:"security"`
	Covered     int     `json:"covered"`
	SignIn      int     `json:"sign_in"`
	NoSession   int     `json:"no_signed_in_user"`
	WebCalls    int     `json:"web_app_calls"`
	WebOpen     int     `json:"web_app_calls_open"`
	WebAskFirst int     `json:"web_app_calls_ask_first"`
	WebPending  int     `json:"web_app_calls_pending"`
	WebSecurity int     `json:"web_app_calls_security"`
	WebCovered  int     `json:"web_app_calls_covered"`
	Parity      float64 `json:"parity_signed_in_percent"`
	WebParity   float64 `json:"parity_web_app_percent"`
}

func (c *parityCounts) add(op parityOperation) {
	c.Operations++
	web := 0
	if op.WebApp != "" {
		web = 1
	}
	c.WebCalls += web
	switch op.State {
	case stateOpen:
		c.Open++
		c.WebOpen += web
		if op.AskFirst {
			c.AskFirst++
			c.WebAskFirst += web
		}
	case statePending:
		c.Pending++
		c.WebPending += web
	case stateSecurity:
		c.Security++
		c.WebSecurity += web
	case stateCovered:
		c.Covered++
		c.WebCovered += web
	case stateSignIn:
		c.SignIn++
	case stateNoSession:
		c.NoSession++
	}
	c.Parity = percentOf(c.Open, c.Operations-c.NoSession)
	c.WebParity = percentOf(c.WebOpen, c.WebCalls)
}

// percentOf truncates to one decimal, so a figure just under 100 never reads as 100.
func percentOf(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part*1000/whole) / 10
}

type parityAreaRow struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	parityCounts
}

type parityGroupRow struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	What  string `json:"what"`
	parityCounts
	Areas []parityAreaRow `json:"areas"`
}

// parityHeadline holds the denominators, and the parity each one gives.
type parityHeadline struct {
	AllOperations int `json:"all_operations"`
	// SignedIn counts what a signed-in user reaches, whether a screen calls it or
	// not. Its web counts are the page's lead figure.
	SignedIn               parityCounts `json:"signed_in_user_reaches"`
	OpenWithNoWebCallFound int          `json:"open_with_no_web_app_call_found"`
	WebParityWithCovered   float64      `json:"parity_web_app_counting_covered_percent"`
	WebParityOncePending   float64      `json:"parity_web_app_once_pending_is_open_percent"`
}

type parityReport struct {
	Note       string            `json:"note"`
	Headline   parityHeadline    `json:"headline"`
	Scan       webScan           `json:"web_app_scan"`
	Groups     []parityGroupRow  `json:"groups"`
	Operations []parityOperation `json:"operations"`
}

const mcpParityNote = "Generated by `" + mcpParityCommand + "`; do not edit by hand. " +
	"Parity is operations an agent reaches over operations a signed-in user reaches. It reads " +
	"backend/api/crm.yaml, the web app's source, the role sweep's matrix and checks, the grant checks in the handlers and the seeded role matrix."

// parityContractOp is the part of one crm.yaml operation this page reads.
type parityContractOp struct {
	OperationID string   `yaml:"operationId"` //nolint:tagliatelle // OpenAPI's key, read as written.
	Tags        []string `yaml:"tags"`
	MCPTool     *struct {
		Verb       string `yaml:"verb"`
		Tier       string `yaml:"tier"`
		Scope      string `yaml:"scope"`
		Router     string `yaml:"router"`
		Action     string `yaml:"action"`
		AskFirst   bool   `yaml:"ask_user_first"`
		RecordType string `yaml:"record_type"`
	} `yaml:"x-mcp-tool"` //nolint:tagliatelle // the contract's own extension key.
	AgentAccess string                 `yaml:"x-agent-access"`      //nolint:tagliatelle // the contract's own extension key.
	Reason      string                 `yaml:"x-human-only-reason"` //nolint:tagliatelle // the contract's own extension key.
	Audience    string                 `yaml:"x-settings-audience"` //nolint:tagliatelle // the contract's own extension key.
	Security    *[]map[string][]string `yaml:"security"`
}

type parityContract struct {
	// routes is keyed by path and upper-case method.
	routes map[string]map[string]parityContractOp
	tags   []string
	// rawOperationIDs counts the operationId lines in the text, which no decoder
	// can fall short of.
	rawOperationIDs int
}

var parityContractOperationID = regexp.MustCompile(`(?m)^\s+operationId:`)

// readParityContract decodes each operation by its method node, as gen-agentpolicy does.
func readParityContract(t *testing.T) parityContract {
	t.Helper()
	raw, err := os.ReadFile(contractDocument)
	if err != nil {
		t.Fatalf("reading %s: %v", contractDocument, err)
	}
	var doc struct {
		Tags []struct {
			Name string `yaml:"name"`
		} `yaml:"tags"`
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", contractDocument, err)
	}
	contract := parityContract{
		routes:          map[string]map[string]parityContractOp{},
		rawOperationIDs: len(parityContractOperationID.FindAll(raw, -1)),
	}
	for _, tag := range doc.Tags {
		contract.tags = append(contract.tags, tag.Name)
	}
	for route, item := range doc.Paths {
		for method, node := range item {
			if !httpMethods[method] {
				continue
			}
			var op parityContractOp
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s %s: %v", method, route, err)
			}
			if contract.routes[route] == nil {
				contract.routes[route] = map[string]parityContractOp{}
			}
			contract.routes[route][strings.ToUpper(method)] = op
			contract.tags = append(contract.tags, op.Tags...)
		}
	}
	slices.Sort(contract.tags)
	contract.tags = slices.Compact(contract.tags)
	return contract
}

func (c parityContract) routeMethods() contractRoutes {
	out := contractRoutes{}
	for route, methods := range c.routes {
		out[route] = slices.Sorted(maps.Keys(methods))
	}
	return out
}

// verbUses counts the operations each verb serves. A sweep row for a verb that
// serves many says nothing about any one of them.
func (c parityContract) verbUses() map[string]int {
	out := map[string]int{}
	for _, methods := range c.routes {
		for _, op := range methods {
			if op.MCPTool != nil {
				out[op.MCPTool.Verb]++
			}
		}
	}
	return out
}

// sessionReaches is the rule gen-agentpolicy holds a human-only operation to: a
// signed-in user's cookie admits it, and absent security inherits the document's,
// which admits it. The generator's copy sits in a package main, which this package
// cannot import, so assertStatesMatchSessionRule holds the two together.
func sessionReaches(security *[]map[string][]string) bool {
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

// parityStateOf reads what the contract declares. It answers empty for a shape
// gen-agentpolicy should have refused, so a new annotation cannot pass as a count.
func parityStateOf(op parityContractOp) string {
	switch {
	case !sessionReaches(op.Security):
		return stateNoSession
	case op.AgentAccess == "auth-bootstrap":
		return stateSignIn
	case op.AgentAccess == humanOnlyAccess && slices.Contains([]string{statePending, stateSecurity, stateCovered}, op.Reason):
		return op.Reason
	case op.AgentAccess == "" && op.MCPTool != nil:
		return stateOpen
	}
	return ""
}

// measuredRep is what the tool role sweep saw the rep role do, per verb. That sweep
// runs the real handler, so a refusal on any action of a verb stands.
func measuredRep(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(toolRolesJSON)
	if err != nil {
		t.Fatalf("reading %s: %v", toolRolesJSON, err)
	}
	var doc struct {
		Rows []struct {
			Verb    string   `json:"verb"`
			Allowed []string `json:"allowed_for"`
			Refused []string `json:"refused_for"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decoding %s: %v", toolRolesJSON, err)
	}
	out := map[string]bool{}
	for _, row := range doc.Rows {
		switch {
		case slices.Contains(row.Refused, repRole):
			out[row.Verb] = false
		case slices.Contains(row.Allowed, repRole):
			if _, seen := out[row.Verb]; !seen {
				out[row.Verb] = true
			}
		}
	}
	return out
}

// repFromChecks judges the checks on the handler. A main-line check the rep lacks
// refuses it. Otherwise an unresolved check, or a conditional one the rep lacks,
// leaves the answer open, because only reading the branch can say who it stops.
func repFromChecks(grants handlerGrants, roles roleGrants) (passes bool, decidedBy, refusedBy string) {
	open := false
	for _, grant := range grants.required {
		held, known := roles.holds(repRole, grant)
		if known && !held {
			return false, decidedChecks, grant
		}
		open = open || !known
	}
	for _, grant := range grants.conditional {
		if held, known := roles.holds(repRole, grant); !known || !held {
			open = true
		}
	}
	switch {
	case open:
		return false, "", ""
	case len(grants.required)+len(grants.conditional) == 0:
		return true, decidedNoCheck, ""
	}
	return true, decidedChecks, ""
}

// parityInputs are the sources the report reads, apart from the contract.
type parityInputs struct {
	scan     webScan
	source   *grantSource
	measured map[string]bool
	// checks is what the sweep recorded each verb's calls asking.
	checks map[string][]string
	roles  roleGrants
}

func buildParityReport(t testing.TB, contract parityContract, in parityInputs) parityReport {
	t.Helper()
	areas := map[string]parityArea{}
	for _, area := range parityAreas {
		areas[area.Key] = area
	}
	verbs := contract.verbUses()
	var ops []parityOperation
	for _, route := range slices.Sorted(maps.Keys(contract.routes)) {
		for _, method := range slices.Sorted(maps.Keys(contract.routes[route])) {
			declared := contract.routes[route][method]
			area, placed := areas[parityAreaOf(route, declared.Tags)]
			if !placed {
				t.Errorf("%s %s (tags %v) has no place in the MCP parity catalog. Add a rule to parityPathAreas or "+
					"parityTagAreas in gates/mcpparityplaces_test.go, then run: %s",
					method, route, declared.Tags, mcpParityCommand)
				continue
			}
			op := parityOperation{
				Method: method, Path: route, OperationID: declared.OperationID, Tags: declared.Tags,
				Group: area.Group, Area: area.Key, State: parityStateOf(declared), WebApp: in.scan.calls[method+" "+route],
			}
			if tool := declared.MCPTool; tool != nil {
				op.AskFirst, op.Tier, op.Scope = tool.AskFirst, tool.Tier, tool.Scope
				op.Verb, op.Router, op.Action = tool.Verb, tool.Router, tool.Action
			}
			if area.Group == groupSettingsByGrant {
				placeSettingsOperation(t, &op, declared, verbs, in)
			} else if declared.Audience != "" {
				t.Errorf("%s %s declares x-settings-audience, but its area %q is not split by grant. Remove it.", method, route, area.Key)
			}
			ops = append(ops, op)
		}
	}
	return summarizeParity(in.scan, ops)
}

// placeSettingsOperation settles the group of a settings operation. The contract
// states an audience only where the source cannot answer. A declaration the source
// could have answered is refused, so it cannot go stale beside the grants.
func placeSettingsOperation(t testing.TB, op *parityOperation, declared parityContractOp, verbs map[string]int, in parityInputs) {
	t.Helper()
	// Admin until the rep is shown to pass, so a finding below leaves a real group.
	op.Group = groupAdmin
	if declared.Audience != "" && declared.Audience != audienceMember && declared.Audience != audienceAdmin {
		t.Errorf("%s %s: x-settings-audience is %q; say %s or %s", op.Method, op.Path, declared.Audience, audienceMember, audienceAdmin)
	}
	passes, decidedBy, refusedBy, swept := sweptRep(t, *op, verbs, in)
	var grants handlerGrants
	if !swept {
		var served bool
		grants, served = in.source.grantsOf(op.OperationID)
		if !served {
			t.Errorf("%s %s: compose.Server serves no %s, so the grant walk read the wrong tree", op.Method, op.Path, handlerName(op.OperationID))
			return
		}
		passes, decidedBy, refusedBy = repFromChecks(grants, in.roles)
	}
	switch {
	case decidedBy != "" && declared.Audience != "":
		t.Errorf("%s %s declares x-settings-audience, but %s already answers for the rep role. Remove the declaration.", op.Method, op.Path, decidedBy)
	case decidedBy == "" && declared.Audience == "":
		t.Errorf("%s %s: the source cannot say whether a non-admin reaches this (required %v, conditional %v). Read the "+
			"handler, then add x-settings-audience: member or admin in api/crm.yaml.", op.Method, op.Path, grants.required, grants.conditional)
	case decidedBy == "":
		passes, decidedBy = declared.Audience == audienceMember, decidedDeclared
	}
	op.RefusedBy = refusedBy
	op.RepPasses, op.DecidedBy = &passes, decidedBy
	if passes {
		op.Group = groupWorkspace
	}
}

func summarizeParity(scan webScan, ops []parityOperation) parityReport {
	report := parityReport{Note: mcpParityNote, Scan: scan, Operations: ops}
	groups := map[string]*parityGroupRow{}
	areasByKey := map[string]map[string]*parityAreaRow{}
	titles := map[string]string{}
	for _, area := range parityAreas {
		titles[area.Key] = area.Title
	}
	for _, info := range parityGroupInfos {
		groups[info.Key] = &parityGroupRow{Key: info.Key, Title: info.Title, What: info.What}
		areasByKey[info.Key] = map[string]*parityAreaRow{}
	}
	for _, op := range ops {
		report.Headline.AllOperations++
		if op.State != stateNoSession {
			report.Headline.SignedIn.add(op)
			if op.State == stateOpen && op.WebApp == "" {
				report.Headline.OpenWithNoWebCallFound++
			}
		}
		groups[op.Group].add(op)
		row := areasByKey[op.Group][op.Area]
		if row == nil {
			row = &parityAreaRow{Key: op.Area, Title: titles[op.Area]}
			areasByKey[op.Group][op.Area] = row
		}
		row.add(op)
	}
	for _, info := range parityGroupInfos {
		row := groups[info.Key]
		for _, area := range parityAreas {
			if found := areasByKey[info.Key][area.Key]; found != nil {
				row.Areas = append(row.Areas, *found)
			}
		}
		report.Groups = append(report.Groups, *row)
	}
	signed := report.Headline.SignedIn
	report.Headline.WebParityWithCovered = percentOf(signed.WebOpen+signed.WebCovered, signed.WebCalls)
	report.Headline.WebParityOncePending = percentOf(signed.WebOpen+signed.WebCovered+signed.WebPending, signed.WebCalls)
	return report
}

func TestTheMCPParityCatalogIsPublished(t *testing.T) {
	contract := readParityContract(t)
	routes := contract.routeMethods()
	scan, strays, err := scanWebApp(routes)
	if err != nil {
		t.Fatalf("reading the web app's source: %v", err)
	}
	for _, stray := range strays {
		t.Errorf("the web app's typed client names a route crm.yaml does not serve: %s. The client cannot "+
			"compile with one, so the scan misread this call.", stray)
	}
	in := parityInputs{scan: scan, source: loadGrantSource(t), measured: measuredRep(t), checks: sweptChecks(t), roles: readRoleGrants(t)}
	report := buildParityReport(t, contract, in)

	assertParityCensus(t, contract, report, in)
	assertStatesMatchSessionRule(t, contract, report)
	assertThePlacesTableHasNoStaleRow(t, contract)
	assertGrantWalkSeesKnownChecks(t, in)
	if t.Failed() {
		return
	}
	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("encoding the parity report: %v", err)
	}
	syncMCPParity(t, mcpParityJSON, append(payload, '\n'))
	syncMCPParity(t, mcpParityPage, renderMCPParityPage(report))
	syncMCPParity(t, workAreaTable, workAreaFile(t, contract, report))
	syncToolRolesPage(t, report)
}

// assertParityCensus fails a scan that saw too little, whatever it then computed.
func assertParityCensus(t *testing.T, contract parityContract, report parityReport, in parityInputs) {
	t.Helper()
	signedIn, settings := report.Headline.SignedIn.Operations, 0
	for _, op := range report.Operations {
		if op.DecidedBy != "" {
			settings++
		}
	}
	for _, floor := range []struct {
		what         string
		got, atLeast int
	}{
		{"operations in the contract", report.Headline.AllOperations, parityOperationFloor},
		{"operations a signed-in user reaches", signedIn, paritySignedInFloor},
		{"web app source files", in.scan.files, parityWebFileFloor},
		{"typed web app calls", in.scan.directCalls, parityWebCallFloor},
		{"settings operations judged by grant", settings, paritySettingsFloor},
		{"objects in the seeded role matrix", len(in.roles), parityRoleObjectFloor},
	} {
		if floor.got < floor.atLeast {
			t.Errorf("the parity census read %d %s and expects at least %d, so its walk has stopped finding them "+
				"rather than the tree having shrunk", floor.got, floor.what, floor.atLeast)
		}
	}
	if report.Headline.AllOperations != contract.rawOperationIDs {
		t.Errorf("the catalog holds %d operations and %s has %d operationId lines: one went unplaced or undecoded",
			report.Headline.AllOperations, contractDocument, contract.rawOperationIDs)
	}
}

// assertStatesMatchSessionRule holds this page's reading of the contract to the
// generator's: a human-only operation states a reason only if a user reaches it.
func assertStatesMatchSessionRule(t *testing.T, contract parityContract, report parityReport) {
	t.Helper()
	for _, op := range report.Operations {
		declared := contract.routes[op.Path][op.Method]
		switch {
		case op.State == "":
			t.Errorf("%s %s is in no state this page knows: x-agent-access %q, reason %q. Teach parityStateOf.",
				op.Method, op.Path, declared.AgentAccess, declared.Reason)
		case op.State == stateNoSession && (declared.MCPTool != nil || declared.Reason != ""):
			t.Errorf("%s %s admits no signed-in user, yet declares an agent policy for one", op.Method, op.Path)
		}
		if isPublic := op.Group == groupPublic; isPublic != (op.State == stateNoSession) {
			t.Errorf("%s %s is in group %q with state %q: the public group holds the operations no signed-in user "+
				"reaches, and only those", op.Method, op.Path, op.Group, op.State)
		}
	}
}

// assertThePlacesTableHasNoStaleRow fails a rule nothing uses and a tag with no
// decision, so a tag added to the contract cannot pass unplaced.
func assertThePlacesTableHasNoStaleRow(t *testing.T, contract parityContract) {
	t.Helper()
	used := map[string]bool{}
	for route, methods := range contract.routes {
		for prefix := range parityPathAreas {
			if pathUnderPrefix(route, prefix) {
				used["path "+prefix] = true
			}
		}
		for _, op := range methods {
			if len(op.Tags) > 0 {
				used["tag "+op.Tags[0]] = true
			}
		}
	}
	for prefix := range parityPathAreas {
		if !used["path "+prefix] {
			t.Errorf("parityPathAreas has a rule for %s, which matches no contract path. Remove it.", prefix)
		}
	}
	for tag := range parityTagAreas {
		if !slices.Contains(contract.tags, tag) {
			t.Errorf("parityTagAreas names the tag %s, which the contract does not use. Remove it.", tag)
		}
	}
	for _, tag := range contract.tags {
		_, hasDefault := parityTagAreas[tag]
		if hasDefault == parityMixedTags[tag] {
			t.Errorf("the contract tag %s must have a default area in parityTagAreas or a line in parityMixedTags, "+
				"and not both", tag)
		}
	}
	known := map[string]bool{}
	for _, area := range parityAreas {
		known[area.Key] = true
	}
	for _, area := range slices.Concat(slices.Collect(maps.Values(parityPathAreas)), slices.Collect(maps.Values(parityTagAreas))) {
		if !known[area] {
			t.Errorf("a parity place names the area %q, which parityAreas does not declare", area)
		}
	}
}

// assertGrantWalkSeesKnownChecks pins one handler per path a check can hide on, so a
// walk that stopped reaching them fails rather than calling a setting ungated.
func assertGrantWalkSeesKnownChecks(t *testing.T, in parityInputs) {
	t.Helper()
	for _, pin := range []struct{ why, operationID, grant string }{
		{"a store method on a handler field", "listPipelines", "pipeline:read"},
		{"a method value handed to a helper", "archiveRole", "role_admin:delete"},
		{"auth.RequireAdmin", "putAnchorCompany", adminRoleGrant},
		{"a stored setting's own object", "getFollowUpSettings", "installation_settings:read"},
		{"auth.RequireAny with its actions", "proposeFxRateRefresh", "fx_rate:create|update"},
	} {
		grants, served := in.source.grantsOf(pin.operationID)
		if !served {
			t.Errorf("compose.Server serves no handler for %s", pin.operationID)
			continue
		}
		if !slices.Contains(grants.required, pin.grant) {
			t.Errorf("the grant walk reads %v on %s; it should see %s through %s", grants.required, pin.operationID, pin.grant, pin.why)
		}
	}
	for _, pin := range []struct{ why, operationID, grant string }{
		{"auth.Allows read as a boolean", "importLinkedInConnections", "contact:update"},
		{"auth.ReadGranted", "listCaptureSenders", "contact:read"},
		{"a check inside an auth helper", "getLeadSettings", "user_admin:delete"},
	} {
		if grants, _ := in.source.grantsOf(pin.operationID); !slices.Contains(grants.conditional, pin.grant) {
			t.Errorf("the grant walk reads %v as conditional on %s; it should see %s through %s", grants.conditional, pin.operationID, pin.grant, pin.why)
		}
	}
	for name, fn := range in.source.methods {
		if strings.Contains(fn.FullName(), ".stubs).") {
			t.Errorf("compose.Server answers %s from its stubs, so the grant walk would read a 501", name)
		}
	}
}

func syncMCPParity(t *testing.T, path string, want []byte) {
	t.Helper()
	if *updateMCPParity {
		if err := os.WriteFile(path, want, 0o600); err != nil {
			t.Fatalf("rewriting %s: %v", path, err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v\nRegenerate with: %s", path, err, mcpParityCommand)
	}
	if string(got) != string(want) {
		t.Errorf("%s no longer matches the contract, the web app and the seeded grants.\nRegenerate with: %s\n"+
			"and commit the result with the change that moved it.\n%s", path, mcpParityCommand, firstParityDifference(got, want))
	}
}

func firstParityDifference(got, want []byte) string {
	have, need := strings.Split(string(got), "\n"), strings.Split(string(want), "\n")
	for i := range min(len(have), len(need)) {
		if have[i] != need[i] {
			return fmt.Sprintf("line %d has:\n\t%s\nand should have:\n\t%s", i+1, have[i], need[i])
		}
	}
	return "one file is a prefix of the other"
}

// A new tag and a new route under a mixed tag both fail to place. A tag default and
// a longer path rule both land where the table says.
func TestParityPlacesReadPathsBeforeTags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path string
		tags       []string
		want       string
	}{
		{"a tag default", "/contacts/{id}/note", []string{"Contacts"}, "contacts"},
		{"a path rule over the tag", "/contacts/{id}/consent/x", []string{"Contacts"}, "consent"},
		{"the longest path rule", "/me/mfa/totp", []string{"Identity"}, "own_security"},
		{"a whole segment only", "/companies/{id}", []string{"Companies"}, "companies"},
		{"a custom verb on a segment", "/company:preview", []string{"Companies"}, "company_profile"},
		{"a new tag", "/brand-new", []string{"BrandNew"}, ""},
		{"a new route under a mixed tag", "/brand-new", []string{"Identity"}, ""},
		{"no tag", "/brand-new", nil, ""},
	} {
		if got := parityAreaOf(tc.path, tc.tags); got != tc.want {
			t.Errorf("%s: %s %v places in %q, want %q", tc.name, tc.path, tc.tags, got, tc.want)
		}
	}
}

func TestACallsEndIsNotAQuotedDelimiter(t *testing.T) {
	t.Parallel()
	for _, code := range []string{
		`, { body: "a) b; c", method: "DELETE" });`,
		`, { body: 'it\'s )', method: "DELETE" });`,
		", { body: `)`, method: \"DELETE\" });",
	} {
		end := ownCallEnd(code, 0)
		if !strings.Contains(code[:end], "DELETE") {
			t.Errorf("the call ended at %q, before its method", code[:end])
		}
	}
}

func TestTheWebScanReadsEachFormOfCall(t *testing.T) {
	t.Parallel()
	routes := contractRoutes{
		"/deals":                 {"GET", "POST"},
		"/deals/{id}":            {"GET", "DELETE"},
		"/deal-rooms/{id}/close": {"POST"},
		"/deal-rooms/{id}/pause": {"POST"},
		"/offers/{id}/pdf":       {"GET"},
		"/company/logo":          {"POST", "DELETE"},
		"/hidden":                {"GET"},
		"/uncalled":              {"GET"},
	}
	scan, strays := scanWebSources(map[string]string{
		"direct.ts":   "await api.GET(\n  \"/deals\",\n);\nanonymous\n  .POST(\"/deals\", {});",
		"comment.ts":  "// api.GET(\"/hidden\")\n/* api.GET(\"/hidden\") */ const url = \"http://x//y\";",
		"constant.ts": "const PATH = \"/deals/{id}\";\nawait api.DELETE(PATH, {});",
		"hole.ts":     "const path = `/deal-rooms/{id}/${verb}` as const;\nawait api.POST(path, {});",
		"link.tsx":    "const a = `${origin}/v1/offers/${id}/pdf`;",
		"upload.ts":   "useMarkWrites(\"/v1/company/logo\", settle);",
		"stray.ts":    "await api.GET(\"/not-in-contract\");",
		"unit.ts":     "await api.GET(\"/ext/unit/thing\");",
		"unknown.ts":  "await api.GET(pathFromAnotherFile);",
	}, routes)
	want := map[string]string{
		"GET /deals":                  webCallDirect,
		"POST /deals":                 webCallDirect,
		"DELETE /deals/{id}":          webCallInferred,
		"POST /deal-rooms/{id}/close": webCallInferred,
		"POST /deal-rooms/{id}/pause": webCallInferred,
		"GET /offers/{id}/pdf":        webCallURL,
		"POST /company/logo":          webCallURL,
	}
	if !maps.Equal(scan.calls, want) {
		t.Errorf("the scan found %v, want %v", scan.calls, want)
	}
	if !slices.Equal(strays, []string{"stray.ts GET /not-in-contract"}) {
		t.Errorf("strays are %v, want only the literal the contract does not serve", strays)
	}
	if scan.unitCalls != 1 {
		t.Errorf("counted %d calls to a unit's route, want 1", scan.unitCalls)
	}
	if !slices.Equal(scan.Unresolved, []string{"unknown.ts GET"}) {
		t.Errorf("unresolved calls are %v, want the one whose path lives elsewhere", scan.Unresolved)
	}
}

// parityFindings is a testing.TB that keeps each Errorf, so a test can read what
// the catalog reports on a planted contract.
type parityFindings struct {
	testing.TB
	messages []string
}

func (f *parityFindings) Helper() {}

func (f *parityFindings) Errorf(format string, args ...any) {
	f.messages = append(f.messages, fmt.Sprintf(format, args...))
}

// A settings operation stays in the census and reaches a real group when its
// handler cannot be read, and the finding names it.
func TestAnUnreadableSettingsOperationIsReportedAndStillCounted(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, audience, want string
	}{
		{"no handler", "", "serves no ListPipelines"},
		{"an audience neither side reads", "everyone", `x-settings-audience is "everyone"`},
	} {
		contract := parityContract{routes: map[string]map[string]parityContractOp{
			"/pipelines": {"GET": {OperationID: "listPipelines", Tags: []string{"Pipelines"}, Audience: tc.audience}},
		}}
		findings := &parityFindings{TB: t}
		report := buildParityReport(findings, contract, parityInputs{source: &grantSource{methods: map[string]*types.Func{}}})
		if !strings.Contains(strings.Join(findings.messages, "\n"), tc.want) {
			t.Errorf("%s: findings are %q, want one with %q", tc.name, findings.messages, tc.want)
		}
		if report.Headline.AllOperations != 1 || report.Operations[0].Group != groupAdmin {
			t.Errorf("%s: counted %d operations in %+v, want the one in the admin group", tc.name, report.Headline.AllOperations, report.Operations)
		}
	}
}

// The rows partition the web app's operations, so they sum to the count the
// headline divides by. A row that restated a part of another would not.
func TestTheBreakdownRowsSumToTheWebAppCalls(t *testing.T) {
	t.Parallel()
	var report parityReport
	for _, op := range []parityOperation{
		{State: stateOpen, WebApp: webCallDirect},
		{State: stateOpen, WebApp: webCallDirect, AskFirst: true},
		{State: statePending, WebApp: webCallDirect},
		{State: stateCovered, WebApp: webCallDirect},
		{State: stateSecurity, WebApp: webCallDirect},
		{State: stateSignIn, WebApp: webCallDirect},
	} {
		report.Headline.SignedIn.add(op)
	}
	var page bytes.Buffer
	renderParityHeadline(&page, report)
	_, table, found := strings.Cut(page.String(), "| Of the operations the web app calls | Count |\n|---|---:|\n")
	if !found {
		t.Fatalf("the headline has no breakdown table:\n%s", page.String())
	}
	sum := 0
	for row := range strings.SplitSeq(strings.TrimSpace(table), "\n") {
		cells := tableCells(strings.Trim(row, "|"))
		count, err := strconv.Atoi(cells[len(cells)-1])
		if err != nil {
			t.Fatalf("row %q has no count: %v", row, err)
		}
		sum += count
	}
	if sum != report.Headline.SignedIn.WebCalls {
		t.Errorf("the rows sum to %d and the web app calls %d:\n%s", sum, report.Headline.SignedIn.WebCalls, table)
	}
}

// A path built on a constant in the same file is that one path. The constant's own
// route gains no call, and the longer route is not lost.
func TestTheWebScanReadsAPathBuiltOnAConstant(t *testing.T) {
	t.Parallel()
	routes := contractRoutes{
		"/offers/{id}":            {"GET", "POST"},
		"/offers/{id}/candidates": {"GET"},
		"/offers/{id}/history":    {"GET"},
		"/notes":                  {"GET"},
		"/tags":                   {"GET"},
	}
	const offer = "const OFFER = \"/offers/{id}\";\n"
	inferred := func(paths ...string) map[string]string {
		out := map[string]string{}
		for _, path := range paths {
			out["GET "+path] = webCallInferred
		}
		return out
	}
	for _, tc := range []struct {
		name, source string
		want         map[string]string
		strays       []string
		unresolved   []string
	}{
		{
			name: "a suffix on a constant", source: offer + "api.GET(`${OFFER}/candidates`, {});",
			want: inferred("/offers/{id}/candidates"),
		},
		{
			name: "a constant holding the suffix", source: offer + "const MORE = `${OFFER}/candidates`;\napi.GET(MORE, {});",
			want: inferred("/offers/{id}/candidates"),
		},
		{
			name: "a hole after the constant", source: offer + "api.GET(`${OFFER}/${tab}`, {});",
			want: inferred("/offers/{id}/candidates", "/offers/{id}/history"),
		},
		{
			name: "one constant of two", source: "const NOTES = \"/notes\";\nexport const TAGS = \"/tags\";\napi.GET(NOTES);",
			want: inferred("/notes"),
		},
		{
			name: "a suffix inside a choice", source: offer + "const path = on ? `${OFFER}/candidates` : `${OFFER}/history`;\napi.GET(path);",
			want: inferred("/offers/{id}/candidates", "/offers/{id}/history"),
		},
		{
			name: "a name declared twice is not one path", source: "const path = \"/notes\";\nfunction f() {\n  const path = \"/tags\";\n  api.GET(path);\n}",
			want: inferred("/notes", "/tags"),
		},
		{
			name: "a suffix the contract does not serve", source: offer + "api.GET(`${OFFER}/gone`, {});",
			want: map[string]string{}, strays: []string{"f.ts GET /offers/{id}/gone"},
		},
		{
			name: "a constant not joined by the whole value", source: "const NOTES = \"/notes\" + extra;\napi.GET(NOTES);",
			want: inferred("/notes"),
		},
		{
			name: "a constant nothing uses", source: "const UNUSED = \"/notes\";\napi.GET(elsewhere);",
			want: map[string]string{}, unresolved: []string{"f.ts GET"},
		},
		{
			name: "a name from another file", source: "api.GET(elsewhere);",
			want: map[string]string{}, unresolved: []string{"f.ts GET"},
		},
	} {
		scan, strays := scanWebSources(map[string]string{"f.ts": tc.source}, routes)
		if !maps.Equal(scan.calls, tc.want) {
			t.Errorf("%s: the scan found %v, want %v", tc.name, scan.calls, tc.want)
		}
		if !slices.Equal(strays, tc.strays) {
			t.Errorf("%s: strays are %v, want %v", tc.name, strays, tc.strays)
		}
		if !slices.Equal(scan.Unresolved, append([]string{}, tc.unresolved...)) {
			t.Errorf("%s: unresolved calls are %v, want %v", tc.name, scan.Unresolved, tc.unresolved)
		}
	}
}

// A method key counts only inside the URL's own call.
func TestAURLTakesItsMethodFromItsOwnCall(t *testing.T) {
	t.Parallel()
	routes := contractRoutes{"/offers/{id}/pdf": {"GET", "POST"}, "/company/logo": {"GET", "DELETE"}}
	scan, _ := scanWebSources(map[string]string{
		"link.ts":   "const a = open(`/v1/offers/${id}/pdf`);\nsend({ method: \"POST\" });",
		"delete.ts": "fetch(\"/v1/company/logo\", { credentials: \"include\", method: \"DELETE\" });",
	}, routes)
	want := map[string]string{"GET /offers/{id}/pdf": webCallURL, "DELETE /company/logo": webCallURL}
	if !maps.Equal(scan.calls, want) {
		t.Errorf("the scan found %v, want %v", scan.calls, want)
	}
}

// parityStateOf reads the two annotations in the order gen-agentpolicy does, where
// x-agent-access decides and an agent is refused.
func TestAHumanOnlyDeclarationOutranksATool(t *testing.T) {
	t.Parallel()
	var op parityContractOp
	declared := "x-mcp-tool: { verb: read_view, tier: auto_execute, scope: read }\nx-agent-access: human-only\nx-human-only-reason: pending\n"
	if err := yaml.Unmarshal([]byte(declared), &op); err != nil {
		t.Fatalf("decoding the planted operation: %v", err)
	}
	if got := parityStateOf(op); got != statePending {
		t.Errorf("parityStateOf = %q, want %q", got, statePending)
	}
}

func TestRepFromChecksLeavesOnlyTheUnreadableOpen(t *testing.T) {
	t.Parallel()
	roles := roleGrants{"deal": {repRole: "CRU-"}, "role_admin": {repRole: "----"}}
	for _, tc := range []struct {
		name      string
		grants    handlerGrants
		passes    bool
		decidedBy string
	}{
		{"a held check", handlerGrants{required: []string{"deal:read"}}, true, decidedChecks},
		{"a missing check", handlerGrants{required: []string{"role_admin:read"}}, false, decidedChecks},
		{"a missing check beside an unknown one", handlerGrants{required: []string{"?:read", "role_admin:read"}}, false, decidedChecks},
		{"the admin role", handlerGrants{required: []string{adminRoleGrant}}, false, decidedChecks},
		{"any of two actions", handlerGrants{required: []string{"deal:delete|update"}}, true, decidedChecks},
		{"no check at all", handlerGrants{}, true, decidedNoCheck},
		{"an unknown check", handlerGrants{required: []string{"?:read"}}, false, ""},
		{"a conditional check the rep lacks", handlerGrants{conditional: []string{"role_admin:read"}}, false, ""},
		{"a conditional check the rep holds", handlerGrants{conditional: []string{"deal:read"}}, true, decidedChecks},
	} {
		passes, decidedBy, _ := repFromChecks(tc.grants, roles)
		if passes != tc.passes || decidedBy != tc.decidedBy {
			t.Errorf("%s: got passes=%v by %q, want passes=%v by %q", tc.name, passes, decidedBy, tc.passes, tc.decidedBy)
		}
	}
}
