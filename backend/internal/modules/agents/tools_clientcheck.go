// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

const (
	clientCheckTool   = "check_client"
	clientCheckKey    = "margince_client_check"
	framingModern     = "modern"
	framingHandshake  = "handshake"
	maxClientInfoEcho = 64
	maxDeclaredEcho   = 1024
)

var clientCheckColours = []string{"red", "green", "blue"}

// clientCheckQuestion is the one form the check shows. Its enum is
// clientCheckColours, which a test holds.
var clientCheckQuestion = chatQuestion{
	Key:     clientCheckKey,
	Message: "Margince client check: pick a colour. Nothing is changed.",
	Schema: json.RawMessage(`{"type":"object","required":["colour"],` +
		`"properties":{"colour":{"type":"string","enum":["red","green","blue"]}}}`),
}

// ClientCheckResult is what the check reports about one client.
type ClientCheckResult struct {
	Supported       bool            `json:"supported" jsonschema:"True when the client answered the form, whatever it chose."`
	Framing         string          `json:"framing" jsonschema:"modern or handshake: which era of the protocol the request used."`
	ProtocolVersion string          `json:"protocol_version" jsonschema:"The revision the request declared; empty in the handshake era."`
	Client          *ClientIdentity `json:"client" jsonschema:"The client's name and version, or null when it sent none."`
	Reason          string          `json:"reason,omitempty" jsonschema:"Why the client is not supported, or what was wrong with its answer."`
	Declared        json.RawMessage `json:"declared,omitempty" jsonschema:"The capabilities the client declared, when it could not be asked."`
	Action          string          `json:"action,omitempty" jsonschema:"accept, decline or cancel: what the user did with the form."`
	Answer          json.RawMessage `json:"answer,omitempty" jsonschema:"The form's values when the user accepted."`
}

// ClientIdentity is the name and version a client sends in its request metadata.
type ClientIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// checkClient asks the connecting client's user a real question. An admin then
// learns whether the client can answer a form, not only whether it says so.
type checkClient struct{ log *slog.Logger }

func (t checkClient) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: clientCheckTool, Title: "Check what an AI tool supports", Version: toolVersionV1,
		Description:   "Asks the user to pick a colour in a small form and reports whether this AI tool could show it. Changes no data.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		Unlisted: true, Gated: true, ProtocolOnly: true,
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[ClientCheckResult](),
	}
}

func (t checkClient) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct{}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	result, err := measureClient(ctx, requestFactsOf(ctx))
	if err != nil {
		return nil, err
	}
	var name, version string
	if result.Client != nil {
		name, version = result.Client.Name, result.Client.Version
	}
	t.log.Info("mcp: client check", "client", name, "version", version,
		"framing", result.Framing, "supported", result.Supported, "action", result.Action)
	return json.Marshal(result)
}

// measureClient asks the form through the chat mechanism every tool shares, so
// the check measures the very path a real question takes.
func measureClient(ctx context.Context, facts requestFacts) (ClientCheckResult, error) {
	result := ClientCheckResult{Framing: framingHandshake, ProtocolVersion: facts.version, Client: clientIdentityOf(facts.clientInfo)}
	if !facts.modern {
		result.Reason = "this server asks for input only in the 2026-07-28 framing; the handshake era needs a stream /mcp does not open"
		return result, nil
	}
	result.Framing = framingModern
	if !canAskInChat(ctx) {
		result.Reason = "the client declared no form elicitation"
		if facts.elicit {
			result.Reason = "the client declared form elicitation, but this server has no key to sign the question"
		}
		result.Declared = boundedDeclared(facts.capabilities)
		return result, nil
	}
	reply, err := replyInChat(ctx, clientCheckQuestion)
	if err != nil {
		return result, err
	}
	if !reply.Accepted {
		result.Supported, result.Action = true, reply.Action
		return result, nil
	}
	return acceptedColour(result, reply.Content), nil
}

// boundedDeclared quotes the declared capabilities when they are short, and
// says nothing when they are not: they are caller text.
func boundedDeclared(capabilities json.RawMessage) json.RawMessage {
	if len(capabilities) > maxDeclaredEcho {
		return nil
	}
	return capabilities
}

func acceptedColour(result ClientCheckResult, content map[string]json.RawMessage) ClientCheckResult {
	var colour string
	if json.Unmarshal(content["colour"], &colour) != nil || !slices.Contains(clientCheckColours, colour) {
		result.Reason = "an accepted form did not carry one of the offered colours"
		return result
	}
	result.Supported, result.Action = true, "accept"
	if answer, err := json.Marshal(map[string]string{"colour": colour}); err == nil {
		result.Answer = answer
	}
	return result
}

// clientIdentityOf reads clientInfo, which the specification requires but this
// server does not, so a missing one is null. The words are caller text and are
// bounded and escaped before they reach a log line or a transcript.
func clientIdentityOf(raw json.RawMessage) *ClientIdentity {
	var info ClientIdentity
	if raw == nil || json.Unmarshal(raw, &info) != nil || info.Name == "" {
		return nil
	}
	return &ClientIdentity{Name: echoSafe(info.Name, maxClientInfoEcho), Version: echoSafe(info.Version, maxClientInfoEcho)}
}

// RegisterClientCheckTool joins the measurement tool to the surface. The tool
// gate decides who it exists for.
func RegisterClientCheckTool(r *Registry, log *slog.Logger) {
	r.Register(checkClient{log: log})
}
