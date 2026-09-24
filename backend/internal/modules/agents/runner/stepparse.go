// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// The step protocol as this package READS it: the one place model output
// becomes a step. stepschema.go is the same protocol as the provider enforces
// it, and TestTheStepSchemaAdmitsExactlyWhatTheStepParserAccepts holds the two
// to one contract.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/modelreply"
)

// modelStep is the step protocol: exactly one of tool-call or final.
type modelStep struct {
	Tool  string          `json:"tool"`
	Args  json.RawMessage `json:"args"`
	Final json.RawMessage `json:"final"`
}

func parseStep(text string) (modelStep, error) {
	// SoleDocument, not Unfence: this channel executes what it reads, so a
	// reply holding two candidate documents is refused rather than resolved by
	// size. See modelreply.SoleDocument — largest-wins hands back an injected
	// step that the model quoted while refusing it.
	cleaned := modelreply.SoleDocument(text)

	var envelope stepEnvelope
	dec := json.NewDecoder(strings.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&envelope); err != nil {
		return modelStep{}, fmt.Errorf(`expected {"tool":..., "args":{...}} or {"final":{...}}: %w`, err)
	}
	// A step is the WHOLE document. json.Decoder stops at the first value and
	// discards what follows it unread, so a reply that LEADS with a quoted
	// injection — `{…} — I will not do that` — decodes the quotation and the
	// refusal after it is never seen. The reduction cannot help here: the
	// document really is at the start of the reply.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return modelStep{}, errors.New("a step must be the whole reply, and this one carries text after the document")
	}
	if (envelope.Tool != nil) == (envelope.Final != nil) {
		return modelStep{}, errors.New(`exactly one of "tool" or "final" must be set`)
	}
	if envelope.Final != nil {
		return finalStep(envelope)
	}
	return toolStep(envelope)
}

// stepEnvelope is a step as decoded, before its shape is checked. Every member
// is raw, so a key that is PRESENT but null is told apart from one that is
// absent: `"tool":null` beside a final is two steps' keys, not one.
type stepEnvelope struct {
	Tool  json.RawMessage `json:"tool"`
	Args  json.RawMessage `json:"args"`
	Final json.RawMessage `json:"final"`
}

// toolStep checks a tool call against the tool-call branches of stepSchema:
// a name, and an args OBJECT — `{}` for a tool that takes none. Which names
// and which arguments are allowed is the allowlist's and the tool's to judge.
func toolStep(envelope stepEnvelope) (modelStep, error) {
	var name string
	if json.Unmarshal(envelope.Tool, &name) != nil || name == "" {
		return modelStep{}, errors.New(`"tool" must name the tool to call`)
	}
	if len(name) > maxToolNameLen {
		// A tool name is a registry identifier, so a long one is not a typo —
		// it is the model writing a payload into a field the trace persists and
		// the refusal path echoes. Bound it here, at the one place model output
		// becomes a step, rather than at each place it is later printed.
		return modelStep{}, fmt.Errorf("tool name is longer than %d characters", maxToolNameLen)
	}
	if !isJSONObject(envelope.Args) {
		return modelStep{}, errors.New(`a tool call carries an "args" object — {} when the tool takes no arguments`)
	}
	return modelStep{Tool: name, Args: envelope.Args}, nil
}

// finalStep checks a final against the final branch of stepSchema: nothing
// beside it, and an object carrying a "summary" string, which is what the
// rail shows under the finished run.
func finalStep(envelope stepEnvelope) (modelStep, error) {
	if envelope.Args != nil {
		return modelStep{}, errors.New(`"args" belongs to a tool call, not to a final`)
	}
	var final struct {
		Summary *string `json:"summary"`
	}
	if !isJSONObject(envelope.Final) || json.Unmarshal(envelope.Final, &final) != nil || final.Summary == nil {
		return modelStep{}, errors.New(`"final" must be an object carrying a "summary" string`)
	}
	return modelStep{Final: envelope.Final}, nil
}

// isJSONObject reports whether raw, already decoded as one JSON value, is an
// object rather than null, an array or a scalar.
func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}
