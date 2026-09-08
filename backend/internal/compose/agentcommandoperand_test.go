// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The REST door's half of the eight bespoke commands (agentcommandoperand.go):
// the routed {id}'s existence-hiding 404, the second path operand's 422, and
// the staged target each decoder resolves to — the same proof shape
// agentcommand_test.go gives archive/patch, for a family whose operand lives
// in a SECOND path parameter rather than in the body.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	chi "github.com/go-chi/chi/v5"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// operandRequest builds a request for a route carrying the router's own {id}
// (as the raw path segment routeID — a malformed one is what proves the 404)
// plus an optional second path parameter the chi router would have bound —
// factKey, field, or person_id.
func operandRequest(method, path, routeID, extraParam, extraValue string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path+"/"+routeID, bytes.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", routeID)
	if extraParam != "" {
		rctx.URLParams.Add(extraParam, extraValue)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// A malformed routed {id} answers 404 for every one of the eight, the same
// existence-hiding answer archiveCommand/patchCommand already give — proven
// once per decoder rather than assuming the shared routedID helper carries
// the property for free.
func TestAMalformedOperandRouteIDAnswersNotFound(t *testing.T) {
	cases := []struct {
		name   string
		decode func(pol agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error)
		req    *http.Request
	}{
		{"confirmOrganizationFact", confirmFactCommand, operandRequest(http.MethodPost, "/v1/organizations", "not-a-uuid", "factKey", "k", nil)},
		{"updateOrganizationFact", updateFactCommand, operandRequest(http.MethodPatch, "/v1/organizations", "not-a-uuid", "factKey", "k", []byte(`{"value":"v"}`))},
		{"confirmOrganizationProfileField", confirmProfileFieldCommand, operandRequest(http.MethodPost, "/v1/organizations", "not-a-uuid", "field", "icp", nil)},
		{"updateOrganizationProfileField", updateProfileFieldCommand, operandRequest(http.MethodPatch, "/v1/organizations", "not-a-uuid", "field", "icp", []byte(`{"value":"v"}`))},
		{"retireCustomField", retireCustomFieldCommand, operandRequest(http.MethodPost, "/v1/custom-fields", "not-a-uuid", "", "", nil)},
		{"updateCustomFieldOptions", updateCustomFieldOptionsCommand, operandRequest(http.MethodPatch, "/v1/custom-fields", "not-a-uuid", "", "", []byte(`{"options":["a"]}`))},
		{"setProjectStakeholder", setStakeholderCommand, operandRequest(http.MethodPut, "/v1/projects", "not-a-uuid", "", "", []byte(`{"person_id":"018f2a10-0000-7000-8000-000000000001","role":"champion"}`))},
		{"removeProjectStakeholder", removeStakeholderCommand, operandRequest(http.MethodDelete, "/v1/projects", "not-a-uuid", "person_id", ids.NewV7().String(), nil)},
		{"setProjectCompany", setCompanyCommand, operandRequest(http.MethodPut, "/v1/projects", "not-a-uuid", "", "", []byte(`{"organization_id":"018f2a10-0000-7000-8000-000000000002","role":"partner"}`))},
		{"removeProjectCompany", removeCompanyCommand, operandRequest(http.MethodDelete, "/v1/projects", "not-a-uuid", "organization_id", ids.NewV7().String(), nil)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.decode(agentPolicy{Op: c.name}, restCommandDeps{records: seamRecord{}}, c.req, nil); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("decoding a malformed id answered %v, want the not-found sentinel", err)
			}
		})
	}
}

// A missing second path operand — a request built without the segment the
// router would otherwise have bound — answers 422 naming it, not a panic on
// an empty FactKey/Field downstream. removeProjectStakeholder's person_id is
// the one operand composed from pathOperand + ids.Parse (agentcommandoperand.go)
// rather than pathOperand alone, so it is included here too: a missing one
// must still answer "missing" through that composition, not fall through to
// ids.Parse("") and answer the malformed-shape code instead.
func TestAMissingSecondPathOperandAnswers422(t *testing.T) {
	id := ids.NewV7().String()
	cases := []struct {
		name      string
		method    string
		path      string
		decode    func(pol agentPolicy, deps restCommandDeps, r *http.Request, body []byte) (agents.GovernedCall, error)
		body      []byte
		wantField string
	}{
		{"confirmOrganizationFact", http.MethodPost, "/v1/organizations", confirmFactCommand, nil, "factKey"},
		{"updateOrganizationFact", http.MethodPatch, "/v1/organizations", updateFactCommand, nil, "factKey"},
		{"confirmOrganizationProfileField", http.MethodPost, "/v1/organizations", confirmProfileFieldCommand, nil, "field"},
		{"updateOrganizationProfileField", http.MethodPatch, "/v1/organizations", updateProfileFieldCommand, nil, "field"},
		{"removeProjectStakeholder", http.MethodDelete, "/v1/projects", removeStakeholderCommand, nil, "person_id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// extraParam left empty: the route matched (a valid {id}), but the
			// second segment the router would bind was never set — the shape a
			// routing bug, not a malformed request, would produce.
			req := operandRequest(c.method, c.path, id, "", "", c.body)
			_, err := c.decode(agentPolicy{Op: c.name}, restCommandDeps{records: seamRecord{}}, req, c.body)
			var detailed *httperr.DetailedError
			if !errors.As(err, &detailed) || detailed.Status != http.StatusUnprocessableEntity {
				t.Fatalf("a missing %s answered %v, want a 422 naming it", c.wantField, err)
			}
			if len(detailed.Fields) != 1 || detailed.Fields[0].Field != c.wantField || detailed.Fields[0].Code != "missing" {
				t.Errorf("the 422 named %+v, want field %q code \"missing\"", detailed.Fields, c.wantField)
			}
		})
	}
}

// A malformed (non-empty) person_id on removeProjectStakeholder is also a
// 422, code "invalid" rather than "missing" — the other half of the
// pathOperand + ids.Parse composition the test above proves the missing
// case for. Neither is the 404 the routed {id} gets: person_id names WHICH
// edge, not whether the project exists, so its shape being wrong is the
// caller's mistake, never an existence leak.
func TestARemoveStakeholderMalformedPersonIDAnswers422(t *testing.T) {
	req := operandRequest(http.MethodDelete, "/v1/projects", ids.NewV7().String(), "person_id", "not-a-uuid", nil)
	_, err := removeStakeholderCommand(agentPolicy{Op: "removeProjectStakeholder"}, restCommandDeps{records: seamRecord{}}, req, nil)
	var detailed *httperr.DetailedError
	if !errors.As(err, &detailed) || detailed.Status != http.StatusUnprocessableEntity {
		t.Fatalf("a malformed person_id answered %v, want a 422", err)
	}
	if len(detailed.Fields) != 1 || detailed.Fields[0].Field != "person_id" || detailed.Fields[0].Code != "invalid" {
		t.Errorf("the 422 named %+v, want field \"person_id\" code \"invalid\"", detailed.Fields)
	}
}

// Each of the eight stages against the routed record it names — proven
// through stageRefusal end to end, the same shape TestAPatchStagesItsRecordAndID
// proves for a whole-record patch.
func TestEachOperandCommandStagesTheRoutedRecord(t *testing.T) {
	orgID, projectID, cfID := ids.NewV7(), ids.NewV7(), ids.NewV7()
	cases := []struct {
		name           string
		pol            agentPolicy
		req            *http.Request
		body           []byte
		wantTargetType string
		wantTargetID   ids.UUID
	}{
		{
			"confirmOrganizationFact",
			agentPolicy{Op: "confirmOrganizationFact", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization},
			operandRequest(http.MethodPost, "/v1/organizations", orgID.String(), "factKey", "named_customer:acme-inc", nil), nil,
			"organization", orgID,
		},
		{
			"updateOrganizationFact",
			agentPolicy{Op: "updateOrganizationFact", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization},
			operandRequest(http.MethodPatch, "/v1/organizations", orgID.String(), "factKey", "named_customer:acme-inc", []byte(`{"value":"Acme Inc"}`)),
			[]byte(`{"value":"Acme Inc"}`), "organization", orgID,
		},
		{
			"confirmOrganizationProfileField",
			agentPolicy{Op: "confirmOrganizationProfileField", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization},
			operandRequest(http.MethodPost, "/v1/organizations", orgID.String(), "field", "icp", nil), nil,
			"organization", orgID,
		},
		{
			"updateOrganizationProfileField",
			agentPolicy{Op: "updateOrganizationProfileField", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization},
			operandRequest(http.MethodPatch, "/v1/organizations", orgID.String(), "field", "icp", []byte(`{"value":"Payments infra"}`)),
			[]byte(`{"value":"Payments infra"}`), "organization", orgID,
		},
		{
			"retireCustomField",
			agentPolicy{Op: "retireCustomField", Access: accessTool, Tool: "update_record", RecordType: recordTypeCustomField},
			operandRequest(http.MethodPost, "/v1/custom-fields", cfID.String(), "", "", nil), nil,
			"custom_field", cfID,
		},
		{
			"updateCustomFieldOptions",
			agentPolicy{Op: "updateCustomFieldOptions", Access: accessTool, Tool: "update_record", RecordType: recordTypeCustomField},
			operandRequest(http.MethodPatch, "/v1/custom-fields", cfID.String(), "", "", []byte(`{"options":["a","b"]}`)),
			[]byte(`{"options":["a","b"]}`), "custom_field", cfID,
		},
		{
			"setProjectStakeholder",
			agentPolicy{Op: "setProjectStakeholder", Access: accessTool, Tool: "update_record", RecordType: recordTypeProject},
			operandRequest(http.MethodPut, "/v1/projects", projectID.String(), "", "", []byte(`{"person_id":"018f2a10-0000-7000-8000-000000000001","role":"champion"}`)),
			[]byte(`{"person_id":"018f2a10-0000-7000-8000-000000000001","role":"champion"}`), "project", projectID,
		},
		{
			"removeProjectStakeholder",
			agentPolicy{Op: "removeProjectStakeholder", Access: accessTool, Tool: "update_record", RecordType: recordTypeProject},
			operandRequest(http.MethodDelete, "/v1/projects", projectID.String(), "person_id", ids.NewV7().String(), nil), nil,
			"project", projectID,
		},
		{
			"setProjectCompany",
			agentPolicy{Op: "setProjectCompany", Access: accessTool, Tool: "update_record", RecordType: recordTypeProject},
			operandRequest(http.MethodPut, "/v1/projects", projectID.String(), "", "", []byte(`{"organization_id":"018f2a10-0000-7000-8000-000000000002","role":"partner"}`)),
			[]byte(`{"organization_id":"018f2a10-0000-7000-8000-000000000002","role":"partner"}`), "project", projectID,
		},
		{
			"removeProjectCompany",
			agentPolicy{Op: "removeProjectCompany", Access: accessTool, Tool: "update_record", RecordType: recordTypeProject},
			operandRequest(http.MethodDelete, "/v1/projects", projectID.String(), "organization_id", ids.NewV7().String(), nil), nil,
			"project", projectID,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			staging := &capturingApprovals{}
			stageRefusal(httptest.NewRecorder(), c.req, staging, restCommandDeps{records: seamRecord{}}, c.pol, c.body)

			if staging.last.TargetType != c.wantTargetType || staging.last.TargetID != c.wantTargetID {
				t.Fatalf("staged target = (%s,%s), want (%s,%s)",
					staging.last.TargetType, staging.last.TargetID, c.wantTargetType, c.wantTargetID)
			}
		})
	}
}

// What resolving these eight through their own commands buys, and it is a
// refusal rather than a label: Guards runs before anything stages.
// An organization or project the caller cannot see stages NOTHING — the same
// proof shape TestAnArchiveOfAnUnseeableRecordStagesNothing gives archive —
// for one op from each seam-served family (organization, project). The two
// custom_field ops have no such proof: the seam has never served that type,
// so there is no read for Guards to skip. That they never attempt one is
// TestCustomFieldCommandsStageAndAdmitOutsideTheRecordSeam's own claim
// (modules/agents/commandaction_test.go), proven there against
// unreadableProvider{} — a provider that fails every read, so a resolver
// that consulted it anyway would fail that test rather than pass here:
// TestEachOperandCommandStagesTheRoutedRecord's use of `seamRecord{}` (every
// read succeeds) cannot tell "never read" apart from "read and got lucky".
func TestAnOperandCommandOfAnUnseeableRecordStagesNothing(t *testing.T) {
	staging := &capturingApprovals{}
	pol := agentPolicy{Op: "confirmOrganizationFact", Access: accessTool, Tool: "update_record", RecordType: recordTypeOrganization}
	req := operandRequest(http.MethodPost, "/v1/organizations", ids.NewV7().String(), "factKey", "named_customer:acme-inc", nil)
	rec := httptest.NewRecorder()

	stageRefusal(rec, req, staging, restCommandDeps{records: hiddenRecord{}}, pol, nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("confirming a fact on an organization the caller cannot see answered %d, want 404 — the "+
			"refusal must not tell a caller that a row they may not see exists", rec.Code)
	}
	if staging.last.Tool != "" {
		t.Errorf("an approval was staged for %q against an organization nobody can decide about", staging.last.Tool)
	}
}

// The other refusal Guards makes: an organization/project the caller CAN see
// but whose authority lives in another system of record — readable, and
// still unstageable, the same shape TestAnArchiveOfAnExternallyHeldRecordStagesNothing
// gives archive.
func TestAnOperandCommandOfARecordHeldElsewhereStagesNothing(t *testing.T) {
	staging := &capturingApprovals{}
	body := []byte(`{"person_id":"018f2a10-0000-7000-8000-000000000001","role":"champion"}`)
	pol := agentPolicy{Op: "setProjectStakeholder", Access: accessTool, Tool: "update_record", RecordType: recordTypeProject}
	req := operandRequest(http.MethodPut, "/v1/projects", ids.NewV7().String(), "", "", body)
	rec := httptest.NewRecorder()

	stageRefusal(rec, req, staging, restCommandDeps{records: mirroredRecord{}}, pol, body)

	if staging.last.Tool != "" {
		t.Errorf("an approval was staged for %q against a project whose authority lives elsewhere — nobody "+
			"could ever release it", staging.last.Tool)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("an externally-held target answered %d, want %d (unsupported_by_sor)", rec.Code, http.StatusUnprocessableEntity)
	}
}

// syntheticOperandRequest builds a well-formed request for route (an
// agentPolicies key, "METHOD /path/{param}/…"), binding {id} to id and every
// OTHER path parameter to a fresh, distinct uuid — enough for any of this
// family's decoders to succeed without this test needing to know which
// parameter names a given route carries (factKey and field accept any
// non-empty string; a uuid satisfies that as well as anything, and is what
// person_id's own ids.Parse requires).
func syntheticOperandRequest(route string, id ids.UUID) *http.Request {
	method, template, _ := strings.Cut(route, " ")
	segments := strings.Split(strings.TrimPrefix(template, "/"), "/")
	rctx := chi.NewRouteContext()
	built := make([]string, 0, len(segments))
	for _, seg := range segments {
		name, isParam := strings.CutPrefix(seg, "{")
		if !isParam {
			built = append(built, seg)
			continue
		}
		name = strings.TrimSuffix(name, "}")
		val := ids.NewV7().String()
		if name == "id" {
			val = id.String()
		}
		rctx.URLParams.Add(name, val)
		built = append(built, val)
	}
	req := httptest.NewRequest(method, "/"+strings.Join(built, "/"), nil)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// What a registration does not say: that the decoder bound to an operation is
// the RIGHT one. TestEveryAgentReachableMutatingRouteDecodesIntoACommand
// (agentcommandcoverage_test.go) proves every one of these routes HAS an entry;
// a mis-wired entry satisfies it completely, because a swapped decoder still
// stages something — often something that looks right, since most of this
// family stages the routed record.
//
// So this walk invokes the bound decoder against a synthetic request for the
// route the policy table names, and checks what it staged against what that
// same table declared: the record type, and the routed id rather than one of
// the route's OTHER path parameters.
//
// THE FAMILY IS DERIVED FROM THE ROUTE'S SHAPE, and that is the whole of the
// filter. A route that names a record with {id} and carries more path after it
// resolves its target from the path, which is what makes a synthetic request
// enough to invoke its decoder. Nothing else narrows the set.
//
// It used to narrow on the tier as well — confirmation_required only, which is
// what the name said. Measured, that reached 2 of the 15 update_record routes in
// this shape and left 13 auto-execute ones unwalked, including both routes the
// walk carried request bodies for: fixtures nothing read, which is how a census
// tells you it has stopped seeing its own subject. Tier is the wrong axis
// besides. A mis-wired decoder is worse under auto_execute than under
// confirmation, because no human is between the wiring and the write.
//
// The whole-record patch shape (route ends exactly at /{id}) stays out, and for
// a reason about the request rather than about the route: those decoders read a
// BODY whose shape is per-operation, where a path parameter is not, so a
// synthetic request cannot exercise them. Their equivalent proof is
// TestAPatchStagesItsRecordAndID and its siblings above, and
// TestEveryWholeRecordPatchRunsItsGuardsBeforeTheAutoExecuteHalf below reaches
// every one of them through the split.
func TestEveryOperandRouteDecodesIntoTheCommandItsPolicyNames(t *testing.T) {
	declared := operationsDeclaringARequestBody(t)
	family := operandShapedRoutes()
	assertTheFamilyIsTheContractsOwn(t, family)
	for route, pol := range family {
		decode, described := restCommands[pol.Op]
		if !described {
			// The completeness gate reports this route by name; here it would
			// only be a nil decoder to dereference.
			continue
		}
		body, written := contractBodies[pol.Op]
		if declaresBody := declared[pol.Op]; declaresBody != written {
			if declaresBody {
				t.Errorf("%s (%s) declares a requestBody in crm.yaml and carries none in contractBodies, so "+
					"the decode below proves only that commandBody short-circuits an empty payload", route, pol.Op)
			} else {
				t.Errorf("%s (%s) declares no requestBody in crm.yaml and carries one in contractBodies, so "+
					"the walk decodes a shape the route cannot produce", route, pol.Op)
			}
			continue
		}
		id := ids.NewV7()
		call, err := decode(pol, operandWalkDeps(), syntheticOperandRequest(route, id), []byte(body))
		if err != nil {
			t.Errorf("%s (%s): decoding a well-formed request answered %v", route, pol.Op, err)
			continue
		}
		info, err := call.Subject(context.Background())
		if err != nil {
			t.Errorf("%s (%s): naming the subject answered %v", route, pol.Op, err)
			continue
		}
		if pol.RecordType != "" && info.TargetType != string(pol.RecordType) {
			t.Errorf("%s (%s) stages target type %q, want %q — the policy table's own declared record type",
				route, pol.Op, info.TargetType, pol.RecordType)
		}
		// A creation binds to a row that does not exist yet, so the routed {id}
		// names its PARENT — the deal a createOffer hangs an offer off, the room
		// a thread opens in. Staging that id would bind a human's yes to the
		// parent record, which is the wrong row to judge and the wrong row to
		// version-pin, so the expectation inverts rather than lapsing: these
		// must stage NO id at all. Read off the tool, not off a list of the
		// three that do it today.
		if pol.Tool == "create_record" {
			if info.TargetID != ids.Nil {
				t.Errorf("%s (%s) stages target id %s, want none — the routed {id} names the PARENT this "+
					"creation hangs off, and an approval bound to it would ask a human about the wrong row",
					route, pol.Op, info.TargetID)
			}
			continue
		}
		// A merge dissolves the record the route names INTO the one its body
		// does, so the survivor is what a human is asked about and what the
		// approval must bind to — the routed id belongs to the record that will
		// not exist. Asserted positively against the body's own target_id
		// rather than skipped: a decoder that staged the routed id here would
		// stage the loser, and "not the routed id" alone would pass on any
		// wrong answer that happened to differ from it.
		if pol.Tool == "merge_records" {
			survivor := bodyTargetID(t, route, pol.Op, body)
			if info.TargetID != survivor {
				t.Errorf("%s (%s) stages target id %s, want the survivor its body names (%s) — a merge's "+
					"approval binds to the record that remains, not the one being folded away",
					route, pol.Op, info.TargetID, survivor)
			}
			continue
		}
		// A route whose policy declares no record type stages no target either:
		// the approval decisions are the only ones, and what they stage instead
		// is TestADecisionStagesNoTargetAndSaysWhichWayItGoes' subject.
		if pol.RecordType == "" {
			continue
		}
		if info.TargetID != id {
			t.Errorf("%s (%s) stages target id %s, want %s — restCommands binds this operationId to the "+
				"WRONG decoder, or the decoder read the wrong path parameter as {id}", route, pol.Op, info.TargetID, id)
		}
	}
}

// operandShapedRoutes are the agent-reachable mutating routes whose target is
// named by a routed {id} the path carries MORE after.
//
// That shape is the derivation, and it is a fact about the route rather than a
// judgement about the operation: everything such a route needs to resolve a
// target is in the path, which is exactly what lets syntheticOperandRequest
// stand in for a real one. A route ending at /{id} is excluded because its
// decoder reads a per-operation body instead.
func operandShapedRoutes() map[string]agentPolicy {
	family := map[string]agentPolicy{}
	for route, pol := range agentPolicies {
		method, template, _ := strings.Cut(route, " ")
		if pol.Access != accessTool || !mutatingMethod(method) {
			continue
		}
		const routedID = "/{id}"
		if at := strings.Index(template, routedID); at >= 0 && at+len(routedID) < len(template) {
			family[route] = pol
		}
	}
	return family
}

// operandWalkDeps answers for each field of restCommandDeps.
//
// Populated across the struct rather than only where the walk's first routes
// happened to reach: a resolver handed a nil dep panics on the nil interface
// instead of reporting a wiring defect, and which deps a decoder reaches is
// what this walk must not have to know in advance. A route the contract adds
// tomorrow gets a working set with nobody remembering to widen this.
func operandWalkDeps() restCommandDeps {
	return restCommandDeps{
		records:  seamRecord{},
		channels: channelKinds{},
		imports:  bothDoorsImports{},
		tags:     bothDoorsTags{},
		stages:   operandWalkStages{},
	}
}

// operandWalkStages answers a stage's semantic for advanceDeal, which is
// operand-shaped and reads one to write its summary. "open" is the routine
// step: the walk is about which row a call binds to, and a close or a reopen
// would put the summary's own wording under test here instead.
type operandWalkStages struct{}

func (operandWalkStages) StageSemantic(context.Context, ids.UUID) (string, ids.UUID, error) {
	return "open", ids.Nil, nil
}

// bodyTargetID reads the survivor a merge's fixture body names.
//
// Read out of the fixture the decoder was handed, rather than restated as a
// constant beside the assertion: the body sent and the id expected back come
// from one value, so a fixture edited for some other reason moves both.
func bodyTargetID(t *testing.T, route, op, body string) ids.UUID {
	t.Helper()
	var named struct {
		TargetID ids.UUID `json:"target_id"`
	}
	if err := json.Unmarshal([]byte(body), &named); err != nil {
		t.Fatalf("%s (%s): reading target_id out of its own fixture body: %v", route, op, err)
	}
	if named.TargetID == ids.Nil {
		t.Fatalf("%s (%s): its fixture body names no target_id, so the assertion below would compare "+
			"against the zero uuid and pass for a decoder that staged nothing", route, op)
	}
	return named.TargetID
}

// assertTheFamilyIsTheContractsOwn holds the walk's corpus to a second,
// independent reading of the same question — and fails in BOTH directions.
//
// A count guard ("did this walk check anything") is not enough, and measuring
// it is what put this here: reinstating the tier prefilter that used to narrow
// this walk drops it from 37 routes to 2, and every assertion below still
// passes. Under-recognition is the one way a gate must not break, because it
// reads a smaller surface, reports success, and leaves no failing assertion to
// notice. A literal 37 would fail the wrong way instead — red on the next route
// the contract grows, with a message about arithmetic.
//
// So the expectation is derived somewhere else: crm.yaml, which is what
// agentPolicies is generated FROM. The generated table and the contract it came
// from are two readings of one fact, so a filter that quietly narrows the walk
// disagrees with the contract and is reported by operationId. A route the
// contract adds appears on both sides at once and says nothing.
func assertTheFamilyIsTheContractsOwn(t *testing.T, family map[string]agentPolicy) {
	t.Helper()
	walked := map[string]bool{}
	for _, pol := range family {
		walked[pol.Op] = true
	}
	declared := operandShapedOperationsInTheContract(t)
	for op := range declared {
		if !walked[op] {
			t.Errorf("%s is an operand-shaped tool-accessible mutating operation in crm.yaml and this walk "+
				"does not reach it — its decoder is never invoked, so every mis-wiring of it passes here", op)
		}
	}
	for op := range walked {
		if !declared[op] {
			t.Errorf("%s is walked here and crm.yaml declares no operand-shaped tool-accessible mutating "+
				"operation by that name — the generated table and the contract disagree", op)
		}
	}
}

// operandShapedOperationsInTheContract reads the family straight out of
// crm.yaml: a mutating operation carrying an `x-mcp-tool` tier (which is what
// puts an operation on the tool surface at all) on a path that names a record
// with {id} and carries more after it.
func operandShapedOperationsInTheContract(t *testing.T) map[string]bool {
	t.Helper()
	const routedID = "/{id}"
	shaped := map[string]bool{}
	for path, item := range loadContract(t).Paths.Map() {
		at := strings.Index(path, routedID)
		if at < 0 || at+len(routedID) >= len(path) {
			continue
		}
		for method, op := range item.Operations() {
			if _, onTheToolSurface := op.Extensions["x-mcp-tool"]; !onTheToolSurface || !mutatingMethod(method) {
				continue
			}
			shaped[op.OperationID] = true
		}
	}
	if len(shaped) == 0 {
		t.Fatal("crm.yaml declares no operand-shaped tool-accessible mutating operation — the comparison " +
			"above would then demand nothing of the walk")
	}
	return shaped
}

// loadContract reads crm.yaml as an OpenAPI document, for the gates in this
// package that derive an expectation from the contract rather than from the
// table generated out of it.
func loadContract(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}
	return doc
}
