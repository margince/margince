// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// How long each stage of an assembled day takes, and what its pgx calls cost,
// kept for /metrics by composed read and stage. A slow worklist is one route
// total until it is split by stage. Both labels are names fixed in this
// package, never anything a request supplies.

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

// The composed reads that meter their stages, one per entry point that opens
// a snapshot. A read nested in another is charged to the outer one.
const (
	ReadWorklist       = "worklist"
	ReadAttention      = "attention"
	ReadTeamExceptions = "team_exceptions"
	// ReadUnmetered is a stage timed outside every composed read.
	ReadUnmetered = "unmetered"
)

// OutsideStages is the stage name for the statements a composed read runs
// outside every timed stage, such as BEGIN and the scope checks.
const OutsideStages = "outside_stages"

// StageKey names one series: the composed read and its stage.
type StageKey struct{ Read, Stage string }

// StageCost is what one stage's pgx calls cost over every metered read since
// process start, each histogram one sample per run. App is the stage's time
// less its DB, its PoolWait and its inner stages. Clipped counts runs where
// those added up to more than the stage's time, so App read zero.
type StageCost struct {
	Statements int64
	Failed     int64
	Clipped    int64
	DB         httpserver.Histogram
	PoolWait   httpserver.Histogram
	App        httpserver.Histogram
}

type stageCost struct {
	statements, failed, clipped int64
	db, poolWait, app           *httpserver.Histogram
}

var stageTimings = struct {
	mu       sync.Mutex
	byStage  map[StageKey]*httpserver.Histogram
	costs    map[StageKey]*stageCost
	poolWait map[string]*httpserver.Histogram
}{
	byStage:  map[StageKey]*httpserver.Histogram{},
	costs:    map[StageKey]*stageCost{},
	poolWait: map[string]*httpserver.Histogram{},
}

// timeStage starts timing one stage; call what it returns when the stage ends.
// Under a metered read it also records what the stage's own pgx calls cost.
func timeStage(ctx context.Context, stage string) func() {
	start := time.Now()
	trail, metered := ctx.Value(stageTrailKey{}).(*stageTrail)
	read := ReadUnmetered
	var frame *stageFrame
	if metered {
		read, frame = trail.read, trail.open()
	}
	return func() {
		elapsed := time.Since(start)
		var spent stageSpend
		if metered {
			spent, _ = trail.close(frame, elapsed)
		}
		stageTimings.mu.Lock()
		defer stageTimings.mu.Unlock()
		key := StageKey{Read: read, Stage: stage}
		h, ok := stageTimings.byStage[key]
		if !ok {
			h = httpserver.NewHistogram(stageBounds)
			stageTimings.byStage[key] = h
		}
		h.Observe(elapsed.Seconds())
		if metered {
			recordStageCost(key, spent)
		}
	}
}

// stageSpend is one run of a stage: its own cost and the time left after it.
type stageSpend struct {
	own     database.StatementCost
	app     time.Duration
	clipped bool
}

// recordStageCost adds one run; the caller holds stageTimings.mu.
func recordStageCost(key StageKey, spent stageSpend) {
	c, ok := stageTimings.costs[key]
	if !ok {
		c = &stageCost{
			db:       httpserver.NewHistogram(stageBounds),
			poolWait: httpserver.NewHistogram(stageBounds),
			app:      httpserver.NewHistogram(stageBounds),
		}
		stageTimings.costs[key] = c
	}
	c.statements += spent.own.Statements
	c.failed += spent.own.Failed
	if spent.clipped {
		c.clipped++
	}
	c.db.Observe(spent.own.DBTime.Seconds())
	c.poolWait.Observe(spent.own.PoolWait.Seconds())
	c.app.Observe(spent.app.Seconds())
}

type stageTrailKey struct{}

// stageTrail is one composed read's open stages, innermost last, over the
// statement meter its context carries.
type stageTrail struct {
	mu     sync.Mutex
	read   string
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
// frame's own spend, and its cost with the inner stages included.
func (t *stageTrail) close(frame *stageFrame, elapsed time.Duration) (stageSpend, database.StatementCost) {
	t.mu.Lock()
	defer t.mu.Unlock()
	inclusive := t.meter.Cost().Minus(frame.began)
	own := inclusive.Minus(frame.innerCost)
	// A read's stages and pgx calls run one after another, so these fit in
	// elapsed. Calls that overlapped would not, and the clip is counted.
	app := elapsed - frame.innerTime - own.DBTime - own.PoolWait
	spent := stageSpend{own: own, app: max(app, 0), clipped: app < 0}
	if at := slices.Index(t.frames, frame); at >= 0 {
		t.frames = slices.Delete(t.frames, at, at+1)
		if at > 0 {
			outer := t.frames[at-1]
			outer.innerTime += elapsed
			outer.innerCost = outer.innerCost.Plus(inclusive)
		}
	}
	return spent, inclusive
}

// meteredStages runs fn with its stages metered under read, unless an outer
// read already meters them. It records what fn spent outside every stage, and
// the pool wait of the whole read.
func meteredStages(ctx context.Context, read string, fn func(context.Context) error) error {
	if _, bound := ctx.Value(stageTrailKey{}).(*stageTrail); bound {
		return fn(ctx)
	}
	meter, ok := database.StatementMeterOf(ctx)
	if !ok {
		ctx, meter = database.WithStatementMeter(ctx)
	}
	trail := &stageTrail{read: read, meter: meter}
	start := time.Now()
	outer := trail.open()
	err := fn(context.WithValue(ctx, stageTrailKey{}, trail))
	spent, inclusive := trail.close(outer, time.Since(start))
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	recordStageCost(StageKey{Read: read, Stage: OutsideStages}, spent)
	h, seen := stageTimings.poolWait[read]
	if !seen {
		h = httpserver.NewHistogram(stageBounds)
		stageTimings.poolWait[read] = h
	}
	h.Observe(inclusive.PoolWait.Seconds())
	return err
}

// StageTimings answers a detached copy of every stage's histogram.
func StageTimings() map[StageKey]httpserver.Histogram {
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	out := make(map[StageKey]httpserver.Histogram, len(stageTimings.byStage))
	for key, h := range maps.All(stageTimings.byStage) {
		out[key] = h.Snapshot()
	}
	return out
}

// StageCosts answers a detached copy of every stage's cost, OutsideStages
// included.
func StageCosts() map[StageKey]StageCost {
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	out := make(map[StageKey]StageCost, len(stageTimings.costs))
	for key, c := range maps.All(stageTimings.costs) {
		out[key] = StageCost{
			Statements: c.statements, Failed: c.failed, Clipped: c.clipped,
			DB: c.db.Snapshot(), PoolWait: c.poolWait.Snapshot(), App: c.app.Snapshot(),
		}
	}
	return out
}

// PoolWaits answers a detached copy of the pool wait per metered read, by read.
func PoolWaits() map[string]httpserver.Histogram {
	stageTimings.mu.Lock()
	defer stageTimings.mu.Unlock()
	out := make(map[string]httpserver.Histogram, len(stageTimings.poolWait))
	for read, h := range maps.All(stageTimings.poolWait) {
		out[read] = h.Snapshot()
	}
	return out
}
