// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The one product reading the Brief's own tables cannot give: whether anybody
// looked at it.
//
// Driven through the real HTTP stack rather than the engine, because the claim
// is about what a REP's morning does — GET /v1/brief is the open, POST is not —
// and a test calling LatestRun directly would prove the function emits while
// saying nothing about which route a rep's browser takes.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// briefOpenEnvelope is the part of a staged brief.opened this suite reads.
type briefOpenEnvelope struct {
	Type   string `json:"type"`
	Entity *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	} `json:"entity"`
	Trace struct {
		AuditLogID string `json:"audit_log_id"`
	} `json:"trace"`
	Payload struct {
		LocalDay string `json:"local_day"`
		Items    int    `json:"items"`
		Unread   int    `json:"unread"`
	} `json:"payload"`
}

// openEvents returns every brief.opened staged so far, oldest first.
func openEvents(t *testing.T, e *apptest.AppEnv) []briefOpenEnvelope {
	t.Helper()
	var out []briefOpenEnvelope
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'brief.opened'
			 ORDER BY seq`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				return err
			}
			var env briefOpenEnvelope
			if err := json.Unmarshal(raw, &env); err != nil {
				return err
			}
			out = append(out, env)
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

// A read of the Brief says so on the bus, and says what was there to read.
//
// The counts are asserted against the run the SAME call returned rather than
// against a literal, so a fixture that ranks a different number of deals moves
// both together and the test keeps meaning what it says.
func TestReadingTheBriefEmitsTheOpen(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	createDealClosingThisWeek(t, e, stages, "Closing this week")
	createDealClosingThisWeek(t, e, stages, "Also closing")

	var generated briefResponse
	if status := e.Call(t, "POST", "/v1/brief", nil, nil, &generated); status != http.StatusCreated {
		t.Fatalf("POST /v1/brief = %d, want 201", status)
	}

	// Generating is not opening. The night assembles a run nobody has looked
	// at yet, and counting that as an open would hide the exact case this
	// event exists to make visible: a queue that was ranked and never read.
	if staged := openEvents(t, e); len(staged) != 0 {
		t.Fatalf("POST /v1/brief staged %d brief.opened event(s), want none — "+
			"assembling a run is not a rep reading it", len(staged))
	}

	var read briefResponse
	if status := e.Call(t, "GET", "/v1/brief", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /v1/brief = %d, want 200", status)
	}

	staged := openEvents(t, e)
	if len(staged) != 1 {
		t.Fatalf("the read staged %d brief.opened event(s), want exactly one", len(staged))
	}
	got := staged[0]

	if got.Payload.Items != len(read.Items) {
		t.Errorf("the event says %d items and the rep was shown %d — the count must be "+
			"what the page rendered, not what the night ranked",
			got.Payload.Items, len(read.Items))
	}
	// Nothing has been marked yet, so every item the rep sees is unread. This
	// is what separates a first open from a return visit.
	if got.Payload.Unread != len(read.Items) {
		t.Errorf("unread = %d on a run nobody has marked, want %d", got.Payload.Unread, len(read.Items))
	}
	if got.Payload.LocalDay == "" {
		t.Error("the event carries no local_day, so a consumer cannot tell today's " +
			"first open from a re-open of yesterday's run")
	}

	// The trace link is what makes the event attributable, and Validate refuses
	// an envelope without one. Asserted rather than assumed: a staged event
	// with no ledger row behind it is an open nobody can attribute.
	if got.Trace.AuditLogID == "" {
		t.Error("the event carries no trace link to its ledger row")
	}
	assertSystemLogRow(t, e, got.Trace.AuditLogID)
}

// The event names no entity, and that is what keeps it internal.
//
// Every catalogued type outside the entity-less pipeline class is selectable by
// a webhook subscription — publicevents_test.go derives that set exactly as
// webhooks.validateEventTypes does. So an entity ref here would turn "this rep
// opened their Brief at 06:42" into a delivery an outside consumer may ask for.
// The gate proves the type is in the right class; this proves the ENVELOPE the
// emit actually stages agrees with it.
func TestTheBriefOpenNamesNoEntity(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	createDealClosingThisWeek(t, e, stages, "Closing this week")

	var generated briefResponse
	if status := e.Call(t, "POST", "/v1/brief", nil, nil, &generated); status != http.StatusCreated {
		t.Fatalf("POST /v1/brief = %d, want 201", status)
	}
	if status := e.Call(t, "GET", "/v1/brief", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("GET /v1/brief = %d, want 200", status)
	}

	staged := openEvents(t, e)
	if len(staged) != 1 {
		t.Fatalf("staged %d brief.opened event(s), want one", len(staged))
	}
	if ent := staged[0].Entity; ent != nil && ent.Type != "" {
		t.Errorf("the open names entity %s/%s — an entity ref makes the type "+
			"subscribable, and a rep's reading habits are not a webhook",
			ent.Type, ent.ID)
	}
}

// Reading twice says so twice.
//
// The obvious wrong implementation records the first open and then treats the
// run as seen — which would answer "did they ever look" while losing "do they
// come back", and the second is the one a product decision turns on.
func TestEveryReadOfTheBriefIsItsOwnOpen(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	createDealClosingThisWeek(t, e, stages, "Closing this week")

	var generated briefResponse
	if status := e.Call(t, "POST", "/v1/brief", nil, nil, &generated); status != http.StatusCreated {
		t.Fatalf("POST /v1/brief = %d, want 201", status)
	}
	for i := range 2 {
		if status := e.Call(t, "GET", "/v1/brief", nil, nil, nil); status != http.StatusOK {
			t.Fatalf("read %d: GET /v1/brief = %d, want 200", i+1, status)
		}
	}

	if staged := openEvents(t, e); len(staged) != 2 {
		t.Fatalf("two reads staged %d open(s), want 2 — a run is not 'seen once'", len(staged))
	}
}

// assertSystemLogRow proves the trace points at a real system_log row.
//
// system_log rather than audit_log, and that is the ruling rather than an
// incidental choice: reading a Brief mutates no record, and audit_log is the
// record-mutation spine. A telemetry row filed there would put a page view in
// the same ledger a reviewer reads to answer "what changed about this deal".
func assertSystemLogRow(t *testing.T, e *apptest.AppEnv, id string) {
	t.Helper()
	var action string
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT action FROM system_log WHERE id = $1`, id).Scan(&action)
	}); err != nil {
		t.Fatalf("the trace id names no system_log row (%v) — if it was filed in "+
			"audit_log instead, a page view is sitting in the record-mutation spine", err)
	}
	if action != "brief.opened" {
		t.Errorf("the ledger row's action is %q, want brief.opened", action)
	}
}
