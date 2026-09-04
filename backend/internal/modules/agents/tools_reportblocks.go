// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// describe_report_blocks — the block grammar as a tool, beside the
// margince://schema/report-blocks resource.
//
// Both doors for the same reason describe_report_vocabulary exists beside
// margince://schema/reports: the Surface-B runner is offered no resource step
// at all, so for a scheduled agent a tool is the ONLY route to the names
// compose_analytics_report refuses against.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// RegisterReportBlocksTool adds the grammar reader to the surface.
func RegisterReportBlocksTool(r *Registry, read ReportBlocksReader) {
	r.Register(describeReportBlocks{read: read})
}

type describeReportBlocks struct {
	read ReportBlocksReader
}

// DescribeReportBlocksResult is the document, unwrapped.
type DescribeReportBlocksResult struct {
	Blocks json.RawMessage `json:"blocks"`
}

func (t describeReportBlocks) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_report_blocks", Title: "Describe the report block grammar",
		Version:       toolVersionV1,
		Description:   describeReportBlocksCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		// No arguments. The grammar is the same for every caller — it is the
		// engine's, not a workspace's — so a filter would only let one narrow
		// what it already receives, at the cost of a name it could spell wrong.
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[DescribeReportBlocksResult](),
	}
}

func (t describeReportBlocks) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// Decoded even with nothing to read, because decodeNoArguments is what
	// enforces the declared additionalProperties: false — without it a call
	// carrying a stray key is accepted silently.
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	// The resource's own composition, not a second rendering of the grammar.
	body, err := t.read.ReportBlocksDocument(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(DescribeReportBlocksResult{Blocks: body})
}
