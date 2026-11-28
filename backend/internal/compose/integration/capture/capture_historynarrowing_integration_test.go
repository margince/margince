// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// rep1Connection is Rep1's connection of one provider, as the seat lists it.
func rep1Connection(t *testing.T, env captureEnv, provider string) capturemod.ConnectionView {
	t.Helper()
	views, err := env.registry.Connections(seatContext(env.e, env.e.Rep1))
	if err != nil {
		t.Fatalf("listing the seat's connections: %v", err)
	}
	for _, v := range views {
		if v.Provider == provider {
			return v
		}
	}
	t.Fatalf("the seat has no %s connection", provider)
	return capturemod.ConnectionView{}
}

// pendingNarrowing is what the seat's gmail connection says is left to narrow.
func pendingNarrowing(t *testing.T, env captureEnv) *capturemod.HistoryNarrowing {
	t.Helper()
	return rep1Connection(t, env, "gmail").HistoryNarrowing
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
	connection := rep1Connection(t, env, "gmail").ID
	named := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM audit_log a,
		jsonb_array_elements_text(a.after->'narrowed_activities') narrowed(id) WHERE a.entity_id = '%s'`, connection))
	distinct := countRows(t, e, fmt.Sprintf(`SELECT count(DISTINCT narrowed.id) FROM audit_log a,
		jsonb_array_elements_text(a.after->'narrowed_activities') narrowed(id) WHERE a.entity_id = '%s'`, connection))
	if named != messages || distinct != messages {
		t.Fatalf("the batch trail names %d imports, %d of them distinct; want each of the %d once", named, distinct, messages)
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

// A removed mailbox's pending request is dropped with a trail, and none of
// its earlier mail is narrowed.
func TestARemovedMailboxDropsItsPendingNarrowing(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	sync(t, customerMail("removed-1@acme.example"))
	connection := rep1Connection(t, env, "gmail").ID
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), capturemod.ProviderMailbox("gmail"), capturemod.PostureHeld, true); err != nil {
		t.Fatalf("narrowing with apply_to_history: %v", err)
	}
	if _, err := e.Owner.Exec(context.Background(), `UPDATE capture_connection SET archived_at = now() WHERE id = $1`, connection); err != nil {
		t.Fatalf("removing the mailbox: %v", err)
	}
	integration.NarrowHistoryToTheEnd(t, e.WS, env.registry)
	if n := openActivityCount(t, e); n != 1 {
		t.Fatalf("%d messages open after the removed mailbox's request ran, want 1: it is dropped", n)
	}
	if got := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM capture_connection
		WHERE id = '%s' AND history_narrowing_held IS NULL`, connection)); got != 1 {
		t.Fatal("the removed mailbox still holds its request")
	}
	if got := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM audit_log
		WHERE entity_id = '%s' AND after->>'dropped_archived' = 'held'`, connection)); got != 1 {
		t.Fatalf("%d audit rows say the request was dropped, want 1", got)
	}
}

// A seat's narrowing reaches only the mail its own mailbox delivered. The
// seat's other mailbox keeps its mail, and the count names only the first.
func TestNarrowingOneMailboxLeavesTheSeatsOtherMailboxAlone(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	const secondAddress = "owner.graph@myco.example"
	second := &mailBatchConnector{accountLabel: secondAddress, name: "graph"}
	env.registry.Register(second)
	secondID, err := env.registry.Connect(humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead}), "graph", connector.Auth("refresh"))
	if err != nil {
		t.Fatalf("connecting the second mailbox: %v", err)
	}
	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), capturemod.ProviderMailbox("graph"), capturemod.PostureShared, false); err != nil {
		t.Fatalf("sharing the second mailbox: %v", err)
	}
	sync(t, customerMail("first-1@acme.example"), customerMail("first-2@acme.example"))
	second.raws = [][]byte{email("anna@acme.example", "Anna Acme", secondAddress, "second-1@acme.example", "")}
	if err := env.registry.SyncOnce(context.Background(), secondID); err != nil {
		t.Fatalf("syncing the second mailbox: %v", err)
	}
	secondMail := activityIDOf(t, e, "second-1@acme.example")

	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), capturemod.ProviderMailbox("gmail"), capturemod.PostureHeld, true); err != nil {
		t.Fatalf("narrowing the first mailbox: %v", err)
	}
	if pending := pendingNarrowing(t, env); pending == nil || pending.Remaining != 2 {
		t.Fatalf("the first mailbox shows %+v left, want its own 2 messages", pending)
	}
	if other := rep1Connection(t, env, "graph").HistoryNarrowing; other != nil {
		t.Fatalf("the second mailbox shows a narrowing it was never asked for: %+v", other)
	}
	integration.NarrowHistoryToTheEnd(t, e.WS, env.registry)
	if got := importPostureOf(t, e, secondMail, e.Rep1); got != capturemod.PostureShared {
		t.Fatalf("the second mailbox's import is %q after narrowing the first, want shared", got)
	}
	if n := openActivityCount(t, e); n != 1 {
		t.Fatalf("%d messages open after the narrowing, want the second mailbox's 1", n)
	}
}

// A batch leaves a trail for every import it narrows. That holds even when
// another mailbox already keeps the activity limited, so its audience stays.
func TestANarrowedImportLeavesATrailWhenTheAudienceStays(t *testing.T) {
	env := newCaptureEnv(t)
	e, sync := env.e, env.sync
	syncOther := secondMailbox(t, e, e.Rep3)
	allowSharedPosture(t, e)
	setPosture(t, env, e.Rep1, capturemod.PostureShared)
	sync(t, customerMailToBoth("trail-1@acme.example"))
	syncOther(t, customerMailToBoth("trail-1@acme.example"))
	activityID := activityIDOf(t, e, "trail-1@acme.example")
	if got := importPostureOf(t, e, activityID, e.Rep1); got != capturemod.PostureShared {
		t.Fatalf("Rep1's import is %q before the change, want shared", got)
	}
	connection := rep1Connection(t, env, "gmail").ID

	if _, err := env.registry.SetMailPosture(
		seatContext(e, e.Rep1), capturemod.ProviderMailbox("gmail"), capturemod.PostureHeld, true); err != nil {
		t.Fatalf("narrowing with apply_to_history: %v", err)
	}
	integration.NarrowHistoryToTheEnd(t, e.WS, env.registry)
	trail := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM audit_log
		WHERE entity_id = '%s' AND after->'narrowed_activities' ? '%s'`, connection, activityID))
	if trail != 1 {
		t.Fatalf("%d audit rows name the narrowed import, want 1", trail)
	}
	steps := countRows(t, e, fmt.Sprintf(`SELECT count(*) FROM audit_log a JOIN event_outbox o
		ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id WHERE a.entity_id = '%s'`, connection))
	if steps < 3 {
		t.Fatalf("%d narrowing steps carry an event, want the request, the batch and the end", steps)
	}
}
