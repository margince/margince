// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// resultTypeInputRequired is the modern resultType of a call that needs the
// user to answer a request before it can finish.
const resultTypeInputRequired = "input_required"

// elicitationMode is what the client declared it can show for an elicitation.
type elicitationMode int

const (
	elicitationNone elicitationMode = iota
	elicitationURLOnly
	elicitationForm
)

// declaredElicitation reads the `elicitation` capability. The specification
// reads an empty object as form mode, so `{}` and a `form` member both offer a
// form. An object with `url` and no `form` offers url only, and one naming
// neither offers nothing this server can ask in.
func declaredElicitation(capabilities json.RawMessage) elicitationMode {
	var declared map[string]json.RawMessage
	if json.Unmarshal(capabilities, &declared) != nil {
		return elicitationNone
	}
	if !isJSONObject(declared["elicitation"]) {
		return elicitationNone
	}
	var modes map[string]json.RawMessage
	if json.Unmarshal(declared["elicitation"], &modes) != nil {
		return elicitationNone
	}
	_, hasForm := modes["form"]
	if hasForm || len(modes) == 0 {
		return elicitationForm
	}
	if _, hasURL := modes["url"]; hasURL {
		return elicitationURLOnly
	}
	return elicitationNone
}

// requestFacts is what one tools/call said about its client and the answers it
// carries. Only a tool that measures the protocol reads it.
type requestFacts struct {
	modern      bool
	version     string
	elicitation elicitationMode
	// capabilities is the declared client capabilities, as sent.
	capabilities json.RawMessage
	// clientInfo is the `_meta` clientInfo member, as sent.
	clientInfo json.RawMessage
	// responses is params.inputResponses, nil when the call carried none.
	responses map[string]json.RawMessage
}

type requestFactsKey struct{}

func withRequestFacts(ctx context.Context, facts requestFacts) context.Context {
	return context.WithValue(ctx, requestFactsKey{}, facts)
}

func requestFactsOf(ctx context.Context) requestFacts {
	facts, _ := ctx.Value(requestFactsKey{}).(requestFacts)
	return facts
}

// InputRequest is one question the server asks the client's user.
type InputRequest struct {
	Method string
	Params json.RawMessage
}

// InputRequired is the error a handler returns to ask for input. The dispatcher
// answers it with an input_required result, only in the modern framing and only
// for a mode the client declared.
type InputRequired struct {
	Requests map[string]InputRequest
}

func (*InputRequired) Error() string { return "the call needs input from the client's user" }

// inputRequiredResult is the modern result of a call that needs input.
type inputRequiredResult map[string]any

func (inputRequiredResult) resultType() string { return resultTypeInputRequired }

func renderInputRequired(asked *InputRequired) inputRequiredResult {
	requests := make(map[string]any, len(asked.Requests))
	for key, request := range asked.Requests {
		requests[key] = map[string]any{"method": request.Method, "params": request.Params}
	}
	return inputRequiredResult{"inputRequests": requests}
}

// inputRequired reports the request the error carries, if it carries one.
func inputRequired(err error) (*InputRequired, bool) {
	var asked *InputRequired
	return asked, errors.As(err, &asked)
}

// inputRequiredAnswer renders a handler's request for input. A server may ask
// only in the modern framing and only in a mode the client declared. Anything
// else is this server's defect, and is logged as one.
//
//craft:ignore naked-any the answer is either a tool error or an input_required result, both rendered as one result map by the framing
func (s *Dispatcher) inputRequiredAnswer(tool string, asked *InputRequired, fr framing) any {
	if !fr.modern || fr.elicitation != elicitationForm {
		s.log.Error("mcp: a tool asked for input the client did not declare", "tool", tool)
		return toolError(internalFaultAdvice)
	}
	return renderInputRequired(asked)
}

// requestFacts is what this request said about its client, plus the answers it
// carries. Only a ProtocolOnly tool is handed it.
func (fr framing) requestFacts(responses map[string]json.RawMessage) requestFacts {
	return requestFacts{
		modern: fr.modern, version: fr.version, elicitation: fr.elicitation,
		capabilities: fr.capabilities, clientInfo: fr.clientInfo, responses: responses,
	}
}

// noteHiddenCall logs the traffic a hidden name still carries, which decides
// when it can go. A name the gate withheld is logged as unknown, so the log
// does not tell an operator that a withheld tool exists either.
func (s *Dispatcher) noteHiddenCall(spec mcp.ToolSpec, registered bool, err error) {
	var unknown *UnknownToolError
	if registered && spec.Unlisted && !errors.As(err, &unknown) {
		s.log.Info("mcp: a hidden tool was called by its own name", "tool", spec.Name)
	}
}
