// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package erasedseattraces

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// roomSeller may create a room, invite seats and write in its threads.
var roomSeller = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"deal":      {Create: true, Read: true, Update: true},
		"deal_room": {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// dealRoom is one room on its own deal, written through the dealrooms store.
type dealRoom struct {
	e    *integration.Env
	deal ids.UUID
	id   ids.DealRoomID
}

// buyer is one invited seat, signed in as the public door signs a buyer in.
type buyer struct {
	seat ids.UUID
	sess dealrooms.Session
}

func newRoom(t *testing.T, e *integration.Env) dealRoom {
	t.Helper()
	deal := e.SeedWonDealLinkedTo(t)
	created, err := dealrooms.NewStore(e.DB()).CreateRoom(e.As(e.AdminUser, nil, roomSeller), dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](deal), Title: "Acme Expansion", Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the room: %v", err)
	}
	return dealRoom{e: e, deal: deal, id: ids.From[ids.DealRoomKind](ids.UUID(created.Id))}
}

func (r dealRoom) seller() context.Context { return r.e.As(r.e.AdminUser, nil, roomSeller) }

// invite seats a buyer. A signed-in buyer also holds a session.
func (r dealRoom) invite(t *testing.T, email string, signIn bool) buyer {
	t.Helper()
	rooms := dealrooms.NewStore(r.e.DB())
	invited, err := rooms.InviteParticipant(r.seller(), r.id, dealrooms.InviteInput{
		FullName: "Rita Reviewer", Email: email, Capability: "comment", Source: "manual",
	})
	if err != nil {
		t.Fatalf("inviting %s: %v", email, err)
	}
	b := buyer{seat: ids.UUID(invited.Participant.Id)}
	if !signIn {
		return b
	}
	issued, err := rooms.ExchangeCredential(r.seller(), invited.Credential)
	if err != nil {
		t.Fatalf("exchanging the credential of %s: %v", email, err)
	}
	if b.sess, err = rooms.ResolveSession(r.seller(), issued.Token); err != nil {
		t.Fatalf("resolving the session of %s: %v", email, err)
	}
	return b
}

func (r dealRoom) opens(t *testing.T, b buyer, body string) ids.UUID {
	t.Helper()
	opened, err := dealrooms.NewStore(r.e.DB()).OpenBuyerThread(r.seller(), b.sess, dealrooms.OpenThreadInput{Body: body, Source: "manual"})
	if err != nil {
		t.Fatalf("a buyer opening a thread: %v", err)
	}
	return ids.UUID(opened.Id)
}

func (r dealRoom) buyerReplies(t *testing.T, b buyer, thread ids.UUID, body string) ids.UUID {
	t.Helper()
	replied, err := dealrooms.NewStore(r.e.DB()).ReplyAsBuyer(r.seller(), b.sess, thread, body, "manual", nil)
	if err != nil {
		t.Fatalf("a buyer replying: %v", err)
	}
	return ids.UUID(replied.Comments[len(replied.Comments)-1].Id)
}

func (r dealRoom) sellerReplies(t *testing.T, thread ids.UUID, body string) ids.UUID {
	t.Helper()
	answered, err := dealrooms.NewStore(r.e.DB()).Reply(r.seller(), r.id, thread, body, "manual", nil)
	if err != nil {
		t.Fatalf("the seller replying: %v", err)
	}
	return ids.UUID(answered.Comments[len(answered.Comments)-1].Id)
}

// wipe anonymizes a seat as an erasure did before it purged threads: name,
// address and revocation. With tombstone it also writes the seat's erase row.
func wipe(t *testing.T, e *integration.Env, seat ids.UUID, tombstone bool) {
	t.Helper()
	e.WsExec(t, `UPDATE deal_room_participant SET full_name = 'Erased Subject', email = $2,
		revoked_at = coalesce(revoked_at, now()) WHERE id = $1`, seat, values.ErasedEmail)
	if !tombstone {
		return
	}
	ctx := principal.SystemActing(e.Admin(), "system:erasure-test")
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.AuditWithEvidence(ctx, tx, "erase", "deal_room_participant", seat, nil, nil,
			map[string]any{"reason": "subject request", "cause": "contact_erasure"})
		return err
	}); err != nil {
		t.Fatalf("tombstoning the erased seat: %v", err)
	}
}

// ledger reads the task's run id and counters from the executor's own row.
func ledger(t *testing.T, e *integration.Env) (run ids.UUID, scanned, changed, batches int64, status string) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT run_id, scanned, changed, batches, status FROM data_task_run
			WHERE task_id = $1`, DataTask().ID).Scan(&run, &scanned, &changed, &batches, &status)
	}); err != nil {
		t.Fatalf("reading the task's ledger: %v", err)
	}
	return run, scanned, changed, batches, status
}

func threadsBy(t *testing.T, e *integration.Env, seat ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM deal_room_thread WHERE author_participant_id = $1`, seat)
}

func threadErasures(t *testing.T, e *integration.Env, id ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_thread' AND entity_id = $1
		AND action = 'erase'`, id)
}

func systemCtx(e *integration.Env) context.Context {
	return principal.SystemActing(e.Admin(), "system:datatask-test")
}

// The task deletes what an erased seat wrote, with its audit tombstones.
// A seat that was never erased, or only carries the address, keeps its thread.
func TestTheTaskDeletesWhatAnErasedSeatWroteAndReplaysAsANoOp(t *testing.T) {
	e := integration.Setup(t)
	r := newRoom(t, e)
	erased := r.invite(t, "erased.buyer@acme.test", true)
	kept := r.invite(t, "kept.buyer@acme.test", true)
	lookalike := r.invite(t, "lookalike.buyer@acme.test", true)

	theirs := r.opens(t, erased, "Clause 4 must change before we sign")
	sellerReply := r.sellerReplies(t, theirs, "We can look at clause 4")
	keptReply := r.buyerReplies(t, kept, theirs, "We agree on clause 4")
	sellerThread, err := dealrooms.NewStore(e.DB()).OpenThread(r.seller(), r.id, dealrooms.OpenThreadInput{Body: "Welcome", Source: "manual"})
	if err != nil {
		t.Fatalf("the seller opening a thread: %v", err)
	}
	erasedReply := r.buyerReplies(t, erased, ids.UUID(sellerThread.Id), "Thanks")
	keptThread := r.opens(t, kept, "Who signs on your side?")
	lookalikeThread := r.opens(t, lookalike, "A question from a real address")
	wipe(t, e, erased.seat, true)
	wipe(t, e, lookalike.seat, false)

	ctx := systemCtx(e)
	if n, _, err := DataTask().Steps[0].Estimate(ctx, e.Pool, 100); err != nil || n != 1 {
		t.Fatalf("the estimate counts %d seat(s) (%v), want the one tombstoned seat", n, err)
	}
	e.RunDataTask(ctx, t, DataTask())

	if n := threadsBy(t, e, erased.seat); n != 0 {
		t.Errorf("%d thread(s) still name the erased seat", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE author_participant_id = $1 OR thread_id = $2`,
		erased.seat, theirs); n != 0 {
		t.Errorf("%d comment(s) by the erased seat or in its thread survived", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_session WHERE participant_id = $1`, erased.seat); n != 0 {
		t.Errorf("%d session(s) of the erased seat survived", n)
	}
	for _, survivor := range []ids.UUID{keptThread, lookalikeThread, ids.UUID(sellerThread.Id)} {
		if n := e.WsCount(t, `SELECT count(*) FROM deal_room_thread WHERE id = $1`, survivor); n != 1 {
			t.Errorf("thread %s was deleted, and its author was never erased", survivor)
		}
	}
	run, _, _, _, _ := ledger(t, e)
	for _, deleted := range []struct {
		what, kind string
		id         ids.UUID
	}{
		{"erased buyer's thread", "deal_room_thread", theirs},
		{"seller's reply in it", "deal_room_comment", sellerReply},
		{"kept buyer's reply in it", "deal_room_comment", keptReply},
		{"erased buyer's reply in the seller's thread", "deal_room_comment", erasedReply},
	} {
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = 'erase'
			AND actor_type = 'system' AND actor_id = $3 AND batch_id = $4
			AND evidence->>'reason' = 'subject_data_removed' AND evidence->>'cause' = 'subject_redaction'`,
			deleted.kind, deleted.id, datatask.ActorID(DataTask().ID), run); n != 1 {
			t.Errorf("the %s has %d erase tombstone(s) naming this task and its run, want 1", deleted.what, n)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_thread' AND entity_id = $1
		AND (before IS NOT NULL OR after IS NOT NULL)`, theirs); n != 0 {
		t.Errorf("%d audit row(s) still carry the erased seat's thread", n)
	}
	if n := threadErasures(t, e, keptThread) + threadErasures(t, e, lookalikeThread); n != 0 {
		t.Errorf("threads of seats never erased got %d tombstone(s)", n)
	}

	before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'erase'`)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := DataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100})
		if err == nil && (out.Scanned != 1 || out.Changed != 0 || !out.Done) {
			t.Errorf("a replay scanned %d and changed %d seat(s), want 1 and 0, done", out.Scanned, out.Changed)
		}
		return err
	}); err != nil {
		t.Fatalf("replaying the batch: %v", err)
	}
	if after := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'erase'`); after != before {
		t.Errorf("a replay wrote %d erase tombstone(s)", after-before)
	}
}

// Pages of two seats cross clean seats between dirty ones, and every dirty
// seat is purged.
func TestTheTaskPagesPastCleanSeatsToEveryDirtyOne(t *testing.T) {
	e := integration.Setup(t)
	r := newRoom(t, e)
	var dirty []ids.UUID
	for i, email := range []string{"a@acme.test", "b@acme.test", "c@acme.test", "d@acme.test", "e@acme.test"} {
		isDirty := i%2 == 0
		b := r.invite(t, email, isDirty)
		if isDirty {
			r.opens(t, b, "A question")
			dirty = append(dirty, b.seat)
		}
		wipe(t, e, b.seat, true)
	}
	task := DataTask()
	task.Steps[0].Limit = 2
	e.RunDataTask(systemCtx(e), t, task)

	for _, seat := range dirty {
		if n := threadsBy(t, e, seat); n != 0 {
			t.Errorf("seat %s still opened %d thread(s)", seat, n)
		}
	}
	if _, scanned, changed, batches, status := ledger(t, e); scanned != 5 || changed != 3 || batches != 3 || status != "done" {
		t.Errorf("the run scanned %d, changed %d in %d batches, status %s; want 5, 3, 3, done",
			scanned, changed, batches, status)
	}
}

// A seat holding more rows than one batch may delete is purged over several
// batches, each within the bound.
func TestASeatLargerThanTheBatchBoundIsPurgedOverSeveralBatches(t *testing.T) {
	e := integration.Setup(t)
	r := newRoom(t, e)
	erased := r.invite(t, "busy.buyer@acme.test", true)
	thread := r.opens(t, erased, "First")
	for _, body := range []string{"Second", "Third", "Fourth", "Fifth"} {
		r.sellerReplies(t, thread, body)
	}
	wipe(t, e, erased.seat, true)
	const bound = 2
	task := taskWithBudget(bound)
	traces := e.WsCount(t, `SELECT (SELECT count(*) FROM deal_room_session WHERE participant_id = $1)
		+ (SELECT count(*) FROM deal_room_engagement WHERE participant_id = $1)
		+ (SELECT count(*) FROM deal_room_comment WHERE thread_id = $2) + 1`, erased.seat, thread)

	e.RunDataTask(systemCtx(e), t, task)

	if n := threadsBy(t, e, erased.seat); n != 0 {
		t.Errorf("%d thread(s) of the erased seat survived", n)
	}
	if _, scanned, changed, batches, status := ledger(t, e); status != "done" || batches < int64(traces/bound) {
		t.Errorf("%d traces took %d batch(es), status %s, scanned %d, changed %d; want at least %d batches",
			traces, batches, status, scanned, changed, traces/bound)
	}
}

// A seat whose deal is under a legal hold is left as it is.
func TestASeatOnAHeldDealIsLeftForALaterRun(t *testing.T) {
	e := integration.Setup(t)
	r := newRoom(t, e)
	erased := r.invite(t, "held.buyer@acme.test", true)
	thread := r.opens(t, erased, "Kept by counsel")
	wipe(t, e, erased.seat, true)
	e.WsExec(t, `UPDATE deal SET legal_hold = true WHERE id = $1`, r.deal)

	e.RunDataTask(systemCtx(e), t, DataTask())

	if n := threadsBy(t, e, erased.seat); n != 1 {
		t.Errorf("the held seat has %d thread(s), want its 1", n)
	}
	if n := threadErasures(t, e, thread); n != 0 {
		t.Errorf("the held thread got %d erase tombstone(s)", n)
	}
	if _, scanned, changed, _, status := ledger(t, e); scanned != 1 || changed != 0 || status != "done" {
		t.Errorf("the run scanned %d and changed %d, status %s; want 1, 0, done", scanned, changed, status)
	}
}

// A batch that fails after its purge leaves every row in place, and a cursor
// that is not one the task wrote is refused.
func TestTheTaskRollsBackWithItsBatchAndRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	r := newRoom(t, e)
	erased := r.invite(t, "rollback.buyer@acme.test", true)
	thread := r.opens(t, erased, "Rolled back")
	wipe(t, e, erased.seat, true)
	ctx := systemCtx(e)
	task := DataTask()
	purge := task.Steps[0].Batch
	interrupted := false
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := purge(ctx, tx, in)
		if err != nil {
			return out, err
		}
		interrupted = out.Changed == 1
		return out, errors.New("interrupt after the purge")
	}
	e.RunDataTask(ctx, t, task)
	if !interrupted {
		t.Fatal("the batch failed before its purge, so no rollback was tested")
	}
	if n := threadsBy(t, e, erased.seat); n != 1 {
		t.Errorf("the rolled-back batch left %d thread(s) of the erased seat, want 1", n)
	}
	if n := threadErasures(t, e, thread); n != 0 {
		t.Errorf("the rolled-back batch left %d erase tombstone(s)", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := purge(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
}
