// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether this role's agent-read control can answer — the gauge an operator
// alerts on instead of a readiness probe.
//
// The bound fails CLOSED: with its counter store unreachable every governed
// counter reports its threshold passed and the whole agent surface refuses. On
// the default api that fault is visible by accident, because the inline relay
// probes the same Redis and /readyz takes the pod out. A SPLIT role — api with
// the relay off, the worker separate — probes nothing and reports healthy while
// every agent read refuses.
//
// Not readiness, and that is the decision rather than the cheap way out.
// Readiness is per-POD: a probe would drain the pod for human traffic too, over
// a fault no human request can meet. The alert on this gauge carries the
// sentence that makes it actionable — agent reads are refusing, human traffic
// is unaffected — and docs/reference/operator-signals.md is where it lives.

import (
	"io"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// writeAgentVolumeSection renders what this role can say about its bound.
//
// TWO gauges rather than one tri-state, because they answer two questions an
// operator asks separately: whether this role bounds agent reads at all, and
// whether the bound can answer. A single "healthy" gauge would make an
// unbounded role — one that declared it serves no agent surface — look broken
// for ever, and a role that is supposed to bound and does not indistinguishable
// from one that never claimed to.
func (s Server) writeAgentVolumeSection(w io.Writer) {
	reach := s.volumeMeter.Answerable()
	httpserver.WriteLine(w, "# HELP margince_agent_volume_bound Whether this role composed a bound on agent reads (MCP-SESS-READS): 1 when it did, 0 when it declared it serves no bounded agent surface.\n")
	httpserver.WriteLine(w, "# TYPE margince_agent_volume_bound gauge\n")
	httpserver.WriteLine(w, "margince_agent_volume_bound %d\n", boolGauge(reach.Bound))
	httpserver.WriteLine(w, "# HELP margince_agent_volume_answerable Whether the bound could read its counter store on its last attempt: 0 means the bound is failing closed and every agent read on this role is refusing. Human traffic is unaffected.\n")
	httpserver.WriteLine(w, "# TYPE margince_agent_volume_answerable gauge\n")
	httpserver.WriteLine(w, "margince_agent_volume_answerable %d\n", boolGauge(reach.Reachable))
}

// boolGauge renders a yes/no as Prometheus writes one.
func boolGauge(yes bool) int {
	if yes {
		return 1
	}
	return 0
}
