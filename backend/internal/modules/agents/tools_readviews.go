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

// DescribeViewsResult carries the published catalog.
type DescribeViewsResult struct {
	Views readViewsDoc `json:"views"`
}

// RegisterReadViewsTool adds describe_views. Unconditional, like read_view.
func RegisterReadViewsTool(r *Registry, read ReadViewsResource) {
	r.Register(describeViews{read: read})
}

type describeViews struct {
	read ReadViewsResource
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

func (t describeViews) Handle(_ context.Context, in json.RawMessage) (json.RawMessage, error) {
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	return json.Marshal(DescribeViewsResult{Views: readViewsDocument(t.read.views)})
}
