// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type keptCalls struct{ records []CallRecord }

func (k *keptCalls) Record(_ context.Context, record CallRecord) {
	k.records = append(k.records, record)
}

// loggedDoor serves /mcp over a registry holding one working tool, as an agent
// with its own passport and connection, and keeps what the door records.
func loggedDoor(t *testing.T, answer string, authenticate func(*http.Request) (context.Context, error)) (http.Handler, *keptCalls) {
	t.Helper()
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(echoTool{spec: objectSpec("read_record", principal.ScopeRead), out: json.RawMessage(answer)})
	kept := &keptCalls{}
	door := NewHTTPHandler(registry, authenticate, func(*http.Request) string { return "Bearer" },
		"margince-crm", "test", discardLog(), WithCallLog(kept))
	return door, kept
}

func asConnectedAgent(passport, connection, correlation ids.UUID) func(*http.Request) (context.Context, error) {
	return func(*http.Request) (context.Context, error) {
		ctx := principal.WithCorrelationID(scopedAgentCtx(principal.ScopeRead), correlation)
		actor, _ := principal.Actor(ctx)
		actor.PassportID, actor.ConnectionID = passport, connection
		return principal.WithActor(ctx, actor), nil
	}
}

// post sends one request through a real server: the door extends its write
// deadline, which a recorder cannot.
func post(t *testing.T, door http.Handler, body string) int {
	t.Helper()
	srv := httptest.NewServer(door)
	defer srv.Close()
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Errorf("closing the answer: %v", err)
	}
	return resp.StatusCode
}

func TestEachAnsweredCallIsRecordedWithItsCallerToolAndOutcome(t *testing.T) {
	passport, connection, correlation := ids.NewV7(), ids.NewV7(), ids.NewV7()
	door, kept := loggedDoor(t, `{"ok":true}`, asConnectedAgent(passport, connection, correlation))
	post(t, door, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_record","arguments":{"action":"look","id":"r1"}}}`)
	post(t, door, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"no_such_tool","arguments":{}}}`)
	post(t, door, `{"jsonrpc":"2.0","id":3,"method":"no/such/method"}`)

	if len(kept.records) != 3 {
		t.Fatalf("recorded %d exchanges, want 3: %+v", len(kept.records), kept.records)
	}
	worked, unknown, unserved := kept.records[0], kept.records[1], kept.records[2]
	if worked.Tool != "read_record" || worked.Action != "look" || worked.Outcome != OutcomeOK ||
		worked.HTTPStatus != http.StatusOK || !strings.Contains(worked.Answer, `"ok"`) ||
		!strings.Contains(string(worked.Arguments), `"id":"r1"`) {
		t.Errorf("the working call was recorded as %+v", worked)
	}
	if worked.CorrelationID != correlation.String() || worked.Caller.PassportID != passport.String() ||
		worked.Caller.ConnectionID != connection.String() || worked.Caller.Type != string(principal.PrincipalAgent) ||
		worked.Caller.UserID == "" {
		t.Errorf("the call's caller and trace were recorded as %+v", worked)
	}
	if unknown.Outcome != OutcomeToolError || !strings.Contains(unknown.Answer, "no_such_tool") {
		t.Errorf("a call to a tool nobody serves was recorded as %+v, want a tool error quoting it", unknown)
	}
	if unserved.Outcome != OutcomeRPCError || unserved.ErrorCode != codeMethodNotFound || unserved.Tool != "" {
		t.Errorf("a method nobody serves was recorded as %+v, want its JSON-RPC error code", unserved)
	}
}

func TestADoorRefusalIsRecordedWithItsStatusAndNoCaller(t *testing.T) {
	for name, tc := range map[string]struct {
		err    error
		status int
	}{
		"no credential":    {errors.New("no token"), http.StatusUnauthorized},
		"no verdict on it": {ErrAuthUnavailable, http.StatusServiceUnavailable},
	} {
		door, kept := loggedDoor(t, `{}`, func(*http.Request) (context.Context, error) { return nil, tc.err })
		if got := post(t, door, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); got != tc.status {
			t.Fatalf("%s: answered %d, want %d", name, got, tc.status)
		}
		if len(kept.records) != 1 || kept.records[0].Outcome != OutcomeRefused ||
			kept.records[0].HTTPStatus != tc.status || kept.records[0].Caller != (CallCaller{}) {
			t.Errorf("%s: recorded %+v, want one refusal with status %d and no caller", name, kept.records, tc.status)
		}
	}
}

func TestALongAnswerIsCutAndSaysSo(t *testing.T) {
	long, err := json.Marshal(map[string]string{"text": strings.Repeat("x", CallAnswerLimit)})
	if err != nil {
		t.Fatal(err)
	}
	door, kept := loggedDoor(t, string(long), asConnectedAgent(ids.NewV7(), ids.NewV7(), ids.NewV7()))
	post(t, door, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_record","arguments":{}}}`)
	if len(kept.records) != 1 {
		t.Fatalf("recorded %d exchanges, want 1", len(kept.records))
	}
	if got := kept.records[0]; len(got.Answer) != CallAnswerLimit || !got.AnswerCut {
		t.Errorf("a long answer was kept at %d bytes, cut %v; want %d and cut", len(got.Answer), got.AnswerCut, CallAnswerLimit)
	}
}

func TestADoorWithNoCallLogRecordsNothingAndStillAnswers(t *testing.T) {
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(echoTool{spec: objectSpec("read_record", principal.ScopeRead), out: json.RawMessage(`{"ok":true}`)})
	door := NewHTTPHandler(registry, asConnectedAgent(ids.NewV7(), ids.NewV7(), ids.NewV7()),
		func(*http.Request) string { return "Bearer" }, "margince-crm", "test", discardLog())
	if got := post(t, door, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_record","arguments":{}}}`); got != http.StatusOK {
		t.Errorf("a door with no call log answered %d, want 200", got)
	}
}

// paced is a refusal that writes its own answer, as the door's rate limit does.
type paced struct{}

func (paced) Error() string { return "too many calls" }
func (paced) WriteRefusal(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusTooManyRequests)
}

func TestARefusalThatWritesItsOwnAnswerIsRecordedWithItsStatus(t *testing.T) {
	door, kept := loggedDoor(t, `{}`, func(*http.Request) (context.Context, error) { return nil, paced{} })
	if got := post(t, door, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); got != http.StatusTooManyRequests {
		t.Fatalf("answered %d, want 429", got)
	}
	if len(kept.records) != 1 || kept.records[0].HTTPStatus != http.StatusTooManyRequests || kept.records[0].Outcome != OutcomeRefused {
		t.Errorf("recorded %+v, want one refusal with status 429", kept.records)
	}
}

func TestARefusalPastAuthenticationKeepsItsCaller(t *testing.T) {
	passport := ids.NewV7()
	authenticated := asConnectedAgent(passport, ids.NewV7(), ids.NewV7())
	door, kept := loggedDoor(t, `{}`, func(r *http.Request) (context.Context, error) {
		ctx, err := authenticated(r)
		if err != nil {
			return nil, err
		}
		return ctx, paced{}
	})
	if got := post(t, door, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`); got != http.StatusTooManyRequests {
		t.Fatalf("answered %d, want 429", got)
	}
	if len(kept.records) != 1 || kept.records[0].Caller.PassportID != passport.String() {
		t.Errorf("recorded %+v, want the paced refusal with its passport", kept.records)
	}
}

func TestARequestRefusedBeforeDispatchIsRecordedOnce(t *testing.T) {
	door, kept := loggedDoor(t, `{}`, asConnectedAgent(ids.NewV7(), ids.NewV7(), ids.NewV7()))
	srv := httptest.NewServer(door)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodPost, srv.URL,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_record","arguments":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Add(headerProtocolVersion, "2025-06-18")
	req.Header.Add(headerProtocolVersion, "2025-03-26")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Error(err)
	}
	post(t, door, `{"jsonrpc":"2.0","id":2,`)
	if len(kept.records) != 2 {
		t.Fatalf("recorded %d exchanges, want one per refused request: %+v", len(kept.records), kept.records)
	}
	twice, unreadable := kept.records[0], kept.records[1]
	if twice.Outcome != OutcomeRPCError || twice.ErrorCode != codeHeaderMismatch || twice.Tool != "read_record" ||
		twice.HTTPStatus != http.StatusBadRequest {
		t.Errorf("a version header sent twice was recorded as %+v", twice)
	}
	if unreadable.Outcome != OutcomeRPCError || unreadable.ErrorCode != codeParseError {
		t.Errorf("a body that does not decode was recorded as %+v", unreadable)
	}
}

func TestAToolAnswerTheLogCannotReadIsRecordedAsUnreadable(t *testing.T) {
	door := &httpMCPHandler{server: &Dispatcher{log: discardLog()}}
	kept := &keptCalls{}
	door.server.callLog = kept
	call := rpcRequest{Method: methodToolsCall, Params: json.RawMessage(`{"name":"read_record","arguments":{}}`)}
	for name, result := range map[string]any{
		"cannot be encoded":   make(chan int),
		"content is no array": map[string]any{"content": "a string"},
	} {
		door.recordExchange(context.Background(), time.Now(), call, rpcResponse{Result: result}, http.StatusOK)
		if got := kept.records[len(kept.records)-1]; got.Outcome != OutcomeUnreadable || got.Answer != "" {
			t.Errorf("%s: recorded %+v, want an unreadable call with no answer", name, got)
		}
	}
}

func TestABodyTheDoorWillNotReadIsRecordedAsARefusal(t *testing.T) {
	door, kept := loggedDoor(t, `{}`, asConnectedAgent(ids.NewV7(), ids.NewV7(), ids.NewV7()))
	if got := post(t, door, strings.Repeat(" ", MaxMCPRequestBytes+1)); got != http.StatusRequestEntityTooLarge {
		t.Fatalf("answered %d, want 413", got)
	}
	if len(kept.records) != 1 || kept.records[0].Outcome != OutcomeRefused ||
		kept.records[0].HTTPStatus != http.StatusRequestEntityTooLarge {
		t.Errorf("recorded %+v, want one refusal with status 413", kept.records)
	}
}
