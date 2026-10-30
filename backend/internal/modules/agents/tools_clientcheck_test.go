// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func checkClientDispatcher(offered bool) *Dispatcher {
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}), WithToolGate(func(context.Context) bool { return offered }))
	registry.Register(checkClient{log: slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))})
	return NewDispatcher(registry, bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
}

// callCheck sends one tools/call the way a client does: era and capabilities in
// `_meta`, then the framing the dispatcher would derive from them.
func callCheck(t *testing.T, d *Dispatcher, meta, extra string) map[string]any {
	t.Helper()
	params := json.RawMessage(`{"name":"check_client","arguments":{},` + extra + `"_meta":` + meta + `}`)
	fr, refused := modernPrecheck(params, "")
	if refused != nil {
		fr = legacyFraming
	}
	resp := d.handle(scopedAgentCtx(principal.ScopeRead), rpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: methodToolsCall, Params: params,
	}, fr)
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func clientMeta(capabilities string) string {
	return `{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
		`"io.modelcontextprotocol/clientCapabilities":` + capabilities + `,` +
		`"io.modelcontextprotocol/clientInfo":{"name":"probe","version":"1.2"}}`
}

func checkData(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("no structured answer: %v", result)
	}
	data, ok := structured["data"].(map[string]any)
	if !ok {
		t.Fatalf("no data in the structured answer: %v", structured)
	}
	return data
}

func TestTheCheckAsksOnlyAClientThatDeclaredAForm(t *testing.T) {
	for _, tc := range []struct {
		name, capabilities string
		wantForm           bool
		wantReason         string
	}{
		{"an empty elicitation object reads as form", `{"elicitation":{}}`, true, ""},
		{"a form member", `{"elicitation":{"form":{}}}`, true, ""},
		{"form and url", `{"elicitation":{"form":{},"url":{}}}`, true, ""},
		{"url only", `{"elicitation":{"url":{}}}`, false, "only url mode"},
		{"none declared", `{}`, false, "no elicitation"},
		{"only a mode this server does not know", `{"elicitation":{"sampling":{}}}`, false, "no elicitation"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := callCheck(t, checkClientDispatcher(true), clientMeta(tc.capabilities), "")
			if tc.wantForm {
				if got[fieldResultType] != nil && got["inputRequests"] == nil {
					t.Fatalf("a form client was not asked: %v", got)
				}
				requests, _ := got["inputRequests"].(map[string]any)
				if _, asked := requests[clientCheckKey]; !asked {
					t.Fatalf("the form is not under %s: %v", clientCheckKey, got)
				}
				return
			}
			data := checkData(t, got)
			if data["supported"] != false || data["framing"] != "modern" {
				t.Errorf("a client with no form was not reported as unsupported in the modern framing: %v", data)
			}
			if reason, _ := data["reason"].(string); !strings.Contains(reason, tc.wantReason) {
				t.Errorf("reason %q does not say %q", reason, tc.wantReason)
			}
		})
	}
}

func TestTheRetryAnswersWhetherTheFormWasShown(t *testing.T) {
	form := clientMeta(`{"elicitation":{}}`)
	for _, tc := range []struct {
		name, responses string
		supported       bool
		action          string
	}{
		{"accept", `{"margince_client_check":{"action":"accept","content":{"colour":"green"}}}`, true, "accept"},
		{"decline", `{"margince_client_check":{"action":"decline"}}`, true, "decline"},
		{"cancel", `{"margince_client_check":{"action":"cancel"}}`, true, "cancel"},
		{"a colour that was not offered", `{"margince_client_check":{"action":"accept","content":{"colour":"pink"}}}`, false, ""},
		{"an entry under another key", `{"other":{"action":"accept"}}`, false, ""},
		{"an entry that is not an object", `{"margince_client_check":"yes"}`, false, ""},
		{"an unknown action", `{"margince_client_check":{"action":"maybe"}}`, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := checkData(t, callCheck(t, checkClientDispatcher(true), form, `"inputResponses":`+tc.responses+`,`))
			if data["supported"] != tc.supported {
				t.Errorf("supported = %v, want %v: %v", data["supported"], tc.supported, data)
			}
			if tc.supported && data["action"] != tc.action {
				t.Errorf("action = %v, want %s", data["action"], tc.action)
			}
			if !tc.supported && data["reason"] == nil {
				t.Errorf("an unsupported answer names no reason: %v", data)
			}
		})
	}
}

func TestTheCheckNamesTheClientAndTheEra(t *testing.T) {
	data := checkData(t, callCheck(t, checkClientDispatcher(true), clientMeta(`{}`), ""))
	client, _ := data["client"].(map[string]any)
	if client["name"] != "probe" || client["version"] != "1.2" || data["protocol_version"] != "2026-07-28" {
		t.Errorf("the answer does not name the client and revision: %v", data)
	}
	missing := `{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientCapabilities":{}}`
	if data := checkData(t, callCheck(t, checkClientDispatcher(true), missing, "")); data["client"] != nil {
		t.Errorf("a client that sent no clientInfo is reported as %v, not null", data["client"])
	}
}

func TestAHandshakeEraCallIsReportedAndNeverAsked(t *testing.T) {
	data := checkData(t, callCheck(t, checkClientDispatcher(true), `{}`, ""))
	if data["supported"] != false || data["framing"] != "handshake" {
		t.Errorf("a handshake call was not reported as unsupported: %v", data)
	}
}

func TestACallToAToolTheGateWithholdsGetsTheUnknownToolAnswer(t *testing.T) {
	absent := callCheck(t, checkClientDispatcher(false), clientMeta(`{"elicitation":{}}`), "")
	d := checkClientDispatcher(true)
	d.registry = NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	nonexistent := callCheck(t, d, clientMeta(`{"elicitation":{}}`), "")
	if absent["isError"] != true || absent["isError"] != nonexistent["isError"] {
		t.Fatalf("a withheld tool answers unlike a name that does not exist:\n%v\n%v", absent, nonexistent)
	}
	if text(absent) != text(nonexistent) {
		t.Errorf("the refusal tells the caller the tool exists: %q vs %q", text(absent), text(nonexistent))
	}
}

func text(result map[string]any) string {
	blocks, _ := result["content"].([]any)
	if len(blocks) == 0 {
		return ""
	}
	block, _ := blocks[0].(map[string]any)
	words, _ := block["text"].(string)
	return words
}

func TestAWithheldToolIsNotListedAndAnOfferedOneIs(t *testing.T) {
	listed := func(offered bool) bool {
		for _, tool := range checkClientDispatcher(offered).toolList(scopedAgentCtx(principal.ScopeRead), framing{}) {
			if tool[fieldName] == clientCheckTool {
				return true
			}
		}
		return false
	}
	if listed(false) {
		t.Error("tools/list names a tool the gate withholds")
	}
	if !listed(true) {
		t.Error("tools/list leaves out a tool the gate offers")
	}
}

func TestARunNeverHoldsAGatedToolEvenWhenTheGateOffersIt(t *testing.T) {
	for _, spec := range checkClientDispatcher(true).registry.Runnable(scopedAgentCtx(principal.ScopeRead)) {
		if spec.Name == clientCheckTool {
			t.Fatal("Runnable offers a Gated tool to a scheduled run")
		}
	}
}

func TestAGatedToolThatIsListedIsRefused(t *testing.T) {
	spec := checkClient{}.Spec()
	spec.Unlisted, spec.ProtocolOnly = false, false
	defer func() {
		if recover() == nil {
			t.Error("a Gated tool that every caller would see was registered")
		}
	}()
	assertGatedShape(spec)
}

// ProtocolOnly skips both REST parity gates, so Register refuses it on a tool
// that is listed to everyone or that names a REST route.
func TestProtocolOnlyIsRefusedOnAToolThatIsListedOrHasARoute(t *testing.T) {
	good := checkClient{}.Spec()
	for name, mutate := range map[string]func(*mcp.ToolSpec){
		"listed":      func(spec *mcp.ToolSpec) { spec.Unlisted = false },
		"not gated":   func(spec *mcp.ToolSpec) { spec.Gated = false },
		"has a route": func(spec *mcp.ToolSpec) { spec.OpenAPIOp = "getThing" },
	} {
		t.Run(name, func(t *testing.T) {
			spec := good
			mutate(&spec)
			defer func() {
				if recover() == nil {
					t.Errorf("a %s ProtocolOnly tool was registered", name)
				}
			}()
			assertProtocolOnlyShape(spec)
		})
	}
	assertProtocolOnlyShape(good)
}

// factsProbe reads the request facts without being ProtocolOnly.
type factsProbe struct{}

func (factsProbe) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "probe_facts", Title: "Probe", Version: toolVersionV1, Description: "reads the request facts",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		InputSchema: schema(`{"type":"object","properties":{},"additionalProperties":false}`),
	}
}

func (factsProbe) Handle(ctx context.Context, _ json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(map[string]bool{"modern": requestFactsOf(ctx).modern})
}

func TestOnlyAProtocolOnlyToolIsHandedTheRequestFacts(t *testing.T) {
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(factsProbe{})
	d := NewDispatcher(registry, bindAuthenticated, "margince-crm", "test").WithLogger(discardLog())
	params := json.RawMessage(`{"name":"probe_facts","arguments":{},"_meta":` + clientMeta(`{}`) + `}`)
	fr, refused := modernPrecheck(params, "")
	if refused != nil {
		t.Fatalf("a modern call was refused before dispatch: %v", refused)
	}
	resp := d.handle(scopedAgentCtx(principal.ScopeRead), rpcRequest{
		JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: methodToolsCall, Params: params,
	}, fr)
	raw, err := json.Marshal(resp.Result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `modern\":false`) {
		t.Errorf("an ordinary tool was handed the request facts: %s", raw)
	}
}

func TestACallThatAsksForInputTheClientDidNotDeclareIsRefusedAsADefect(t *testing.T) {
	d := checkClientDispatcher(true)
	for _, fr := range []framing{{}, {modern: true}, {modern: true, elicitation: elicitationURLOnly}} {
		got := d.inputRequiredAnswer(clientCheckTool, &InputRequired{}, fr)
		refusal, ok := got.(map[string]any)
		if !ok || refusal["isError"] != true {
			t.Errorf("framing %+v was asked for input: %v", fr, got)
		}
	}
}

func TestAnInputRequiredResultCarriesItsResultType(t *testing.T) {
	got := callCheck(t, checkClientDispatcher(true), clientMeta(`{"elicitation":{}}`), "")
	if got[fieldResultType] != resultTypeInputRequired {
		t.Errorf("resultType = %v, want %s", got[fieldResultType], resultTypeInputRequired)
	}
}

func TestAnAnswerFromAClientThatDeclaredNoFormIsNotCounted(t *testing.T) {
	answer := `"inputResponses":{"margince_client_check":{"action":"accept","content":{"colour":"red"}}},`
	for _, caps := range []string{`{}`, `{"elicitation":{"url":{}}}`} {
		data := checkData(t, callCheck(t, checkClientDispatcher(true), clientMeta(caps), answer))
		if data["supported"] != false {
			t.Errorf("a client declaring %s measured as supported from an answer it should not send: %v", caps, data)
		}
	}
}

func TestLongDeclaredCapabilitiesAreNotQuotedBack(t *testing.T) {
	long := `{"x":"` + strings.Repeat("a", maxDeclaredEcho) + `"}`
	data := checkData(t, callCheck(t, checkClientDispatcher(true), clientMeta(long), ""))
	if data["declared"] != nil {
		t.Errorf("a %d-byte capabilities object was quoted back: %v", len(long), data["declared"])
	}
}

func TestTheFormOffersTheColoursTheCheckAccepts(t *testing.T) {
	var form struct {
		RequestedSchema struct {
			Properties struct {
				Colour struct {
					Enum []string `json:"enum"`
				} `json:"colour"`
			} `json:"properties"`
		} `json:"requestedSchema"` //nolint:tagliatelle // the protocol's own member name
	}
	if err := json.Unmarshal([]byte(clientCheckForm), &form); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(form.RequestedSchema.Properties.Colour.Enum, clientCheckColours) {
		t.Errorf("the form offers %v but the check accepts %v", form.RequestedSchema.Properties.Colour.Enum, clientCheckColours)
	}
}
