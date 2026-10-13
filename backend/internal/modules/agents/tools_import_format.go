// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// describeImportFormat answers the format the template and the screen's column
// guide are built from, so an agent writes files the import reads.
type describeImportFormat struct{ imports Imports }

func (t describeImportFormat) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_import_format", Title: "Describe the import format", Version: toolVersionV1,
		Description:   describeImportFormatCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp:    "getImportFormat",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[ImportFormatResult](),
	}
}

func (t describeImportFormat) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct{}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	format, err := t.imports.DescribeFormat(ctx)
	if err != nil {
		return nil, err
	}
	return json.Marshal(ImportFormatResult{Format: format})
}
