// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The worklist family assembles its answer in ONE transaction.
//
// This is the gate margince#4912 asks for, and it is a fitness function rather
// than a number: it measures the surface against a plain route measured in the
// same run, so it cannot drift as the rest of the request grows a connection or
// loses one.
//
// WHAT IT PROTECTS. Each of these pages composes its answer from six fixed
// lanes and sixteen optional ones, and every lane reader opens its own
// transaction by default. Read that way the family measured 33 transactions per
// request for /v1/worklist and 54 for /v1/worklist/exceptions — roughly three
// round trips each, in series, which was most of the 400 ms p95 that surface
// answered in. It also meant one page could contradict itself, because twenty
// lanes read from twenty instants.
//
// WHY A BUDGET AND NOT AN EXACT COUNT. The floor is not 1: a request resolves
// its session and its principal before the page is assembled, and the walk it
// freezes commits on its own terms (walk.go says why). Those are real and they
// are not lanes. What the budget refuses is a count that scales with LANES —
// a seventeenth lane added the old way spends another transaction and fails
// here, which is exactly the regression this exists to catch.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// laneBudget is how many transactions a composed page may spend beyond what a
// trivial authenticated route spends.
//
// Generous on purpose: the assembled day itself is one, the walk it freezes is
// a second, and the margin absorbs the request machinery either side without
// this test becoming a tripwire for changes it is not about. It is still an
// order of magnitude below the 33 and 54 these routes measured when every lane
// opened its own.
const laneBudget = 8

// acquisitionsDuring answers how many pooled connections a call took.
//
// AcquireCount rather than pg_stat_database: every transaction this product
// opens begins by acquiring from the pool, so the delta is the transaction
// count — and it is read in-process, so there is no counter-flush window to
// sleep through and nothing for an unrelated committer to add to.
func acquisitionsDuring(t *testing.T, e *apptest.AppEnv, call func()) int64 {
	t.Helper()
	before := e.Pool.Stat().AcquireCount()
	call()
	return e.Pool.Stat().AcquireCount() - before
}

func TestTheWorklistFamilyAssemblesInOneTransaction(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// THE BRIEF RUN IS SEEDED FIRST, and this is not setup detail — it is the
	// difference between this test measuring something and measuring nothing.
	//
	// The brief lane is assembleDay's first, and its read WRITES: it resurfaces
	// expired snoozes and records the open. Against a bootstrapped workspace
	// with no brief_run it returns ErrNotFound before reaching any of that, so
	// every assertion below would pass over a lane that never ran — and the
	// write-under-read this whole change has to get right would be untested.
	// Seeded through POST /v1/brief — the route the night's own worker reaches
	// (briefjobs.go), rather than an INSERT here that could drift from what it
	// actually assembles. GET only reads, and 404s when no run exists.
	// 201 assembled one, 200 found the night's already there. Either leaves a
	// run for the brief lane to read, which is the whole requirement here.
	var brief map[string]any
	if status := e.Call(t, "POST", "/v1/brief", nil, nil, &brief); status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("POST /v1/brief → %d, want 201 or 200 — without a run the brief "+
			"lane short-circuits on ErrNotFound and this test measures a page "+
			"that never assembled the lane it exists to measure", status)
	}

	// The reference reading, taken from the same pool in the same run: a route
	// that answers from one statement. Whatever a request costs before it
	// reaches a page's lanes, it costs here too.
	var me map[string]any
	baseline := acquisitionsDuring(t, e, func() {
		if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
			t.Fatalf("GET /v1/me → %d, want 200", status)
		}
	})

	for _, route := range []string{"/v1/worklist", "/v1/attention", "/v1/worklist/exceptions"} {
		var page map[string]any
		spent := acquisitionsDuring(t, e, func() {
			// 200 or 403: whether this seat may read the page is another
			// test's subject, and a refusal still assembles nothing, so a
			// refused route would pass this vacuously. Guarded below.
			status := e.Call(t, "GET", route, nil, nil, &page)
			if status != http.StatusOK {
				t.Fatalf("GET %s → %d, want 200 — a refused page assembles no lanes, "+
					"so this measurement would pass having measured nothing", route, status)
			}
		})
		if over := spent - baseline; over > laneBudget {
			t.Errorf("GET %s took %d pooled connections against /v1/me's %d — %d more, over the %d budget.\n\n"+
				"That is the shape margince#4912 fixed: a transaction per lane reader rather than one "+
				"snapshot for the page. A lane added without going through the feed's Snapshots seam "+
				"spends its own, and the cost is paid in series on the first screen of every rep's day.",
				route, spent, baseline, over, laneBudget)
		}
	}
}
