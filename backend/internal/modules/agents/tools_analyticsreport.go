// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// compose_analytics_report — a report whose every figure came from the
// database.
//
// The tool takes a document and hands it to the same validator and renderer the
// HTTP surface uses. It does not decide what a block may contain and it does
// not resolve a handle: both live in compose, behind one seam, so the tool
// surface and the web surface cannot come to disagree about what a report may
// say.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// AnalyticsReportComposer renders a validated document. Implemented in compose,
// over the same RenderReport the HTTP handler calls.
type AnalyticsReportComposer func(ctx context.Context, doc json.RawMessage) (json.RawMessage, error)

// RegisterAnalyticsReportTool adds the composer to the surface.
func RegisterAnalyticsReportTool(r *Registry, render AnalyticsReportComposer) {
	r.Register(composeAnalyticsReport{render: render})
}

type composeAnalyticsReport struct {
	render AnalyticsReportComposer
}

func (t composeAnalyticsReport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "compose_analytics_report", Title: "Compose an analytics report",
		Version:     toolVersionV1,
		Description: composeAnalyticsReportCopy.render(),
		// Read, not write: composing renders a document from runs that already
		// exist. Nothing is stored and no record moves, so a write scope would
		// claim an authority this never uses.
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// The block vocabulary is NOT restated here. `kind` is left an open
		// string in the schema and closed by the validator, which refuses an
		// unknown kind BY NAME with the set — the same trade run_report makes
		// for its per-report vocabularies. Reciting fourteen block kinds and
		// their fields would cost more of every caller's window, every step,
		// than the one round trip it saves.
		InputSchema: schema(`{
			"type": "object",
			"required": ["blocks"],
			"additionalProperties": false,
			"properties": {
				"blocks": {
					"type": "array",
					"minItems": 1,
					"items": {
						"type": "object",
						"required": ["kind"],
						"additionalProperties": false,
						"properties": {
							"kind": {"type": "string"},
							"text": {"type": "string"},
							"severity": {"type": "string"},
							"cells": {
								"type": "array",
								"items": {
									"type": "object",
									"required": ["run_id", "column"],
									"additionalProperties": false,
									"properties": {
										"run_id": {"type": "string"},
										"column": {"type": "string"},
										"group": {"type": "array"}
									}
								}
							}
						}
					}
				}
			}
		}`),
		OutputSchema: schemaFor[ComposeAnalyticsReportResult](),
	}
}

// ComposeAnalyticsReportResult is the rendered document.
//
// Blocks is raw rather than typed here for the reason the vocabulary is not in
// the schema: the block shapes are compose's, and a second Go spelling of them
// in this package would be the drift the seam exists to prevent.
type ComposeAnalyticsReportResult struct {
	// Blocks carries each composed block with its figures resolved, in the
	// order the document composed them.
	Blocks json.RawMessage `json:"blocks"`
}

func (t composeAnalyticsReport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// Passed through unparsed. The document's shape is decided by the
	// validator, which refuses by naming the block and the reason; decoding it
	// into a second set of Go types here would be a second answer to what a
	// report may contain.
	return t.render(ctx, in)
}
