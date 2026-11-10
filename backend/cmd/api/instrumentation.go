// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"io"
	"log/slog"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// startInstrumentation starts the composed units' instrumentation. It runs
// after registration and before the process logger, so that logger is built
// already wrapped by the units' handlers. The boot logger it hands the units
// sits on the bare handler, so a unit's own diagnostics never pass through a
// unit's wrapper. The caller defers compose.FlushInstrumentation.
func startInstrumentation(ctx context.Context, cfg apiConfig, stdout io.Writer) error {
	bootHandler, err := httpserver.LogHandler(stdout, cfg.logLevel, cfg.logFormat)
	if err != nil {
		return err
	}
	compose.StartInstrumentation(ctx, "api", slog.New(bootHandler))
	return nil
}
