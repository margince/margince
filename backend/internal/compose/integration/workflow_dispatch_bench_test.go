// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

// AC-W2 (interfaces.md §5 / PERF-R7 + GATE-CORE-6): trigger→dispatch p95 must
// hold under 200ms on the seeded dataset — v1 dropped this acceptance
// criterion silently.
//
// It carries the `bench` tag, so no merge gate runs it — both golangci passes
// still type-check it, which is the only thing standing between a by-hand lane
// and rot. NOT `make vet`: that pass is untagged by design and says so, so it
// cannot see this file at all. `make bench-dispatch` is how it runs.
//
// It rode the standing integration lane until it stopped being a signal there.
// Its p95 is the eleventh-largest of two hundred samples taken on a runner
// where six integration shards share one Postgres, and a stall on that runner
// is not a rare event: the health check that finally moved it recorded
// p50=13.19ms — sixteen times inside the budget — against p95=201.63ms, and
// filed `main` as broken over the 1.63ms. Raising the sample count from 25 had
// already been tried and bought one run. What a contended runner measures is
// the runner, and the same conclusion sent PERF-3/PERF-7, PERF-1/PERF-4,
// CAP-PARAM-1 and MOBILE-AC-2 to their own by-hand targets before it.
//
// NOTHING REPLACES IT IN THE STANDING LANE, and that is stated here rather than
// papered over. PERF-3/PERF-7 could leave because a deterministic test already
// held its regression (the seq_scan count in lastactivity_integration_test.go);
// AC-W2 has no such sibling. Three instruments were tried and measured before
// this file was written — a `seq_scan` delta, a rows-read ceiling, and both over
// a history seeded under the firing handler. None of them can fail: the engine
// dispatches over the APP POOL, pg_stat_force_next_flush() flushes only the
// calling backend, and a reader on the owner connection therefore sees counters
// the dispatch has not reported yet. A gate that cannot fail is worse than an
// absent one, so the gap is an issue with a design question in it rather than a
// green test with nothing behind it.
//
// What this measures, precisely: engine.HandleEvent(ctx, env) is the exact call
// the cg:workflows redis subscriber makes once it decodes an envelope off the
// bus (cmd/worker's runSubscriber wraps this same method in events.Dedupe). It
// is synchronous — Match every registered handler, Plan the matching ones,
// claim the (handler, key) row, Apply the effect — all DB-bound work whose cost
// is the variable, dataset-dependent quantity the 200ms budget actually
// targets. Two timestamps bracket exactly that call, nothing else.
//
// What this deliberately excludes, and why: the cg:workflows redis
// relay+subscriber transport (event_outbox → platform/events.Relay → Redis
// stream → a Subscriber's XREADGROUP) is NOT measured here. compose (this
// package's home) cannot import the redis client by architectural design —
// .go-arch-lint.yml grants compose only [pgx, chi, oapi, river], deliberately
// no redis, because the bus is a cmd/platform concern compose never touches
// directly. That transport hop is small and near-constant (a stream append + a
// consumer-group read), and it is separately proven by internal/platform/events'
// own bus integration test — this is the honest scope of what AC-W2 asks
// compose to gate, not a workaround for an import compose is not allowed to
// make.
//
// A warmup batch runs first and is discarded: connection setup and query-plan
// caching are fixed costs a firing dispatch never pays in steady state, and the
// search perfbench harness discards them the same way (PERF-3/PERF-7).

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/search"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// workflowDispatchBudget is AC-W2's own budget — a calibration starting value
// in the same spirit as search.Perf3Budget/Perf7Budget: changing it is a noted
// budget revision, never a silent bump.
const workflowDispatchBudget = 200 * time.Millisecond

// The sample size is what makes the p95 below a measurement rather than a coin
// toss, and it is arithmetic rather than taste.
//
// search.MeasureQuery ranks a percentile by index, so p95 IS one sample: at 25
// it is the second-largest, which means a single stall decides the verdict. At
// 200 the same rank is the eleventh-largest, so ten samples have to be slow
// before the gate trips. That is the difference between "this dispatch got
// unlucky once" and "this dispatch has a tail", and only the second is the
// regression AC-W2 exists to catch. On the contended lane runner even that was
// not enough, which is why this suite is by hand; on a machine a person is
// watching, it is what makes the number worth reading.
const dispatchSampleSize = 200

func TestWorkflowTriggerToDispatchP95HoldsOnTheSeededDataset(t *testing.T) {
	e := Setup(t)
	seedAllStarterAutomations(t, e)
	engine := compose.NewWorkflowEngine(e.DB())

	// Warmup: discarded, per the file doc comment's fixed-cost note.
	for _, leadID := range seedTriggerLeads(t, e, dispatchWarmupSamples) {
		if err := engine.HandleEvent(context.Background(), leadCreatedEnvelope(leadID)); err != nil {
			t.Fatalf("warmup dispatch for lead %s: %v", leadID, err)
		}
	}

	sampleIDs := seedTriggerLeads(t, e, dispatchSampleSize)
	durations := make([]time.Duration, 0, len(sampleIDs))
	for _, leadID := range sampleIDs {
		env := leadCreatedEnvelope(leadID)
		start := time.Now()
		if err := engine.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("dispatching lead %s: %v", leadID, err)
		}
		durations = append(durations, time.Since(start))
	}

	stats, err := search.MeasureQuery("workflow_dispatch", workflowDispatchBudget, durations)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("AC-W2 trigger→dispatch [engine.HandleEvent, transport excluded — see file doc comment]: p50=%s p95=%s p99=%s (budget %s, %d samples)",
		stats.P50, stats.P95, stats.P99, stats.Budget, stats.Samples)
	if stats.P95 > workflowDispatchBudget {
		t.Fatalf("AC-W2: trigger→dispatch p95 = %s over the %s budget (%d samples)", stats.P95, workflowDispatchBudget, stats.Samples)
	}
}

// dispatchWarmupSamples is discarded, per the file doc comment's fixed-cost
// note.
const dispatchWarmupSamples = 5

// leadCreatedEnvelope builds the delivery engine.HandleEvent would receive for
// leadID's creation — the same envelope shape workflow_integration_test.go's
// own direct-dispatch suites already build by hand for this package
// (TestWorkflowRouteLeadAssignsExactlyOnce), reused here rather than
// re-derived.
func leadCreatedEnvelope(leadID ids.UUID) kevents.Envelope {
	return kevents.Envelope{
		EventID:    ids.NewV7(),
		Type:       leadCreatedEventType,
		OccurredAt: time.Now().UTC(),
		Entity:     kevents.EntityRef{Type: "lead", ID: leadID},
	}
}

// seedTriggerLeads creates n leads through the real domain write path
// (people.Store.CreateLead), which stages "lead.created" via storekit.Emit in
// the same transaction as the row — an honest trigger entity for
// Match/Plan/Apply to work against, never a hand-built stand-in.
func seedTriggerLeads(t *testing.T, e *Env, n int) (leadIDs []ids.UUID) {
	t.Helper()
	for i := range n {
		name := fmt.Sprintf("Dispatch Perf Lead %d", i)
		lead, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{FullName: &name, Source: "manual"})
		if err != nil {
			t.Fatalf("seeding trigger lead %d: %v", i, err)
		}
		leadIDs = append(leadIDs, ids.UUID(lead.Id))
	}
	return leadIDs
}
