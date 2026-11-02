// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
	"github.com/margince/margince/backend/pkg/extension"
)

// StartInstrumentation calls every composed unit's Instrumentation.Start and
// installs what they return. A role calls it once, after RegisterExtensions
// and before InstallProcessLogger, handing it a logger on the base handler so
// a unit's own diagnostics never pass through a unit's wrapper.
func StartInstrumentation(ctx context.Context, role string, boot *slog.Logger) {
	startInstrumentation(ctx, ComposedExtensions(), role, boot)
}

func startInstrumentation(ctx context.Context, exts []extension.Extension, role string, boot *slog.Logger) {
	version := buildinfo.ReleaseVersion
	if version == "" {
		version = buildinfo.Unknown
	}
	process := extension.Process{Role: role, Version: version, Logger: boot, RedactPath: capabilitypath.Redact}
	var started []instrument.Started
	for _, e := range exts {
		if e.Instrumentation == nil || e.Instrumentation.Start == nil {
			continue
		}
		hooks, err := startOne(ctx, e, process)
		if err != nil {
			boot.Error("extension instrumentation did not start; continuing without it",
				"unit", string(e.Name), "err", err)
			continue
		}
		started = append(started, instrument.Started{Unit: string(e.Name), Hooks: hooks})
	}
	instrument.Install(started, boot)
}

func startOne(ctx context.Context, e extension.Extension, p extension.Process) (hooks extension.Instruments, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("start panicked: %v\n%s", rec, debug.Stack())
		}
	}()
	return e.Instrumentation.Start(ctx, p)
}
