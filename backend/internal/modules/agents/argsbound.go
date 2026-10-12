// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// withinArgsBound refuses arguments larger than the tool declares, before any
// reading of them: the MCP transport admits one tool's file, not every tool's.
func withinArgsBound(spec mcp.ToolSpec, args []byte) error {
	limit := spec.MaxArgsBytes
	if limit == 0 {
		limit = httperr.MaxBodyBytes
	}
	if int64(len(args)) <= limit {
		return nil
	}
	return &BadArgsError{
		Cause: fmt.Errorf("the arguments are %s, over the %s this tool accepts",
			httperr.Megabytes(int64(len(args))), httperr.Megabytes(limit)),
		Guidance: "send less in one call; a file goes through attach_document, or is uploaded in the Margince app",
	}
}
