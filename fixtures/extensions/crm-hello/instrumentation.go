// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package crmhello

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/margince/margince/backend/pkg/extension"
)

// The smoke fixture's instrumentation is the least that walks the whole path.
// It has a Start at boot, a Work hook around every job, an edge the browser can
// reach without a session, and a browser export. Its Work hook only counts the
// runs it wraps, so a test can see the hook was called.
var workRuns atomic.Int64

func startInstrumentation(context.Context, extension.Process) (extension.Instruments, error) {
	return extension.Instruments{
		Work: func(ctx context.Context, _ extension.Work, run func(context.Context) error) error {
			workRuns.Add(1)
			return run(ctx)
		},
	}, nil
}

func instrumentationEdge() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	return mux
}
