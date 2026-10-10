// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Registry-served routes: contract operations whose answer is an agent tool's.
// Each handler maps the request onto the tool's arguments and calls the same
// Registry.Invoke the MCP door calls. So a passport gets one admission, one
// charge and one answer on either door. The operation's x-mcp-tool says
// `served_by: registry`, which makes agentGate leave admission to Invoke.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// ListSlippingDeals serves whats_slipping_this_week.
func (s Server) ListSlippingDeals(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSlippingDealsParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "whats_slipping_this_week", params)
}

// ListAtRiskRelationships serves at_risk_relationships.
func (s Server) ListAtRiskRelationships(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "at_risk_relationships", noArguments{})
}

// CatchUpOnRecord serves catch_me_up_on.
func (s Server) CatchUpOnRecord(w http.ResponseWriter, r *http.Request, params crmcontracts.CatchUpOnRecordParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "catch_me_up_on", params)
}

// PrepForMeeting serves prep_for_meeting.
func (s Server) PrepForMeeting(w http.ResponseWriter, r *http.Request, params crmcontracts.PrepForMeetingParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "prep_for_meeting", params)
}

// PrepareProjectHandoff serves prepare_handoff.
func (s Server) PrepareProjectHandoff(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	serveTool(w, r, s.toolRegistry.Invoke, "prepare_handoff", struct {
		ProjectID crmcontracts.Id `json:"project_id"`
	}{id})
}

// ListOpenCommitments serves review_commitments.
func (s Server) ListOpenCommitments(w http.ResponseWriter, r *http.Request, params crmcontracts.ListOpenCommitmentsParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "review_commitments", params)
}

// ListIntroPaths serves intro_path_to.
func (s Server) ListIntroPaths(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	serveTool(w, r, s.toolRegistry.Invoke, "intro_path_to", struct {
		CompanyID crmcontracts.Id `json:"company_id"`
	}{id})
}

// SearchContext serves search_context.
func (s Server) SearchContext(w http.ResponseWriter, r *http.Request, params crmcontracts.SearchContextParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "search_context", params)
}

// SearchReportEvidence serves search_report_evidence, over the run the path names.
func (s Server) SearchReportEvidence(w http.ResponseWriter, r *http.Request, runID openapi_types.UUID) {
	args, ok := toolArgsWithPath(w, r, pathArgument{"run_id", runID.String()})
	if !ok {
		return
	}
	serveTool(w, r, s.toolRegistry.Invoke, "search_report_evidence", args)
}

// DraftDealFollowUps serves draft_follow_ups_for.
func (s Server) DraftDealFollowUps(w http.ResponseWriter, r *http.Request, params crmcontracts.DraftDealFollowUpsParams) {
	serveToolCommand(w, r, s.toolRegistry.Invoke, "draft_follow_ups_for", reservedArgs{params.IdempotencyKey, params.XApprovalToken})
}

// ProgressDeal serves progress_deal, on the deal the path names.
func (s Server) ProgressDeal(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ProgressDealParams) {
	serveToolCommand(w, r, s.toolRegistry.Invoke, "progress_deal", reservedArgs{params.IdempotencyKey, params.XApprovalToken},
		pathArgument{dealIDField, id.String()})
}

// QualifyLead serves qualify_lead, on the lead the path names.
func (s Server) QualifyLead(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.QualifyLeadParams) {
	serveToolCommand(w, r, s.toolRegistry.Invoke, "qualify_lead", reservedArgs{params.IdempotencyKey, params.XApprovalToken},
		pathArgument{"lead_id", id.String()})
}

// QueryWorkspace serves query_workspace.
func (s Server) QueryWorkspace(w http.ResponseWriter, r *http.Request) {
	serveToolBody(w, r, s.toolRegistry.Invoke, "query_workspace")
}

// ResolveEntities serves resolve_entities.
func (s Server) ResolveEntities(w http.ResponseWriter, r *http.Request) {
	serveToolBody(w, r, s.toolRegistry.Invoke, "resolve_entities")
}

// DescribeQueryVocabulary serves describe_query_vocabulary.
func (s Server) DescribeQueryVocabulary(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "describe_query_vocabulary", noArguments{})
}

// DescribeAnalyticsVocabulary serves describe_analytics_vocabulary.
func (s Server) DescribeAnalyticsVocabulary(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "describe_analytics_vocabulary", noArguments{})
}

// DescribeRecordFields serves describe_record_fields.
func (s Server) DescribeRecordFields(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "describe_record_fields", noArguments{})
}

// DescribeReportBlocks serves describe_report_blocks.
func (s Server) DescribeReportBlocks(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "describe_report_blocks", noArguments{})
}

// DescribeReportVocabulary serves describe_report_vocabulary.
func (s Server) DescribeReportVocabulary(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "describe_report_vocabulary", noArguments{})
}

// GetActingIdentity serves whoami.
func (s Server) GetActingIdentity(w http.ResponseWriter, r *http.Request) {
	serveTool(w, r, s.toolRegistry.Invoke, "whoami", noArguments{})
}

// noArguments is the argument object of a tool that takes none.
type noArguments struct{}

// pathArgument is a path parameter the tool takes as one of its arguments.
type pathArgument struct{ name, value string }

// toolArgsWithPath reads the body as the tool's argument object and sets each
// path parameter over it. So the record the path names is the one the tool
// acts on, whatever the body says. An empty body is an empty object. On a
// refusal it writes the answer and reports false.
func toolArgsWithPath(w http.ResponseWriter, r *http.Request, path ...pathArgument) (map[string]json.RawMessage, bool) {
	var args map[string]json.RawMessage
	if err := httperr.DecodeOrRefusal(w, r, &args); err != nil && !errors.Is(err, io.EOF) {
		httperr.Write(w, r, err)
		return nil, false
	}
	if args == nil {
		args = map[string]json.RawMessage{}
	}
	for _, p := range path {
		args[p.name] = json.RawMessage(strconv.Quote(p.value))
	}
	return args, true
}

// reservedArgs are the two arguments every write tool reserves, its retry key
// and the approval a retry redeems, as the route's declared headers carry them.
// They come from the generated Params, so a header the route does not declare
// never reaches the tool.
type reservedArgs struct{ idempotencyKey, approvalID *string }

// serveToolCommand runs a write tool for a REST request. The route is not in
// replayableOperations, so the Idempotency-Key header becomes the tool's own
// idempotency_key, and an X-Approval-Token its approval_id. A REST retry is then
// claimed and redeemed once, by Invoke, as the same call over MCP would be.
func serveToolCommand(w http.ResponseWriter, r *http.Request, invoke toolInvoker, tool string, reserved reservedArgs, path ...pathArgument) {
	args, ok := toolArgsWithPath(w, r, path...)
	if !ok {
		return
	}
	for _, fold := range []struct {
		header, arg string
		value       *string
	}{
		{idempotencyKeyHeader, "idempotency_key", reserved.idempotencyKey},
		{approvalTokenHeader, "approval_id", reserved.approvalID},
	} {
		if err := foldHeader(args, fold.header, fold.value, fold.arg); err != nil {
			httperr.Write(w, r, err)
			return
		}
	}
	serveTool(w, r, invoke, tool, args)
}

// foldHeader sets arg to the header's value. A body that already names a
// different value is refused, since the call would claim two keys or redeem two
// approvals.
func foldHeader(args map[string]json.RawMessage, header string, sent *string, arg string) error {
	if sent == nil || *sent == "" {
		return nil
	}
	value := *sent
	if inArgs, named := args[arg]; named {
		var inBody string
		if json.Unmarshal(inArgs, &inBody) != nil || inBody != value {
			return httperr.Validation(header, "conflicts_with_body",
				"the "+header+" header and the body's "+arg+" disagree; send one of them")
		}
	}
	args[arg] = json.RawMessage(strconv.Quote(value))
	return nil
}

// serveToolBody runs a tool whose argument object is the request body itself.
// The body passes through undecoded. So the tool refuses a member it does not
// take, as over MCP, and a number keeps its precision.
func serveToolBody(w http.ResponseWriter, r *http.Request, invoke toolInvoker, tool string) {
	var args json.RawMessage
	if !httperr.Decode(w, r, &args) {
		return
	}
	serveTool(w, r, invoke, tool, args)
}

// serveTool runs one tool for a REST request and writes the tool's payload.
//
// args becomes the tool's argument object as JSON. Name the parameters after
// the tool's arguments and a generated Params struct passes straight through.
// Each optional field is tagged omitempty, so an omitted parameter stays an
// omitted argument and the tool applies its own default.
//
//craft:ignore naked-any json.Marshal's own input: the args are whichever Params or body type the contract generated
func serveTool(w http.ResponseWriter, r *http.Request, invoke toolInvoker, tool string, args any) {
	in, err := json.Marshal(args)
	if err != nil {
		httperr.Write(w, r, fmt.Errorf("compose: encoding the arguments of %s: %w", tool, err))
		return
	}
	sealed, err := invoke(r.Context(), tool, in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeToolPayload(w, r, sealed)
}

// writeToolPayload answers a REST call with the envelope's `data`, the payload
// an operation's 200 schema declares. The trace id moves to a header. The MCP
// door answers the whole envelope, so its `data` and this body are one JSON.
func writeToolPayload(w http.ResponseWriter, r *http.Request, sealed json.RawMessage) {
	payload, trace, err := unwrapToolEnvelope(sealed)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if trace != "" {
		w.Header().Set(toolTraceHeader, trace)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	//craft:ignore swallowed-errors WriteHeader already committed the response — nothing can report a write failure to the client anymore
	_, _ = w.Write(payload)
}
