// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The outbound authorization counter family: what this process decided about
// the mail it was asked to send.
//
// Until now /metrics said nothing about it. An operator could see the outbox
// backlog and the job queue, but not that a category had begun refusing every
// message — which from outside looks exactly like nobody sending any.

import (
	"io"
	"sort"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// writeAuthzDecisionMetrics renders one counter per label combination seen.
//
// Sorted, because Prometheus does not care but a human reading a scrape by hand
// does, and ranging a map would reshuffle the block on every request.
func writeAuthzDecisionMetrics(w io.Writer, totals map[consent.DecisionCount]uint64) {
	if len(totals) == 0 {
		// This process has decided nothing yet. Printing zeros for every
		// combination would claim the engine ran and decided nothing either
		// way, which is a different fact from not having run — and the
		// difference is the one an operator is reading this to tell.
		return
	}
	counts := make([]consent.DecisionCount, 0, len(totals))
	for count := range totals {
		counts = append(counts, count)
	}
	sort.Slice(counts, func(i, j int) bool { return authzLabelKey(counts[i]) < authzLabelKey(counts[j]) })
	httpserver.WriteLine(w, "# HELP margince_communication_authz_decisions_total What the outbound engine decided about each recipient at transmit, since process start.\n")
	httpserver.WriteLine(w, "# TYPE margince_communication_authz_decisions_total counter\n")
	for _, count := range counts {
		httpserver.WriteLine(w,
			"margince_communication_authz_decisions_total{verdict=%s,category=%s,mode=%s} %d\n",
			httpserver.Label(string(count.Verdict)), httpserver.Label(string(count.Category)),
			httpserver.Label(string(count.Mode)), totals[count])
	}
}

// authzLabelKey orders one counter line. It joins the three labels and nothing
// else — the recipient is not a field of DecisionCount and must never become
// one.
func authzLabelKey(c consent.DecisionCount) string {
	return string(c.Verdict) + "\x00" + string(c.Category) + "\x00" + string(c.Mode)
}

// writeAuthzSection is the fan-out's entry point.
func (Server) writeAuthzSection(w io.Writer) {
	writeAuthzDecisionMetrics(w, consent.DecisionTotals())
}
