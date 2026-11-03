// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/margince/margince/backend/internal/platform/instrument"
	"github.com/margince/margince/backend/internal/shared/buildinfo"
	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
	"github.com/margince/margince/backend/pkg/extension"
)

// instrumentFlushWindow bounds how long a unit may take to flush at shutdown.
const instrumentFlushWindow = 2 * time.Second

// StartInstrumentation calls every composed unit's Instrumentation.Start and
// installs what they return. A role calls it once, after RegisterExtensions
// and before InstallProcessLogger. The logger it is handed sits on the base
// handler, so a unit's own diagnostics never pass through a unit's wrapper.
func StartInstrumentation(ctx context.Context, role string, boot *slog.Logger) {
	startUnits(ctx, ComposedExtensions(), role, boot)
}

// FlushInstrumentation gives every unit's Shutdown hook a bounded window to
// flush. A role defers it so it runs after the signal context is cancelled,
// which is why the window is detached from ctx's cancellation.
func FlushInstrumentation(ctx context.Context) {
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), instrumentFlushWindow)
	defer cancel()
	instrument.Shutdown(flushCtx)
}

func startUnits(ctx context.Context, exts []extension.Extension, role string, boot *slog.Logger) {
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
