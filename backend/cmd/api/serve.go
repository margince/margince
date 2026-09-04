// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Serving the composed handler, and stopping it without dropping work.
//
// Its own file beside boot.go, which ASSEMBLES the role. What happens after
// assembly — the operational limits a listener runs under, and the window
// in-flight requests get when a signal arrives — is a separate concern with a
// separate failure mode, and boot.go is about what the role is made of.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// serveUntilSignal serves the composed handler with explicit operational
// limits — a server without timeouts leaks connections under slow clients —
// until the listener fails or ctx is cancelled. Shutdown drains in-flight
// requests inside a bounded window of its own: the ctx that ended the serve is
// already cancelled, and reusing it would abandon those requests rather than
// give them time to finish.
func serveUntilSignal(ctx context.Context, cfg apiConfig, handler http.Handler, stdout io.Writer) error {
	srv := &http.Server{
		Addr:              cfg.addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	if cfg.inlineRelay {
		_, _ = fmt.Fprintf(stdout, "api listening on %s (base path /v1), relaying events to %s\n", cfg.addr, cfg.redisAddr)
	} else {
		_, _ = fmt.Fprintf(stdout, "api listening on %s (base path /v1); the outbox relay runs in cmd/worker\n", cfg.addr)
	}

	//nolint:contextcheck // the drain gets its own context on purpose: ctx is already cancelled by the time this runs, and a cancelled one would abandon in-flight requests instead of bounding them.
	stopHTTP := func() error {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return stopHTTP()
	}
}
