// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/modelreply"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// objectRootKey is the one property a non-object root is carried under on the
// OpenAI wire, whose response_format takes nothing but an object at the root.
const objectRootKey = "value"

// completeOnOpenAIWire runs one OpenAI-wire completion with its schema made
// object-rooted, and hands the caller the answer to the schema it asked for.
//
// Complete only: a stream's tokens reach the caller before any unwrap could, so
// Stream sends the schema as given and the endpoint answers for it.
func completeOnOpenAIWire(ctx context.Context, req model.Request,
	complete func(context.Context, model.Request) (model.Response, error),
) (model.Response, error) {
	ctx, attempt := trackHTTPAttempt(ctx)
	sent, wrapped := objectRooted(req.ResponseSchema)
	req.ResponseSchema = sent
	resp, err := complete(ctx, req)
	if err == nil && wrapped {
		resp.Text = unwrapObjectRoot(resp.Text)
	}
	return reportSchemaDowngrade(resp, err, strictDowngrade(sent), attempt)
}

// objectRooted returns raw itself when its root already describes an object,
// and otherwise raw as the sole required property of a closed object.
//
// A root this cannot decode is sent as given: the endpoint's own refusal names
// the fault better than a guess here would.
func objectRooted(raw json.RawMessage) (json.RawMessage, bool) {
	var root schemaNode
	if len(raw) == 0 || json.Unmarshal(raw, &root) != nil || root.isObject() {
		return raw, false
	}
	wrapper, err := json.Marshal(struct {
		Type                 string                     `json:"type"`
		Properties           map[string]json.RawMessage `json:"properties"`
		Required             []string                   `json:"required"`
		AdditionalProperties bool                       `json:"additionalProperties"`
	}{kwObject, map[string]json.RawMessage{objectRootKey: raw}, []string{objectRootKey}, false})
	if err != nil {
		return raw, false
	}
	return wrapper, true
}

// unwrapObjectRoot is the answer under objectRootKey. A reply that is not
// only that wrapper, such as one cut off or answered bare, is handed on
// unchanged for the caller's own parser to judge.
//
// SoleDocument, not Unfence: the agent loop executes what this returns, so an
// ambiguous reply must reach its parser whole rather than reduced by size.
func unwrapObjectRoot(text string) string {
	var wrapper map[string]json.RawMessage
	if json.Unmarshal([]byte(modelreply.SoleDocument(text)), &wrapper) != nil || len(wrapper) != 1 {
		return text
	}
	inner, carried := wrapper[objectRootKey]
	if !carried {
		return text
	}
	return string(inner)
}
