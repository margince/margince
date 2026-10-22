// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithPayloadCaptureFlag reports the same deployment posture that controls
// model-path capture, distinguishing disabled capture from an older call.
func (h Handlers) WithPayloadCaptureFlag(enabled bool) Handlers {
	h.capturePayloads = enabled
	return h
}

// ListAiCalls implements (GET /ai/calls).
func (h Handlers) ListAiCalls(
	w http.ResponseWriter,
	r *http.Request,
	params crmcontracts.ListAiCallsParams,
) {
	read, err := callListOf(params)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	page, err := h.calls.ListCalls(r.Context(), read.cursor, read.limit, read.filter)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	data := make([]crmcontracts.AiCallSummary, 0, len(page.Items))
	for _, item := range page.Items {
		data = append(data, wireAiCallSummary(item))
	}
	response := crmcontracts.AiCallListResponse{
		Data:                  data,
		Page:                  crmcontracts.PageInfo{HasMore: page.HasMore},
		PayloadCaptureEnabled: h.capturePayloads,
		Tasks:                 page.Tasks,
		TaskOptions:           wireAiCallTaskOptions(page.Tasks),
		Models:                page.Models,
	}
	if page.NextCursor != "" {
		response.Page.NextCursor = &page.NextCursor
	}
	httperr.WriteJSON(w, http.StatusOK, response)
}

// callListRead is one /ai/calls read: the page asked for and what narrows it.
type callListRead struct {
	cursor *string
	limit  *int
	filter CallListFilter
}

// callListOf checks what the generated binding cannot: an outcome off the
// enum and a window that ends before it starts, which would match nothing.
func callListOf(params crmcontracts.ListAiCallsParams) (callListRead, error) {
	filter := CallListFilter{
		Task: deref(params.Task), Provider: deref(params.Provider), Model: deref(params.Model),
		ServedModel: deref(params.ServedModel), ServedProvider: deref(params.ServedProvider), Tier: deref(params.Tier),
		Query: deref(params.Q), From: params.From, To: params.To,
	}
	if params.Outcome != nil {
		if !params.Outcome.Valid() {
			return callListRead{}, httperr.Validation("outcome", "invalid_parameter",
				"outcome must be one of answered, fallback, declined, failed")
		}
		filter.Outcome = string(*params.Outcome)
	}
	if filter.From != nil && filter.To != nil && !filter.To.After(*filter.From) {
		return callListRead{}, httperr.Validation("to", "invalid_parameter", "to must be after from")
	}
	return callListRead{cursor: params.Cursor, limit: params.Limit, filter: filter}, nil
}

// GetAiCall implements (GET /ai/calls/{id}).
func (h Handlers) GetAiCall(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	detail, err := h.calls.GetCall(r.Context(), ids.UUID(id))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireAiCall(detail))
}

func wireAiCallSummary(summary CallSummary) crmcontracts.AiCallSummary {
	return crmcontracts.AiCallSummary{
		Id: openapi_types.UUID(summary.ID), OccurredAt: summary.OccurredAt,
		Task: summary.Task, Kind: summary.Kind, Tier: summary.Tier, Provider: summary.Provider,
		ModelId: summary.ModelID, ServedModel: summary.ServedModel,
		CallsAttempted: summary.Attempt, TokensIn: int(summary.TokensIn),
		TokensOut: int(summary.TokensOut), ReasoningTokens: int(summary.ReasoningTokens),
		CachedTokens: int(summary.CachedTokens), LatencyMs: int(summary.LatencyMS),
		CacheHit: summary.CacheHit, Degraded: summary.Degraded,
		ErrorSentinel: summary.ErrorSentinel, HasPayload: summary.HasPayload,
		DecisionAttempted:     summary.DecisionAttempted,
		TaskDisplayName:       taskDisplayName(Task(summary.Task)),
		Outcome:               crmcontracts.AiCallOutcome(summary.Outcome),
		EstimatedCostMicrousd: summary.CostMicroUSD,
	}
}

func wireAiCallTaskOptions(tasks []string) []crmcontracts.AiCallTaskOption {
	options := make([]crmcontracts.AiCallTaskOption, 0, len(tasks))
	for _, task := range tasks {
		options = append(options, crmcontracts.AiCallTaskOption{Task: task, DisplayName: taskDisplayName(Task(task))})
	}
	return options
}

func wireAiCall(detail CallDetail) crmcontracts.AiCall {
	summary := wireAiCallSummary(detail.CallSummary)
	out := crmcontracts.AiCall{
		Id: summary.Id, OccurredAt: summary.OccurredAt, Task: summary.Task, TaskDisplayName: summary.TaskDisplayName,
		Kind: summary.Kind, Tier: summary.Tier, Provider: summary.Provider, ModelId: summary.ModelId,
		ServedModel: summary.ServedModel, CallsAttempted: summary.CallsAttempted,
		TokensIn: summary.TokensIn, TokensOut: summary.TokensOut,
		ReasoningTokens: summary.ReasoningTokens, CachedTokens: summary.CachedTokens,
		LatencyMs: summary.LatencyMs, CacheHit: summary.CacheHit,
		Degraded: summary.Degraded, ErrorSentinel: summary.ErrorSentinel,
		HasPayload: summary.HasPayload, ServedIdentitySource: detail.ServedIdentitySource,
		ConfigHash: detail.ConfigHash, Config: detail.Config,
		ContextScopes:         detail.ContextScopes,
		ContextFingerprint:    detail.ContextFingerprint,
		LogicalCallId:         openapi_types.UUID(detail.LogicalCallID),
		Attempts:              make([]crmcontracts.AiCallAttempt, 0, len(detail.Attempts)),
		PayloadCaptured:       detail.Payload != nil,
		DecisionAttempted:     summary.DecisionAttempted,
		Outcome:               summary.Outcome,
		EstimatedCostMicrousd: summary.EstimatedCostMicrousd,
	}
	if detail.CorrelationID != nil {
		value := openapi_types.UUID(*detail.CorrelationID)
		out.CorrelationId = &value
	}
	if detail.AgentRunID != nil {
		value := openapi_types.UUID(*detail.AgentRunID)
		out.AgentRunId = &value
	}
	for _, attempt := range detail.Attempts {
		out.Attempts = append(out.Attempts, wireAiCallAttempt(attempt))
	}
	if detail.Payload != nil {
		out.Payload = &struct {
			Request  any `json:"request"`
			Response any `json:"response"`
		}{Request: detail.Payload.Request, Response: detail.Payload.Response}
	}
	return out
}

func wireAiCallAttempt(attempt CallAttempt) crmcontracts.AiCallAttempt {
	out := crmcontracts.AiCallAttempt{
		Attempt: attempt.Attempt, IsTerminal: attempt.IsTerminal, Kind: attempt.Kind,
		Tier: optionalText(attempt.Tier), Provider: optionalText(attempt.Provider),
		ModelId:       optionalText(attempt.ModelID),
		AttemptReason: attempt.AttemptReason, ErrorSentinel: attempt.ErrorSentinel,
		ServedModel:    optionalText(attempt.ServedModel),
		ServedProvider: optionalText(attempt.ServedProvider),
		TokensIn:       int(attempt.TokensIn), TokensOut: int(attempt.TokensOut),
		LatencyMs: int(attempt.LatencyMS), OccurredAt: attempt.OccurredAt,
	}
	if answer := attempt.DecisionAnswer; answer != nil {
		choice, confidence := answer.Choice, answer.Confidence
		out.DecisionChoice, out.DecisionConfidence = &choice, &confidence
	}
	return out
}

// optionalText is an optional wire field from a column that stores "" for
// "none": a failed walk that never reached a rung names no tier, and the wire
// says so by omitting it rather than by sending an empty string.
func optionalText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// defaultStatsWindow is the window a figures read takes when it names none.
const defaultStatsWindow = "7d"

// GetAiCallStats implements (GET /ai/call-stats).
func (h Handlers) GetAiCallStats(w http.ResponseWriter, r *http.Request, params crmcontracts.GetAiCallStatsParams) {
	window, span, err := statsWindow((*string)(params.Window))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	group := string(GroupByProvider)
	if params.Group != nil {
		group = string(*params.Group)
	}
	rows, err := h.calls.CallStats(r.Context(), CallStatsQuery{
		Window: span, GroupBy: CallStatsGroup(group),
		Filter: CallStatsFilter{Provider: deref(params.Provider), Model: deref(params.Model), Tier: deref(params.Tier), Task: Task(deref(params.Task))},
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.AiCallStats{Window: window, Group: group, Rows: make([]crmcontracts.AiCallStatsRow, 0, len(rows))}
	for _, row := range rows {
		out.Rows = append(out.Rows, crmcontracts.AiCallStatsRow{
			Key: row.Key, Calls: row.Calls, Failed: row.Failed, Timeouts: row.Timeouts, P50Ms: row.P50Ms, P95Ms: row.P95Ms,
			TokensIn: row.TokensIn, TokensOut: row.TokensOut, CostMicrousd: row.CostMicroUSD, Unpriced: row.Unpriced,
			Switched: row.Switched, SwitchedTo: optionalText(row.SwitchedTo),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetAiTaskFlow implements (GET /ai/call-stats/flow).
func (h Handlers) GetAiTaskFlow(w http.ResponseWriter, r *http.Request, params crmcontracts.GetAiTaskFlowParams) {
	if _, known := taskLadders[Task(params.Task)]; !known {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return
	}
	window, span, err := statsWindow((*string)(params.Window))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	flow, err := h.calls.TaskFlow(r.Context(), Task(params.Task), span)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.AiTaskFlow{Task: params.Task, Window: window, Total: flow.Total, Unanswered: flow.Unanswered, Steps: make([]crmcontracts.AiFlowStep, 0, len(flow.Steps))}
	for _, step := range flow.Steps {
		out.Steps = append(out.Steps, crmcontracts.AiFlowStep{
			Decision: step.Decision, Tier: string(step.Tier), Provider: step.Provider, Model: step.Model,
			Attempts: step.Attempts, Answered: step.Answered, P50Ms: step.P50Ms, GaveUp: step.GaveUp,
			Switched: step.Switched, SwitchedTo: optionalText(step.SwitchedTo),
		})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// statsWindow is the named window and its length, refusing a name the
// screens do not offer rather than answering for another window under it.
func statsWindow(named *string) (string, time.Duration, error) {
	if named == nil {
		return defaultStatsWindow, CallStatsWindows[defaultStatsWindow], nil
	}
	span, ok := CallStatsWindows[*named]
	if !ok {
		return "", 0, invalidAt("window", "must be one of 24h, 7d, 30d")
	}
	return *named, span, nil
}
