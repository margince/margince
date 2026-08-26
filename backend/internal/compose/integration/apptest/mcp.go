// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

// The MCP transport as a suite drives it: initialize, list, call.
//
// It lives here because more than one suite now speaks the protocol. The
// agentaccess package owns the DISCOVERY chain — the 401's pointer, the
// metadata documents, the origin they close on — and keeps its own harness for
// it. What that suite does not own is the plain act of calling a tool, which is
// every scenario suite's first line.
//
// The one property this file exists to preserve: a refused tool answers HTTP
// 200 with isError set, because a refusal travels IN BAND. A helper that fails
// the test on a non-200 cannot tell a call that ran from one that was denied,
// so Call returns both and lets the caller say which it wanted.

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// MCPClient calls tools over /mcp as one bearer credential.
//
// A passport rather than a cookie, always: every /mcp request binds an AGENT
// principal, so there is no such thing as a cookie-authenticated tool call. The
// human half of a scenario is a REST call on AppEnv.Client, and the two are
// deliberately different objects so a test cannot blur them.
type MCPClient struct {
	env    *AppEnv
	bearer string
	nextID int
}

// NewMCPClient binds a client to one credential. It does not handshake — call
// Initialize when the scenario is about the handshake, and skip it when the
// scenario is about a tool.
func NewMCPClient(e *AppEnv, bearer string) *MCPClient {
	return &MCPClient{env: e, bearer: bearer}
}

// MCPResult is one tools/call outcome, with the refusal kept separate from the
// text so neither can be read as the other.
type MCPResult struct {
	// Text is the first content block's text, which is where every tool this
	// repo serves puts its answer.
	Text string
	// IsError is the protocol's in-band refusal flag. A refused call is a 200,
	// so this is the only thing that distinguishes one.
	IsError bool
}

// JSON decodes the tool's own payload into out.
//
// Every tool on this surface answers inside a shared envelope — schema_version,
// trace_id, freshness, trust, evidence, warnings — with its own result nested
// under `data`. The nesting is deliberate: a merged payload would let a tool's
// field name collide with an envelope field, so the envelope's meaning would
// depend on which tool answered.
//
// A scenario asserting on what a tool SAID wants the payload, so this unwraps.
// Envelope reads the whole thing for the rare assertion that is about the
// envelope itself.
func (r MCPResult) JSON(t *testing.T, out any) {
	t.Helper()
	envelope := r.Envelope(t)
	if len(envelope.Data) == 0 {
		t.Fatalf("the tool answered with an envelope carrying no data:\n%s", r.Text)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		t.Fatalf("the tool's payload does not decode: %v\n%s", err, string(envelope.Data))
	}
}

// Envelope decodes the whole envelope, for an assertion about provenance,
// freshness or trust rather than about the answer.
func (r MCPResult) Envelope(t *testing.T) agents.Envelope {
	t.Helper()
	var envelope agents.Envelope
	if err := json.Unmarshal([]byte(r.Text), &envelope); err != nil {
		t.Fatalf("the tool answer is not a result envelope: %v\n%s", err, r.Text)
	}
	return envelope
}

// Initialize performs the protocol handshake and returns the negotiated
// revision.
func (c *MCPClient) Initialize(t *testing.T, protocolVersion string) string {
	t.Helper()
	result := c.rpc(t, "initialize", map[string]any{"protocolVersion": protocolVersion})
	negotiated, _ := result["protocolVersion"].(string)
	if negotiated == "" {
		t.Fatalf("initialize settled no protocolVersion: %v", result)
	}
	return negotiated
}

// ListTools names every tool this credential is served, in the order the server
// returns them.
func (c *MCPClient) ListTools(t *testing.T) []string {
	t.Helper()
	result := c.rpc(t, "tools/list", nil)
	raw, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list carries no tools array: %v", result)
	}
	names := make([]string, 0, len(raw))
	for _, entry := range raw {
		tool, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("tools/list entry is not an object: %v", entry)
		}
		name, ok := tool["name"].(string)
		if !ok || name == "" {
			t.Fatalf("tools/list entry carries no name: %v", tool)
		}
		names = append(names, name)
	}
	return names
}

// Call invokes one tool and returns what came back, refusal included. It is the
// primitive; CallOK and CallRefused are the two things a scenario actually
// means, and saying which one is expected is what makes a test readable.
func (c *MCPClient) Call(t *testing.T, tool string, args map[string]any) MCPResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	result := c.rpc(t, "tools/call", map[string]any{"name": tool, "arguments": args})
	isError, _ := result["isError"].(bool)
	return MCPResult{Text: firstText(t, tool, result), IsError: isError}
}

// CallOK invokes a tool that must run, and fails naming the refusal if it did
// not.
func (c *MCPClient) CallOK(t *testing.T, tool string, args map[string]any) MCPResult {
	t.Helper()
	got := c.Call(t, tool, args)
	if got.IsError {
		t.Fatalf("%s was refused and the scenario needs it to run:\n%s", tool, got.Text)
	}
	return got
}

// CallRefused invokes a tool that must NOT run, and returns the refusal text so
// the caller can assert on what it says.
//
// Asserting on the text is the point rather than a convenience: "already
// exists" and "already exists — Anna Weber holds that address" are different
// products, and only the second one a user can act on.
func (c *MCPClient) CallRefused(t *testing.T, tool string, args map[string]any) string {
	t.Helper()
	got := c.Call(t, tool, args)
	if !got.IsError {
		t.Fatalf("%s ran, and the scenario needs it refused:\n%s", tool, got.Text)
	}
	return got.Text
}

// rpc issues one JSON-RPC request and returns its result member.
//
// Every id is distinct within a client. The protocol does not require it over a
// request-per-exchange transport, but a repeated id makes a captured session
// unreadable when a scenario fails and somebody has to see which call did what.
func (c *MCPClient) rpc(t *testing.T, method string, params map[string]any) map[string]any {
	t.Helper()
	c.nextID++
	envelope := map[string]any{"jsonrpc": "2.0", "id": c.nextID, "method": method}
	if params != nil {
		envelope["params"] = params
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encoding %s request: %v", method, err)
	}

	req, err := http.NewRequest(http.MethodPost, c.env.TS.URL+"/mcp", strings.NewReader(string(payload)))
	if err != nil {
		t.Fatalf("building %s request: %v", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}

	//nolint:bodyclose // closed by CloseBody below; bodyclose only recognises a Close in the same package
	resp, err := c.env.Client.Do(req)
	if err != nil {
		t.Fatalf("%s: %v", method, err)
	}
	defer CloseBody(t, resp)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("%s: reading response: %v", method, err)
	}
	// A transport-level non-200 is an admission failure — a rejected
	// credential, an absent route — and never a refused tool. Naming that
	// difference here keeps it out of every caller.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s → HTTP %d (a refused TOOL answers 200 with isError; this is the transport rejecting the call)\n%s",
			method, resp.StatusCode, string(body))
	}
	return rpcResultOf(t, method, unframe(resp.Header.Get("Content-Type"), string(body)))
}

// unframe strips the single SSE event the transport answers with when the
// client accepts text/event-stream.
//
// This client sends that Accept header on purpose, because a real MCP client
// does: the streamable-HTTP transport picks its framing from Accept, and a
// suite that quietly asked for JSON only would be exercising a frame no
// deployed client receives. The body is one complete event either way — the
// server writes "data: <the whole JSON-RPC response>\n\n" and returns — so
// unframing is a prefix strip rather than a stream reader.
func unframe(contentType, body string) string {
	if !strings.Contains(contentType, "text/event-stream") {
		return body
	}
	var payload strings.Builder
	for _, line := range strings.Split(body, "\n") {
		if data, found := strings.CutPrefix(line, "data: "); found {
			payload.WriteString(data)
		}
	}
	return payload.String()
}

// rpcResultOf unwraps the JSON-RPC envelope, failing on a protocol-level error.
//
//craft:ignore naked-any a JSON-RPC result is an open object by the protocol — asserting on one means reading it untyped
func rpcResultOf(t *testing.T, method, body string) map[string]any {
	t.Helper()
	var envelope struct {
		JSONRPC string         `json:"jsonrpc"`
		Result  map[string]any `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("%s response does not decode: %v\n%s", method, err, body)
	}
	if envelope.JSONRPC != "2.0" {
		t.Fatalf("%s: jsonrpc = %q, want \"2.0\"\n%s", method, envelope.JSONRPC, body)
	}
	if envelope.Error != nil {
		t.Fatalf("%s: JSON-RPC error %d: %s", method, envelope.Error.Code, envelope.Error.Message)
	}
	if envelope.Result == nil {
		t.Fatalf("%s: response carries neither a result nor an error\n%s", method, body)
	}
	return envelope.Result
}

// firstText reads the text out of a tools/call result.
//
// A refused call carries its reason in the SAME place a successful one carries
// its answer, so this reads both and never judges which it got.
//
//craft:ignore naked-any a tools/call result is an open object by the protocol — asserting on one means reading it untyped
func firstText(t *testing.T, tool string, result map[string]any) string {
	t.Helper()
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("%s answered with no content: %v", tool, result)
	}
	block, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("%s content[0] is not an object: %v", tool, content[0])
	}
	text, ok := block["text"].(string)
	if !ok {
		t.Fatalf("%s content[0] carries no text: %v", tool, block)
	}
	return text
}

// MCPBearerToken mints a passport and returns the raw token, for a caller that
// wants an MCPClient rather than a REST header map.
//
// PassportBearer answers the REST shape ("Authorization: Bearer …" as a header
// map) because that is what AppEnv.Call takes. The MCP client wants the token
// itself, and peeling the prefix off at every call site is how a test ends up
// sending "Bearer Bearer …".
func MCPBearerToken(t *testing.T, e *AppEnv, label string, scopes ...string) string {
	t.Helper()
	header := PassportBearer(t, e, label, scopes...)
	token, found := strings.CutPrefix(header["Authorization"], "Bearer ")
	if !found {
		t.Fatalf("passport %q did not mint a bearer header: %v", label, header)
	}
	return token
}
