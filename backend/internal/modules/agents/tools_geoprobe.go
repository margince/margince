// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The check_location_support tool (🟢): the verb the location check hangs off.
//
// WHY A TOOL EXISTS AT ALL FOR A VIEW THAT NEEDS NO DATA. On this surface a view
// is "a document hung off a tool that already answers" — a host is told to
// render a card as part of a tool's result, and never fetches one on its own. A
// published document no tool names is one no host will ever show, which the
// composed-surface sweep refuses by name (TestEveryServedViewIsNamedByATool).
// So the probe needs a verb, and this is the smallest honest one.
//
// WHAT IT ANSWERS. Nothing about the device — a server cannot know where a
// caller is, and this tool does not guess. It answers what the SERVER has done:
// that its views ask their host for geolocation, and that the answer to whether
// the host grants it can only be read in the card beside this text. The finding
// is produced in the browser, by the view, and it is the browser's own error
// string that carries it.
//
// IT IS A PROBE AND IS MEANT TO BE DELETED, along with the view, once every host
// in the matrix has answered. See apps.GeoProbeURI.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

var checkLocationSupportCopy = toolCopy{
	Purpose: "Find out whether this chat host lets a Margince card read the device's location, " +
		"which is what would let a contact be tagged with the event you are standing at.",
	Limits: "It does not read a location and cannot: the answer comes from the card shown beside " +
		"this result, and only after the person using it presses the button on that card. " +
		"A host is free to refuse, and refusing is the expected outcome until one is shown not to.",
	Instead: "To record where something happened, put it in the activity you log with log_activity; " +
		"this tool tags nothing and writes nothing.",
}

// LocationSupport is what the server can say on its own: what it asked for, and
// where the actual answer has to come from.
type LocationSupport struct {
	// Declared is the permission this build's views request of their host.
	Declared string `json:"declared_permission"`
	// AnsweredBy names the surface that produces the finding, because it is not
	// this one.
	AnsweredBy string `json:"answered_by"`
	// Note is the sentence a client with no view renders instead of the card.
	// A host without the Apps extension gets a straight answer rather than a
	// reference to a panel it will never show.
	Note string `json:"note"`
}

type checkLocationSupportTool struct{}

func (t checkLocationSupportTool) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "check_location_support", Title: "Can a card read this device's location", Version: toolVersionV1,
		Description:   checkLocationSupportCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// No OpenAPIOp: there is no REST operation behind this and there should
		// not be one. It reads no record, so a second door onto it would be a
		// door onto nothing. search_context is the standing precedent for a tool
		// with no twin.
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[LocationSupport](),
		// The whole point of the tool. The card is where the browser is, and the
		// browser is the only thing that can answer.
		UI: &mcp.ToolUI{ResourceURI: apps.GeoProbeURI},
	}
}

func (t checkLocationSupportTool) Handle(context.Context, json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(LocationSupport{
		Declared:   "geolocation",
		AnsweredBy: "the Location check card shown beside this answer",
		Note: "This build's cards ask their host for permission to read the device's position. " +
			"Whether the host grants it is the host's decision and cannot be read from here. " +
			"Open the card beside this answer and press Read my location: the message it prints " +
			"is the finding. Wording about a permissions policy means the host never allowed the " +
			"card to ask, and nobody was prompted.",
	})
}

// RegisterGeoProbeTool wires the location check.
//
// Unconditional, unlike the seam-backed registrations around it: it depends on
// no store and no reader, so there is no absent seam that would make it lie.
func RegisterGeoProbeTool(r *Registry) {
	r.Register(checkLocationSupportTool{})
}
