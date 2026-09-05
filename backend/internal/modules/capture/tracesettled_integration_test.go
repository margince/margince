// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A message stops waiting when its sender is judged.
//
// `capture_trace` records what the pipeline did at the time and never changes,
// which is right for the table and wrong for the screen above it: a mailbox
// whose forty-nine strangers all came back noise went on reporting forty-nine
// messages sent for a verdict and no records not made — the exact opposite of
// what happened, on the surface a person opens to find out.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASettledVerdictMovesTheMessageOutOfTheWaitingCount(t *testing.T) {
	ctx, ws, db, store := traceReadWorkspace(t)
	me := ids.NewV7()
	memberCtx := memberContext(ctx, ws, me)

	// Three strangers, three answers, and the same deferred row for each. The
	// pipeline filed all three identically; only the verdicts differ.
	seedRecord(memberCtx, t, db, me, seededRecord{
		SourceID: "v-noise", Sender: "newsletter@client.io",
		Outcome: capture.TraceDeferred, Ledger: true, Verdict: capture.PendingStatusNoise,
	})
	seedRecord(memberCtx, t, db, me, seededRecord{
		SourceID: "v-real", Sender: "buyer@client.io",
		Outcome: capture.TraceDeferred, Ledger: true, Verdict: capture.PendingStatusReal,
	})
	seedRecord(memberCtx, t, db, me, seededRecord{
		SourceID: "v-open", Sender: "unknown@client.io",
		Outcome: capture.TraceDeferred, Ledger: true, Verdict: capture.PendingStatusPending,
	})

	window, err := store.ListMine(memberCtx, nil, nil)
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	for outcome, want := range map[string]int{"suppressed": 1, "captured": 1, "deferred": 1} {
		if got := window.Funnel[outcome]; got != want {
			t.Errorf("funnel[%q] = %d, want %d — a judged sender's message counts under the answer, "+
				"and only an open question still counts as waiting", outcome, got, want)
		}
	}

	// And the rows say what the tiles do, which is the half a reader sees first:
	// a list under a tile reading "no person created" must not be full of rows
	// still labelled as sent for a verdict.
	rows := map[string]int{}
	for _, entry := range window.Entries {
		if entry.Outcome != string(capture.TraceDeferred) {
			t.Errorf("the stored outcome reads %q — the trace is append-only, and a read that "+
				"rewrote it would lose what the pipeline actually did", entry.Outcome)
		}
		rows[entry.OutcomeNow]++
	}
	for outcome, want := range map[string]int{"suppressed": 1, "captured": 1, "deferred": 1} {
		if rows[outcome] != want {
			t.Errorf("%d row(s) read %q, want %d", rows[outcome], outcome, want)
		}
	}
}

// The counters and the rows they head are one answer.
//
// The filter label reads "showing N of M <outcome>", with N counted from the
// rows and M from the tiles. Grouped two different ways those are two claims
// about one window, printed one above the other — which is how the screen came
// to say `SENT FOR A VERDICT 49` over forty-nine rows reading `judged noise`.
func TestTheCountersAgreeWithTheRowsTheyHead(t *testing.T) {
	ctx, ws, db, store := traceReadWorkspace(t)
	me := ids.NewV7()
	memberCtx := memberContext(ctx, ws, me)

	for _, seed := range []struct {
		source, sender, verdict string
		outcome                 capture.TraceOutcome
	}{
		{"a-1", "one@client.io", capture.PendingStatusNoise, capture.TraceDeferred},
		{"a-2", "two@client.io", capture.PendingStatusReal, capture.TraceDeferred},
		{"a-3", "three@client.io", capture.PendingStatusPending, capture.TraceDeferred},
		{"a-4", "four@client.io", capture.PendingStatusRejected, capture.TraceDeferred},
		{"a-5", "five@client.io", "", capture.TraceCaptured},
	} {
		seedRecord(memberCtx, t, db, me, seededRecord{
			SourceID: seed.source, Sender: seed.sender, Outcome: seed.outcome,
			Ledger: seed.verdict != "", Verdict: seed.verdict,
		})
	}

	window, err := store.ListMine(memberCtx, nil, nil)
	if err != nil {
		t.Fatalf("ListMine: %v", err)
	}
	counted := map[string]int{}
	for _, entry := range window.Entries {
		counted[entry.OutcomeNow]++
	}
	for outcome, tile := range window.Funnel {
		if counted[outcome] != tile {
			t.Errorf("the tile for %q says %d and the list under it holds %d rows",
				outcome, tile, counted[outcome])
		}
	}
	for outcome, rows := range counted {
		if window.Funnel[outcome] != rows {
			t.Errorf("%d rows read %q and no tile counts them", rows, outcome)
		}
	}
}

// The schedule reaches the wire, and a deployment that cannot read it says so
// by omission rather than by printing midnight in 1970.
func TestTheAnswerCarriesTheSenderVerdictSchedule(t *testing.T) {
	ctx, ws, db, _ := traceReadWorkspace(t)
	me := ids.NewV7()
	memberCtx := memberContext(ctx, ws, me)
	seedTrace(memberCtx, t, db, me, "clocked", 0)

	next := time.Now().UTC().Add(37 * time.Minute).Truncate(time.Second)
	pass := func(context.Context) (capture.VerdictClock, error) {
		return capture.VerdictClock{Every: time.Hour, Running: true, NextAt: &next}, nil
	}
	body := readActivity(memberCtx, t, capture.NewTraceHandlers(capture.NewTraceStore(db), false, pass))

	if body.SenderVerdict == nil {
		t.Fatal("the answer carries no schedule, so the screen can only say 'waiting'")
	}
	if body.SenderVerdict.EverySeconds != 3600 {
		t.Errorf("the pass runs every %ds, want the declared hour", body.SenderVerdict.EverySeconds)
	}
	if !body.SenderVerdict.Running {
		t.Error("a pass in flight is reported as not running — queued and running are different " +
			"states to somebody watching a counter that has not moved")
	}
	if body.SenderVerdict.NextPassAt == nil || !body.SenderVerdict.NextPassAt.Equal(next) {
		t.Errorf("next_pass_at = %v, want %v", body.SenderVerdict.NextPassAt, next)
	}
}

func TestAnAnswerWithNoQueueReaderCarriesNoSchedule(t *testing.T) {
	ctx, ws, db, _ := traceReadWorkspace(t)
	me := ids.NewV7()
	memberCtx := memberContext(ctx, ws, me)
	seedTrace(memberCtx, t, db, me, "unclocked", 0)

	body := readActivity(memberCtx, t, capture.NewTraceHandlers(capture.NewTraceStore(db), false, nil))
	if body.SenderVerdict != nil {
		t.Error("a deployment that composed no queue reader reported a schedule anyway — " +
			"an invented next pass is worse than none")
	}
}

// readActivity drives the personal read through the transport and decodes it.
func readActivity(ctx context.Context, t *testing.T, handlers capture.TraceHandlers) crmcontracts.CaptureActivityResponse {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/capture/activity", nil).WithContext(ctx)
	handlers.ListMyCaptureActivity(w, r, crmcontracts.ListMyCaptureActivityParams{})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body crmcontracts.CaptureActivityResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding the answer: %v", err)
	}
	return body
}
