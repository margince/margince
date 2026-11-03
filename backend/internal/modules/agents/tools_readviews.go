// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The read_view catalog, reachable by a caller that reads tools and not
// resources. It serves the resource's own rendering, so the two doors answer
// one document.

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// DescribeViewsResult carries the published catalog verbatim.
type DescribeViewsResult struct {
	Views json.RawMessage `json:"views"`
}

// ReadViewsReader answers the read_view catalog. ReadViewsResource satisfies it.
type ReadViewsReader interface {
	ReadViewsDocument(ctx context.Context) (json.RawMessage, error)
}

// RegisterReadViewsTool adds describe_views. Unconditional, like read_view.
func RegisterReadViewsTool(r *Registry, read ReadViewsReader) {
	r.Register(describeViews{read: read})
}

type describeViews struct {
	read ReadViewsReader
}

func (t describeViews) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_views", Title: "Describe the views",
		Version:       toolVersionV1,
		Description:   describeViewsCopy.render(),
		Instead:       describeViewsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp:    "describeReadViews",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[DescribeViewsResult](),
	}
}

func (t describeViews) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	body, err := t.read.ReadViewsDocument(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(DescribeViewsResult{Views: body})
}

var _ ReadViewsReader = ReadViewsResource{}
