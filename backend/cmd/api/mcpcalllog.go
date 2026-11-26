// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/mcpcalllog"
	ps "github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/platform/config"
)

// mcpDoorLanes opens what /mcp serves beside its registry: the view documents
// and the call log. stop ends both.
func mcpDoorLanes(ctx context.Context, cfg apiConfig, logger *slog.Logger) ([]compose.Option, func(), error) {
	viewOpts, stopViews, err := mcpAppViewsLane(ctx, cfg, logger)
	if err != nil {
		return nil, nil, err
	}
	logOpts, closeLog, err := mcpCallLogLane(cfg, logger)
	if err != nil {
		stopViews()
		return nil, nil, err
	}
	return append(viewOpts, logOpts...), func() { closeLog(); stopViews() }, nil
}

// mcpCallLogLane opens the file mcp.call_log names, when it names one and the
// connector serves /mcp. Its lines hold personal data, so boot says it is on.
func mcpCallLogLane(cfg apiConfig, logger *slog.Logger) ([]compose.Option, func(), error) {
	path := config.Val(cfg.settings, ps.MCPCallLog)
	if path == "" || !cfg.mcpConnector {
		return nil, func() {}, nil
	}
	file, err := mcpcalllog.Open(path, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("api: %w", err)
	}
	logger.Warn("mcp: call log on; every /mcp call, its arguments and its answer, personal data included, is appended to this file",
		"path", path)
	closeFile := func() {
		if err := file.Close(); err != nil {
			logger.Error("mcp: the call log did not close cleanly", "path", path, "error", err)
		}
	}
	return []compose.Option{compose.WithMCPCallLog(file)}, closeFile, nil
}
