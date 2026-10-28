// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How long each stage of an assembled day takes, kept for /metrics. A slow
// worklist is one route total until it is split by stage. The label is a stage
// or lane name fixed in this package, never anything a request supplies.

import (
	"maps"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// stageBounds are upper bounds in seconds: dense where a healthy lane lands,
// and running to the four-second statement budget a degradable lane may use.
var stageBounds = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8}

var stageTimings = struct {
	mu      sync.Mutex
	byStage map[string]*httpserver.Histogram
}{byStage: map[string]*httpserver.Histogram{}}

// timeStage starts timing one stage; call what it returns when the stage ends.
func timeStage(stage string) func() {
	start := time.Now()
	return func() {
		elapsed := time.Since(start).Seconds()
		stageTimings.mu.Lock()
		defer stageTimings.mu.Unlock()
		h, ok := stageTimings.byStage[stage]
		if !ok {
			h = httpserver.NewHistogram(stageBounds)
			stageTimings.byStage[stage] = h
		}
		h.Observe(elapsed)
	}
}

// StageTimings answers a detached copy of every stage's histogram, by stage.
func StageTimings() map[string]httpserver.Histogram {
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	out := make(map[string]httpserver.Histogram, len(stageTimings.byStage))
	for stage, h := range maps.All(stageTimings.byStage) {
		out[stage] = h.Snapshot()
	}
	return out
}
