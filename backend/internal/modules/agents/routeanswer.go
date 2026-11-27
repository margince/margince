// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"sync"
)

// RouteAnswerTruncated is the member a write's answer carries when its body was
// too long to return. The write has committed, so the caller must not retry.
const RouteAnswerTruncated = "truncated"

// writeAnswerSchema admits the two members of a write's cut answer beside the
// route's own, and so can require none of the route's.
func writeAnswerSchema(answer json.RawMessage) json.RawMessage {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(answer, &shape); err != nil {
		return answer
	}
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(shape["properties"], &properties); err != nil || properties == nil {
		properties = map[string]json.RawMessage{}
	}
	// A route's own status may be a word, and a cut answer's is the HTTP code. This
	// surface refuses anyOf, so a status that may be either stays untyped.
	if _, declared := properties["status"]; declared {
		properties["status"] = json.RawMessage(`{}`)
	} else {
		properties["status"] = json.RawMessage(`{"type":"integer"}`)
	}
	properties[RouteAnswerTruncated] = json.RawMessage(`{"type":"boolean"}`)
	shape["properties"] = mustMarshal(properties)
	delete(shape, "required")
	return mustMarshal(shape)
}

// routeAnswer is what a verb's spec reads off its route's answer schema.
type routeAnswer struct {
	write    string
	freeText bool
}

// routeAnswers holds a routeAnswer per answer schema text. Both members are read
// off the text alone. Without it, each composition and each routed call (a
// router asks its target's spec again) decodes the same schemas once more.
var routeAnswers sync.Map

func routeAnswerOf(schema json.RawMessage) routeAnswer {
	if known, ok := routeAnswers.Load(string(schema)); ok {
		if facts, ok := known.(routeAnswer); ok {
			return facts
		}
	}
	facts := routeAnswer{write: string(writeAnswerSchema(schema)), freeText: CarriesFreeText(schema)}
	routeAnswers.Store(string(schema), facts)
	return facts
}
