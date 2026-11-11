// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package homecache

import (
	"io"
	"sync/atomic"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// served counts, per read, the answers Serve gave from a kept copy (hits) and
// from a fresh read (misses), since process start. A background refresh is
// neither: no reader waited on it.
var served = [...]struct {
	read         Read
	hits, misses atomic.Uint64
}{{read: Worklist}, {read: Magic}}

func countServed(read Read, hit bool) {
	for i := range served {
		if served[i].read != read {
			continue
		}
		if hit {
			served[i].hits.Add(1)
		} else {
			served[i].misses.Add(1)
		}
		return
	}
}

// WriteMetrics renders the hit and miss counts into the /metrics exposition.
func WriteMetrics(w io.Writer) {
	httpserver.WriteLine(w, "# HELP margince_home_cache_reads_total Home reads since process start, by read and whether a kept answer served them.\n")
	httpserver.WriteLine(w, "# TYPE margince_home_cache_reads_total counter\n")
	for i := range served {
		read := httpserver.Label(string(served[i].read))
		httpserver.WriteLine(w, "margince_home_cache_reads_total{read=%s,outcome=%s} %d\n", read, httpserver.Label("hit"), served[i].hits.Load())
		httpserver.WriteLine(w, "margince_home_cache_reads_total{read=%s,outcome=%s} %d\n", read, httpserver.Label("miss"), served[i].misses.Load())
	}
}
