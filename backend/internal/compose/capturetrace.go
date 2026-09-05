// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The capture-trace read surface: what the pipeline did with a member's
// messages in the last 24 hours.
//
// It is composed separately from the capture pipeline itself because the two
// answer to different things. The pipeline is wired wherever capture runs; this
// surface exists only where there is an HTTP role to serve it, and it carries
// the deployment's payload posture, which the pipeline takes as a Sink option.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/pipelinetrace"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/jobs"
)

// WithCaptureTrace composes the trace read surface.
//
// tracePayloads is the deployment's capture.trace_payloads posture, reported on
// every answer so a client can distinguish "the operator did not enable this"
// from "this row has no payload" — the same distinction /ai/calls draws with
// payload_capture_enabled, and for the same reason.
func WithCaptureTrace(tracePayloads bool) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		traces := capture.NewTraceStore(InstallationDB(pool))
		s.traceHandlers = capture.NewTraceHandlers(traces, tracePayloads, verdictPass(pool, CounterpartyVerdictArgs{}.Kind()))
		// The per-message ladder is composed HERE rather than in its own Option
		// because it answers the same question as the window above it, from the
		// same rows and the same posture. Two options would let a deployment
		// wire the list without the drawer it opens — a row a member can click
		// and nothing behind it.
		//
		// It is the cross-module edge: capture holds the stored rungs and
		// activities holds the label and the person link, and neither may
		// import the other, so compose injects both.
		s.pipelineTraceHandlers = pipelinetrace.NewHandlers(pipelinetrace.NewAssembler(
			traces, activities.NewStore(InstallationDB(pool)), tracePayloads))
	}
}

// verdictPass reads when one periodic verdict pass next runs.
//
// The seam exists because the two halves belong to different owners and neither
// may reach the other: the schedule is the job queue's, and capture must not
// learn to read river_job — platform/jobs says why every statement over that
// table lives in one package. The kinds come from the ARGS TYPES the runner
// registers rather than from string literals: a kind renamed in one place and
// not the other would leave this reading a queue nobody writes and reporting
// "no pass is coming" forever.
//
// One pass per surface, named where a person meets the wait. The sender verdict
// is what the capture-activity counters wait on; the thread verdict, an hour
// faster, is what a held thread waits on. Handing both to both screens would put
// a clock on each that nothing there is waiting for.
func verdictPass(pool *pgxpool.Pool, kind string) capture.VerdictPass {
	return func(ctx context.Context) (capture.VerdictClock, error) {
		pass, err := jobs.PassFor(ctx, pool, kind)
		if err != nil {
			return capture.VerdictClock{}, err
		}
		return capture.VerdictClock{Every: pass.Every, Running: pass.Running, NextAt: pass.NextAt}, nil
	}
}
