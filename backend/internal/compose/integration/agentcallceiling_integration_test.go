// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The shared app harness composes agentvolume.Unmetered(). A meter that cannot
// reach its counter fails closed, and would refuse every agent read in suites
// testing something else. This file builds its own Server with a live ceiling,
// or nothing would prove the REST door enforces it.
import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// boundedApp is the app under a live call ceiling. It returns the meter too, so
// a test can put the window into the state it is about.
func boundedApp(t *testing.T, slug string, limit int) (*apptest.AppEnv, *agentvolume.Meter) {
	t.Helper()
	meter := agentvolume.New(redistest.Client(t), agentvolume.Limits{Calls: limit}, time.Hour)
	e := apptest.SetupAppWithOptions(t, compose.WithAgentVolume(meter))
	apptest.BootstrapWorkspaceSession(t, e, "Call Ceiling", slug+"@fable.test", "Admin")
	return e, meter
}

// asPassport builds the context the meter counts one passport against — the
// same binding the gate resolves from a presented Bearer.
func asPassport(t *testing.T, e *apptest.AppEnv, passport ids.UUID) context.Context {
	t.Helper()
	var ws ids.UUID
	if err := e.Owner.QueryRow(t.Context(), `SELECT id FROM workspace LIMIT 1`).Scan(&ws); err != nil {
		t.Fatalf("reading the workspace id: %v", err)
	}
	ctx := principal.WithWorkspaceID(t.Context(), ws)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + passport.String(), PassportID: passport,
	})
}

// spendWindow charges the meter against one passport, as the MCP door would.
func spendWindow(t *testing.T, e *apptest.AppEnv, meter *agentvolume.Meter, passport ids.UUID, calls int) {
	t.Helper()
	if err := meter.Consume(asPassport(t, e, passport), agentvolume.Calls, calls); err != nil {
		t.Fatalf("spending the window: %v", err)
	}
}

// callsCharged is what the window has actually observed against one passport.
func callsCharged(t *testing.T, e *apptest.AppEnv, meter *agentvolume.Meter, passport ids.UUID) int {
	t.Helper()
	return meter.Read(asPassport(t, e, passport), agentvolume.Calls).Observed
}

// A passport that has spent its window is refused on the REST door too. Before
// this, /v1 sat outside the control entirely: a credential that spent its calls
// through MCP could keep reading the same records over REST.
func TestASpentWindowRefusesTheSamePassportOnTheRestDoor(t *testing.T) {
	e, meter := boundedApp(t, "call-ceiling-rest", 100)
	bearer, passport := passportWithID(t, e, "reading agent", "read")
	seedContacts(t, e, 2)

	// Served under the ceiling first, so the refusal below is the ceiling firing
	// rather than the route being closed to agents for some other reason.
	if status := e.Call(t, "GET", "/v1/contacts", nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("an agent read under the ceiling → %d, want 200", status)
	}

	spendWindow(t, e, meter, passport, 100)

	// The VOLUME-BUDGET response specifically: a 403 or a 500 would also fail an
	// is-not-200 check while meaning something entirely different.
	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "GET", "/v1/contacts", nil, bearer, &problem)

	if status != http.StatusTooManyRequests {
		t.Errorf("a passport that spent its window → %d, want 429; /v1 is outside the ceiling", status)
	}
	if problem.Code != "rate_limited" {
		t.Errorf("the refusal carried code %q, want \"rate_limited\" — a caller branches on the code, not the prose", problem.Code)
	}
}

// A HUMAN session is never touched by it, however much any agent has called. A
// busy agent must not be able to lock its own operator out of the product it is
// acting inside.
func TestAHumanSessionIsUnaffectedByASpentAgentWindow(t *testing.T) {
	e, meter := boundedApp(t, "call-ceiling-human", 100)
	_, passport := passportWithID(t, e, "reading agent", "read")
	seedContacts(t, e, 2)

	spendWindow(t, e, meter, passport, 500)

	if status := e.Call(t, "GET", "/v1/contacts", nil, nil, nil); status != http.StatusOK {
		t.Errorf("a human read → %d after an agent spent its window; humans are outside this ceiling", status)
	}
}

// A door that refuses on a counter also pays into it. One request is one call
// however many records it returns. A page of four and a single record cost the
// same, which is what a reader of the usage meter expects.
func TestARestReadSpendsOneCallWhateverItReturns(t *testing.T) {
	e, meter := boundedApp(t, "call-ceiling-charge", 100)
	bearer, passport := passportWithID(t, e, "reading agent", "read")
	seedContacts(t, e, 3)

	var created struct {
		ID ids.UUID `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"source":    "manual",
		"full_name": "Single Read",
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("seeding the contact → %d", status)
	}

	if status := e.Call(t, "GET", "/v1/contacts", nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("an agent list read → %d, want 200", status)
	}
	if charged := callsCharged(t, e, meter, passport); charged != 1 {
		t.Errorf("a page of 4 records charged %d calls, want 1", charged)
	}

	if status := e.Call(t, "GET", "/v1/contacts/"+created.ID.String(), nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("an agent single read → %d, want 200", status)
	}
	if charged := callsCharged(t, e, meter, passport); charged != 2 {
		t.Errorf("a list and a single read charged %d calls, want 2", charged)
	}
}

// The derived obligation: every counter the admission gate can refuse on has a
// charge point on the same door.
//
// platform/auth refuses on Calls and on agentvolume.CounterFor(spec), which is
// Calls, Writes or Egress. Egress is not driven here: CounterFor picks between
// Writes and Egress at the same charge point (ChargeEffect), so the Writes row
// proves the call site. Reaching Egress also needs an approval staged and
// redeemed, since every egress tool with a REST twin is confirm-first.
//
// Each counter is measured across the one request that must charge it, because a
// total taken at the end cannot say which door paid. A read must move Calls by
// one, so a second charge on a read-only route is caught.
func TestEveryCounterTheRestDoorRefusesOnIsChargedOnIt(t *testing.T) {
	e, meter := boundedApp(t, "call-ceiling-census", 1000)
	bearer, passport := passportWithID(t, e, "counting agent", "read", "write")
	seedContacts(t, e, 2)
	ctx := asPassport(t, e, passport)

	advance := func(during func()) map[agentvolume.Counter]int {
		counters := []agentvolume.Counter{agentvolume.Writes, agentvolume.Calls}
		was := map[agentvolume.Counter]int{}
		for _, c := range counters {
			was[c] = meter.Read(ctx, c).Observed
		}
		during()
		moved := map[agentvolume.Counter]int{}
		for _, c := range counters {
			moved[c] = meter.Read(ctx, c).Observed - was[c]
		}
		return moved
	}

	onRead := advance(func() {
		if status := e.Call(t, "GET", "/v1/contacts", nil, bearer, nil); status != http.StatusOK {
			t.Fatalf("the agent read → %d, want 200", status)
		}
	})
	onWrite := advance(func() {
		if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
			"source":    "manual",
			"full_name": "Charged By The Gate",
		}, bearer, nil); status != http.StatusCreated {
			t.Fatalf("the agent write → %d, want 201", status)
		}
	})

	for _, owed := range []struct {
		counter agentvolume.Counter
		moved   int
		want    int
		door    string
	}{
		{agentvolume.Calls, onRead[agentvolume.Calls], 1, "a GET is one call"},
		{agentvolume.Writes, onRead[agentvolume.Writes], 0, "a GET changes nothing"},
		{agentvolume.Writes, onWrite[agentvolume.Writes], 1, "a POST mutates"},
		{agentvolume.Calls, onWrite[agentvolume.Calls], 1, "a POST is one call"},
	} {
		if owed.moved != owed.want {
			t.Errorf("the REST door charged %s by %d, want %d: %s", owed.counter, owed.moved, owed.want, owed.door)
		}
	}
}

// REST requests ALONE can spend the window, reads included, and once they have
// the same passport's writes are refused too. A passport that only reads over
// /v1 must approach the same ceiling as one calling over /mcp.
func TestRestCallsAloneCanSpendTheWindow(t *testing.T) {
	e, _ := boundedApp(t, "call-ceiling-selfspend", 4)
	bearer, _ := passportWithID(t, e, "reading agent", "read", "write")
	seedContacts(t, e, 3)

	for i := range 4 {
		if status := e.Call(t, "GET", "/v1/contacts", nil, bearer, nil); status != http.StatusOK {
			t.Fatalf("agent read %d → %d, want 200", i+1, status)
		}
	}

	if status := e.Call(t, "GET", "/v1/contacts", nil, bearer, nil); status != http.StatusTooManyRequests {
		t.Errorf("the fifth agent read → %d, want 429; reading over /v1 never spends the window it is refused on", status)
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"source":    "manual",
		"full_name": "Refused",
	}, bearer, nil); status != http.StatusTooManyRequests {
		t.Errorf("a write after the reads spent the ceiling → %d, want 429; the ceiling suspends every verb", status)
	}
}

// passportWithID mints a passport and returns both the header an agent presents
// and the id the meter counts against.
func passportWithID(t *testing.T, e *apptest.AppEnv, label string, scopes ...string) (map[string]string, ids.UUID) {
	t.Helper()
	var minted struct {
		ID    ids.UUID `json:"passport_id"`
		Token string   `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": label, "scopes": scopes,
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport %q → %d", label, status)
	}
	if minted.Token == "" || minted.ID.IsZero() {
		t.Fatalf("passport %q minted without a token or an id", label)
	}
	return map[string]string{"Authorization": "Bearer " + minted.Token}, minted.ID
}

// seedContacts gives the list something to return, so the admitted read is a real
// one rather than an empty page.
func seedContacts(t *testing.T, e *apptest.AppEnv, n int) {
	t.Helper()
	for i := range n {
		if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
			"source":    "manual",
			"full_name": "Metered Contact " + string(rune('A'+i)),
		}, nil, nil); status != http.StatusCreated {
			t.Fatalf("seeding contact %d → %d", i, status)
		}
	}
}
