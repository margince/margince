// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
)

// WithAIMetrics sets this role's /metrics AI renderer.
//
// The renderer is ai.WriteProcessMetrics in both binaries, and both wire it
// UNCONDITIONALLY: the counters belong to the process, not to a router, so a
// role that resolved no model path still publishes the (empty) families rather
// than leaving an operator unable to tell "made no calls" from "renders no
// counters". Registering twice is harmless for the same reason — both
// registrations name one process-wide collector.
func WithAIMetrics(write func(io.Writer)) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		s.aiMetrics = write
	}
}

// writeAIMetrics renders this process's AI counters exactly once.
//
// The nil branch survives for the tests that build a Server without the option;
// no binary leaves it unset, so it is not the "AI-less role" posture the
// fleet-wide sections take.
func (s Server) writeAIMetrics(w io.Writer) {
	if s.aiMetrics != nil {
		s.aiMetrics(w)
	}
}

// writeMetricsSections renders every counter family this role wired, in one
// call, because httpserver.Metrics takes ONE extra renderer and its own comment
// says the parameter list has reached the point where a further section should
// not become another argument.
func (s Server) writeMetricsSections(w io.Writer) {
	s.httpMetrics.Write(w)
	s.writeAIMetrics(w)
	s.writeMCPAppMetrics(w)
	s.writeLicenseMetrics(w)
	s.writeCaptureSection(w)
	s.writeAuthzSection(w)
}
