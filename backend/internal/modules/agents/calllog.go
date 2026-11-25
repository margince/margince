// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// CallLog receives one record for each exchange the HTTP door answers. The
// composition root decides where it goes; a nil one keeps nothing.
type CallLog interface {
	Record(ctx context.Context, record CallRecord)
}

// WithCallLog hands every answered exchange to log, refusals at the door
// included.
func WithCallLog(log CallLog) HTTPOption {
	return func(d *Dispatcher) { d.callLog = log }
}

// CallAnswerLimit bounds the answer one record keeps, in bytes, so a long read
// cannot grow the log without limit.
const CallAnswerLimit = 16 << 10

// The outcomes a record names. A tool error is a call the server ran and
// refused, which is how a confirm-first call answers too.
const (
	OutcomeOK        = "ok"
	OutcomeToolError = "tool_error"
	OutcomeRPCError  = "rpc_error"
	OutcomeRefused   = "refused"
)

// CallRecord is one exchange as the server saw it.
type CallRecord struct {
	At            time.Time       `json:"at"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Caller        CallCaller      `json:"caller"`
	Method        string          `json:"method,omitempty"`
	Tool          string          `json:"tool,omitempty"`
	Action        string          `json:"action,omitempty"`
	Arguments     json.RawMessage `json:"arguments,omitempty"`
	HTTPStatus    int             `json:"http_status"`
	Outcome       string          `json:"outcome"`
	// ErrorCode is the JSON-RPC error code of an rpc_error.
	ErrorCode int   `json:"error_code,omitempty"`
	LatencyMS int64 `json:"latency_ms"`
	// Answer is a tool call's text, cut to CallAnswerLimit; AnswerCut says so.
	Answer    string `json:"answer,omitempty"`
	AnswerCut bool   `json:"answer_cut,omitempty"`
}

// CallCaller names who made the call. It is empty for a request refused before
// its credential was read.
type CallCaller struct {
	Type         string `json:"type,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	PassportID   string `json:"passport_id,omitempty"`
	ConnectionID string `json:"connection_id,omitempty"`
	Seat         string `json:"seat,omitempty"`
}

func callerOf(ctx context.Context) CallCaller {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return CallCaller{}
	}
	user := actor.UserID
	if !actor.OnBehalfOf.IsZero() {
		user = actor.OnBehalfOf
	}
	caller := CallCaller{Type: string(actor.Type), Seat: string(actor.SeatType)}
	if !user.IsZero() {
		caller.UserID = user.String()
	}
	if !actor.PassportID.IsZero() {
		caller.PassportID = actor.PassportID.String()
	}
	if !actor.ConnectionID.IsZero() {
		caller.ConnectionID = actor.ConnectionID.String()
	}
	return caller
}

func newCallRecord(ctx context.Context, start time.Time, status int) CallRecord {
	record := CallRecord{
		At: start.UTC(), Caller: callerOf(ctx), HTTPStatus: status,
		LatencyMS: time.Since(start).Milliseconds(),
	}
	if id, ok := principal.CorrelationID(ctx); ok {
		record.CorrelationID = id.String()
	}
	return record
}

// recordRefusal logs a request the door answered before dispatching it.
func (h *httpMCPHandler) recordRefusal(r *http.Request, start time.Time, status int) {
	if h.server.callLog == nil {
		return
	}
	record := newCallRecord(r.Context(), start, status)
	record.Outcome = OutcomeRefused
	h.server.callLog.Record(r.Context(), record)
}

// answer records one answered request, then writes it. The record comes first,
// so a client that holds the answer can find its line in the log.
func (h *httpMCPHandler) answer(w http.ResponseWriter, r *http.Request, start time.Time, req rpcRequest, resp rpcResponse, status int) {
	h.recordExchange(r.Context(), start, req, resp, status)
	writeRPCResponse(w, r, resp, status)
}

// recordExchange logs one answered request and its answer.
func (h *httpMCPHandler) recordExchange(ctx context.Context, start time.Time, req rpcRequest, resp rpcResponse, status int) {
	if h.server.callLog == nil {
		return
	}
	record := newCallRecord(ctx, start, status)
	record.Method = req.Method
	record.Outcome = OutcomeOK
	if resp.Error != nil {
		record.Outcome, record.ErrorCode = OutcomeRPCError, resp.Error.Code
	}
	if req.Method == methodToolsCall {
		record.Tool, record.Action, record.Arguments = calledTool(req.Params)
		answer, isError := toolAnswer(resp)
		if isError && resp.Error == nil {
			record.Outcome = OutcomeToolError
		}
		record.Answer = textcut.Bytes(answer, CallAnswerLimit)
		record.AnswerCut = len(record.Answer) < len(answer)
	}
	h.server.callLog.Record(ctx, record)
}

// calledTool reads a tools/call's tool, its router action and its arguments.
func calledTool(params json.RawMessage) (tool, action string, arguments json.RawMessage) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return "", "", nil
	}
	var routed struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(call.Arguments, &routed); err != nil {
		return call.Name, "", call.Arguments
	}
	return call.Name, routed.Action, call.Arguments
}

// toolAnswer reads a tool result's text parts and its error flag through the
// result's wire shape, the one a client reads.
func toolAnswer(resp rpcResponse) (text string, isError bool) {
	body, err := json.Marshal(resp.Result)
	if err != nil {
		return "", false
	}
	var wire struct {
		//nolint:tagliatelle // isError is the MCP result's wire member, camelCase by the specification
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		return "", false
	}
	parts := make([]string, 0, len(wire.Content))
	for _, part := range wire.Content {
		if part.Text != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n"), wire.IsError
}

// statusWriter keeps the status a refusal was answered with.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}
