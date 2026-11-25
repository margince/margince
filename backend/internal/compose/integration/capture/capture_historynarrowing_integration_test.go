// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
)

// pendingNarrowing is what the seat's own connection says is left to narrow.
func pendingNarrowing(t *testing.T, env captureEnv) *capturemod.HistoryNarrowing {
	t.Helper()
	views, err := env.registry.Connections(seatContext(env.e, env.e.Rep1))
	if err != nil || len(views) != 1 {
		t.Fatalf("listing the seat's connections: %d, %v", len(views), err)
	}
	return views[0].HistoryNarrowing
}

// A history larger than one batch narrows over several runs. Each run commits
// its batch, so mail it has not reached stays open and the connection counts
// down. The next run resumes where one stopped, and every message narrows once.
func TestHistoryNarrowsOneBatchAtATimeAndResumes(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	const messages = 250
	raws := make([][]byte, 0, messages)
	for i := range messages {
		raws = append(raws, customerMail(fmt.Sprintf("batch-%d@acme.example", i)))
	}
	sync(t, raws...)
	if n := openActivityCount(t, e); n != messages {
		t.Fatalf("%d open messages before the change, want %d", n, messages)
	}
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), capturemod.ProviderMailbox("gmail"), capturemod.PostureHeld, true); err != nil {
		t.Fatalf("narrowing with apply_to_history: %v", err)
	}
	seq := countRows(t, e, `SELECT coalesce(max(seq), 0) FROM event_outbox`)

	if !integration.NarrowHistoryBatch(t, e.WS, env.registry) {
		t.Fatal("the first run found no narrowing pending")
	}
	open := openActivityCount(t, e)
	if open == 0 || open >= messages {
		t.Fatalf("%d of %d messages still open after one batch; a batch narrows some and leaves the rest", open, messages)
	}
	if pending := pendingNarrowing(t, env); pending == nil || pending.Remaining != open {
		t.Fatalf("the connection shows %+v left, want the %d still open", pending, open)
	}

	integration.NarrowHistoryToTheEnd(t, e.WS, env.registry)
	if n := openActivityCount(t, e); n != 0 {
		t.Fatalf("%d messages still open after the narrowing ended", n)
	}
	if pending := pendingNarrowing(t, env); pending != nil {
		t.Fatalf("the connection still shows %+v after the narrowing ended", pending)
	}
	twice := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM (SELECT envelope->>'entity_id' FROM event_outbox
		WHERE seq > %d AND envelope->>'type' = 'activity.updated' GROUP BY 1 HAVING count(*) > 1) repeated`, seq))
	moved := countRows(t, e, fmt.Sprintf(`SELECT count(DISTINCT envelope->>'entity_id') FROM event_outbox
		WHERE seq > %d AND envelope->>'type' = 'activity.updated'`, seq))
	if twice != 0 || moved != messages {
		t.Fatalf("%d messages announced as moved, %d of them more than once; want each of the %d once", moved, twice, messages)
	}
}

// A narrowing still pending keeps its strictest request. A later request to
// narrow to classified does not loosen a pending hold, and a move towards
// shared does not cancel it.
func TestAPendingHeldNarrowingIsNeitherLoosenedNorDropped(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	sync(t, customerMail("pending-1@acme.example"))
	activityID := oneActivityID(t, e)

	for _, posture := range []string{capturemod.PostureHeld, capturemod.PostureClassified, capturemod.PostureShared} {
		if _, err := env.registry.SetMailPosture(
			seatContext(e, e.Rep1), capturemod.ProviderMailbox("gmail"), posture, true); err != nil {
			t.Fatalf("setting %s with apply_to_history: %v", posture, err)
		}
	}
	if pending := pendingNarrowing(t, env); pending == nil || pending.Posture != capturemod.PostureHeld {
		t.Fatalf("the pending narrowing is %+v, want held", pending)
	}
	integration.NarrowHistoryToTheEnd(t, e.WS, env.registry)
	if got := importPostureOf(t, e, activityID, e.Rep1); got != capturemod.PostureHeld {
		t.Fatalf("the import row is %q after the narrowing, want held", got)
	}
}
