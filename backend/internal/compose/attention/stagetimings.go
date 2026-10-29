// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How long each stage of an assembled day takes, and what its statements cost,
// kept for /metrics. A slow worklist is one route total until it is split by
// stage. The label is a stage or lane name fixed in this package, never
// anything a request supplies.

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// stageBounds are upper bounds in seconds, dense where a healthy lane lands.
// The 0.75s worklist target has its own bucket, and the last runs past the
// four-second budget a degradable lane may use.
var stageBounds = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 0.75, 1, 2, 4, 8}

// OutsideStages is the stage name for the statements a composed read runs
// outside every timed stage, such as BEGIN and the scope checks.
const OutsideStages = "outside_stages"

// StageCost is what one stage's statements cost over every metered read since
// process start. DB is database time as the client saw it. App is the stage's
// own Go time: its duration less its statements, pool waits and inner stages.
type StageCost struct {
	Statements int64
	Failed     int64
	DB         httpserver.Histogram
	App        httpserver.Histogram
}

type stageCost struct {
	statements, failed int64
	db, app            *httpserver.Histogram
}

var stageTimings = struct {
	mu       sync.Mutex
	byStage  map[string]*httpserver.Histogram
	costs    map[string]*stageCost
	poolWait *httpserver.Histogram
}{
	byStage:  map[string]*httpserver.Histogram{},
	costs:    map[string]*stageCost{},
	poolWait: httpserver.NewHistogram(stageBounds),
}

// timeStage starts timing one stage; call what it returns when the stage ends.
// Under a metered read it also records what the stage's own statements cost.
func timeStage(ctx context.Context, stage string) func() {
	start := time.Now()
	trail, metered := ctx.Value(stageTrailKey{}).(*stageTrail)
	var frame *stageFrame
	if metered {
		frame = trail.open()
	}
	return func() {
		elapsed := time.Since(start)
		var own database.StatementCost
		var app time.Duration
		if metered {
			own, app, _ = trail.close(frame, elapsed)
		}
		stageTimings.mu.Lock()
		defer stageTimings.mu.Unlock()
		h, ok := stageTimings.byStage[stage]
		if !ok {
			h = httpserver.NewHistogram(stageBounds)
			stageTimings.byStage[stage] = h
		}
		h.Observe(elapsed.Seconds())
		if metered {
			recordStageCost(stage, own, app)
		}
	}
}

// recordStageCost adds one run of stage; the caller holds stageTimings.mu.
func recordStageCost(stage string, own database.StatementCost, app time.Duration) {
	c, ok := stageTimings.costs[stage]
	if !ok {
		c = &stageCost{db: httpserver.NewHistogram(stageBounds), app: httpserver.NewHistogram(stageBounds)}
		stageTimings.costs[stage] = c
	}
	c.statements += own.Statements
	c.failed += own.Failed
	c.db.Observe(own.DBTime.Seconds())
	c.app.Observe(app.Seconds())
}

type stageTrailKey struct{}

// stageTrail is one composed read's open stages, innermost last, over the
// statement meter its context carries.
type stageTrail struct {
	mu     sync.Mutex
	meter  *database.StatementMeter
	frames []*stageFrame
}

// stageFrame is one open stage: the meter reading it began at, and what the
// stages inside it have taken so far.
type stageFrame struct {
	began     database.StatementCost
	innerTime time.Duration
	innerCost database.StatementCost
}

func (t *stageTrail) open() *stageFrame {
	t.mu.Lock()
	defer t.mu.Unlock()
	frame := &stageFrame{began: t.meter.Cost()}
	t.frames = append(t.frames, frame)
	return frame
}

// close ends frame and charges it to the stage around it. It answers the
// frame's own cost and Go time, and its cost with the inner stages included.
func (t *stageTrail) close(frame *stageFrame, elapsed time.Duration) (database.StatementCost, time.Duration, database.StatementCost) {
	t.mu.Lock()
	defer t.mu.Unlock()
	inclusive := t.meter.Cost().Minus(frame.began)
	own := inclusive.Minus(frame.innerCost)
	app := max(elapsed-frame.innerTime-own.DBTime-own.PoolWait, 0)
	if at := slices.Index(t.frames, frame); at >= 0 {
		t.frames = slices.Delete(t.frames, at, at+1)
		if at > 0 {
			outer := t.frames[at-1]
			outer.innerTime += elapsed
			outer.innerCost = outer.innerCost.Plus(inclusive)
		}
	}
	return own, app, inclusive
}

// meteredStages runs fn with its stages metered, unless an outer read already
// meters them. It records what fn spent outside every stage, and the pool wait
// of the whole read, which is mostly the snapshot's one acquire.
func meteredStages(ctx context.Context, fn func(context.Context) error) error {
	if _, bound := ctx.Value(stageTrailKey{}).(*stageTrail); bound {
		return fn(ctx)
	}
	meter, ok := database.StatementMeterOf(ctx)
	if !ok {
		ctx, meter = database.WithStatementMeter(ctx)
	}
	trail := &stageTrail{meter: meter}
	start := time.Now()
	outer := trail.open()
	err := fn(context.WithValue(ctx, stageTrailKey{}, trail))
	own, app, inclusive := trail.close(outer, time.Since(start))
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	recordStageCost(OutsideStages, own, app)
	stageTimings.poolWait.Observe(inclusive.PoolWait.Seconds())
	return err
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

// StageCosts answers a detached copy of every stage's statement cost, by stage,
// OutsideStages included.
func StageCosts() map[string]StageCost {
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	out := make(map[string]StageCost, len(stageTimings.costs))
	for stage, c := range maps.All(stageTimings.costs) {
		out[stage] = StageCost{Statements: c.statements, Failed: c.failed, DB: c.db.Snapshot(), App: c.app.Snapshot()}
	}
	return out
}

// PoolWaits answers a detached copy of the pool wait per metered read.
func PoolWaits() httpserver.Histogram {
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	return stageTimings.poolWait.Snapshot()
}
