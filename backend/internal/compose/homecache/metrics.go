// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"io"
	"sync/atomic"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// outcome is how Serve answered a reader: from a kept copy within its Life, from
// one past it while it was read again, or fresh.
type outcome int

const (
	hit outcome = iota
	stale
	miss
)

var outcomeLabels = [...]string{hit: "hit", stale: "stale", miss: "miss"}

// served counts, per read, the answers Serve gave by outcome since process
// start. A background refresh is none: no reader waited on it.
var served = [...]struct {
	read  Read
	count [len(outcomeLabels)]atomic.Uint64
}{{read: Worklist}, {read: Magic}}

func countServed(read Read, how outcome) {
	for i := range served {
		if served[i].read == read {
			served[i].count[how].Add(1)
			return
		}
	}
}

// WriteMetrics renders the hit and miss counts into the /metrics exposition.
func WriteMetrics(w io.Writer) {
	httpserver.WriteLine(w, "# HELP margince_home_cache_reads_total Home reads since process start, by read and whether a kept answer served them, past its life or not.\n")
	httpserver.WriteLine(w, "# TYPE margince_home_cache_reads_total counter\n")
	for i := range served {
		read := httpserver.Label(string(served[i].read))
		for how, label := range outcomeLabels {
			httpserver.WriteLine(w, "margince_home_cache_reads_total{read=%s,outcome=%s} %d\n",
				read, httpserver.Label(label), served[i].count[how].Load())
		}
	}
}
