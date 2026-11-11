// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package erasedseattraces

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
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

// room is one deal room with two buyer seats and the threads they wrote in.
type room struct {
	id                        ids.DealRoomID
	erased, kept              ids.UUID
	erasedThread, keptThread  ids.UUID
	sellerThread, sellerReply ids.UUID
	erasedReplyInSellerThread ids.UUID
}

// seedRoom writes every row through the dealrooms store. It then anonymizes
// one seat as an erasure did before it purged threads: name, address and
// revocation only.
func seedRoom(t *testing.T, e *integration.Env) room {
	t.Helper()
	seller := e.As(e.AdminUser, nil, roomSeller)
	rooms := dealrooms.NewStore(e.DB())
	created, err := rooms.CreateRoom(seller, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](e.SeedWonDealLinkedTo(t)), Title: "Acme Expansion", Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the room: %v", err)
	}
	r := room{id: ids.From[ids.DealRoomKind](ids.UUID(created.Id))}
	erasedSess, erased := signedInSeat(t, e, r.id, "erased.buyer@acme.test")
	keptSess, kept := signedInSeat(t, e, r.id, "kept.buyer@acme.test")
	r.erased, r.kept = erased, kept

	theirs, err := rooms.OpenBuyerThread(seller, erasedSess, dealrooms.OpenThreadInput{
		Body: "Clause 4 must change before we sign", Source: "manual"})
	if err != nil {
		t.Fatalf("the erased buyer opening a thread: %v", err)
	}
	r.erasedThread = ids.UUID(theirs.Id)
	answered, err := rooms.Reply(seller, r.id, r.erasedThread, "We can look at clause 4", "manual", nil)
	if err != nil {
		t.Fatalf("the seller answering: %v", err)
	}
	r.sellerReply = ids.UUID(answered.Comments[len(answered.Comments)-1].Id)
	ours, err := rooms.OpenThread(seller, r.id, dealrooms.OpenThreadInput{Body: "Welcome to the room", Source: "manual"})
	if err != nil {
		t.Fatalf("the seller opening a thread: %v", err)
	}
	r.sellerThread = ids.UUID(ours.Id)
	replied, err := rooms.ReplyAsBuyer(seller, erasedSess, r.sellerThread, "Thanks", "manual", nil)
	if err != nil {
		t.Fatalf("the erased buyer replying in the seller's thread: %v", err)
	}
	r.erasedReplyInSellerThread = ids.UUID(replied.Comments[len(replied.Comments)-1].Id)
	keeps, err := rooms.OpenBuyerThread(seller, keptSess, dealrooms.OpenThreadInput{Body: "Who signs on your side?", Source: "manual"})
	if err != nil {
		t.Fatalf("the kept buyer opening a thread: %v", err)
	}
	r.keptThread = ids.UUID(keeps.Id)

	e.WsExec(t, `UPDATE deal_room_participant SET full_name = 'Erased Subject', email = $2,
		revoked_at = now() WHERE id = $1`, r.erased, values.ErasedEmail)
	return r
}

// signedInSeat invites a buyer and signs them in as the public door does.
func signedInSeat(t *testing.T, e *integration.Env, roomID ids.DealRoomID, email string) (dealrooms.Session, ids.UUID) {
	t.Helper()
	seller := e.As(e.AdminUser, nil, roomSeller)
	rooms := dealrooms.NewStore(e.DB())
	invited, err := rooms.InviteParticipant(seller, roomID, dealrooms.InviteInput{
		FullName: "Rita Reviewer", Email: email, Capability: "comment", Source: "manual",
	})
	if err != nil {
		t.Fatalf("inviting %s: %v", email, err)
	}
	issued, err := rooms.ExchangeCredential(seller, invited.Credential)
	if err != nil {
		t.Fatalf("exchanging the credential of %s: %v", email, err)
	}
	sess, err := rooms.ResolveSession(seller, issued.Token)
	if err != nil {
		t.Fatalf("resolving the session of %s: %v", email, err)
	}
	return sess, ids.UUID(invited.Participant.Id)
}

func runTask(ctx context.Context, t *testing.T, e *integration.Env, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func erasures(t *testing.T, e *integration.Env, entity string, id ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2
		AND action = 'erase'`, entity, id)
}

func estimateLeft(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	count, _, err := DataTask().Steps[0].Estimate(ctx, e.Pool, 100)
	if err != nil {
		t.Fatalf("estimating the task: %v", err)
	}
	return count
}

// The task deletes what an anonymized seat wrote and tombstones it as the
// live erasure does. A seat that was never erased keeps its thread.
func TestTheTaskDeletesWhatAnErasedSeatWroteAndReplaysAsANoOp(t *testing.T) {
	e := integration.Setup(t)
	r := seedRoom(t, e)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if n := estimateLeft(ctx, t, e); n != 1 {
		t.Fatalf("the estimate counts %d seat(s) with traces, want the one erased seat", n)
	}

	runTask(ctx, t, e, DataTask())

	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_thread WHERE author_participant_id = $1`, r.erased); n != 0 {
		t.Errorf("%d thread(s) still name the erased seat", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE author_participant_id = $1
		OR thread_id = $2`, r.erased, r.erasedThread); n != 0 {
		t.Errorf("%d comment(s) by the erased seat or in its thread survived", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_session WHERE participant_id = $1`, r.erased); n != 0 {
		t.Errorf("%d session(s) of the erased seat survived", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_thread WHERE id = ANY($1)`,
		[]ids.UUID{r.keptThread, r.sellerThread}); n != 2 {
		t.Errorf("%d of the kept buyer's and the seller's threads survived, want 2", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_comment WHERE thread_id = $1`, r.sellerThread); n != 1 {
		t.Errorf("the seller's thread holds %d comment(s), want only the seller's opening one", n)
	}
	for entity, id := range map[string]ids.UUID{
		"deal_room_thread": r.erasedThread, "deal_room_comment": r.sellerReply,
	} {
		if n := erasures(t, e, entity, id); n != 1 {
			t.Errorf("%s %s has %d erase tombstone(s), want 1", entity, id, n)
		}
	}
	if n := erasures(t, e, "deal_room_comment", r.erasedReplyInSellerThread); n != 1 {
		t.Errorf("the erased buyer's reply has %d erase tombstone(s), want 1", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_room_thread' AND entity_id = $1
		AND (before IS NOT NULL OR after IS NOT NULL)`, r.erasedThread); n != 0 {
		t.Errorf("%d audit row(s) still carry the erased seat's thread", n)
	}
	if n := erasures(t, e, "deal_room_thread", r.keptThread); n != 0 {
		t.Errorf("the kept buyer's thread was tombstoned %d time(s)", n)
	}
	if n := estimateLeft(ctx, t, e); n != 0 {
		t.Errorf("the estimate still counts %d seat(s) after the run", n)
	}

	before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'erase'`)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := DataTask().Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 100})
		if err == nil && (out.Scanned != 1 || out.Changed != 0) {
			t.Errorf("a replay scanned %d and changed %d seat(s), want 1 and 0", out.Scanned, out.Changed)
		}
		return err
	}); err != nil {
		t.Fatalf("replaying the batch: %v", err)
	}
	if after := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'erase'`); after != before {
		t.Errorf("a replay wrote %d erase tombstone(s)", after-before)
	}
}

// A batch that fails after its purge leaves every row in place, and a cursor
// that is not an id is refused.
func TestTheTaskRollsBackWithItsBatchAndRefusesAMalformedCursor(t *testing.T) {
	e := integration.Setup(t)
	r := seedRoom(t, e)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
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
	runTask(ctx, t, e, task)
	if !interrupted {
		t.Fatal("the batch failed before its purge, so no rollback was tested")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal_room_thread WHERE author_participant_id = $1`, r.erased); n != 1 {
		t.Errorf("the rolled-back batch left %d thread(s) of the erased seat, want 1", n)
	}
	if n := erasures(t, e, "deal_room_thread", r.erasedThread); n != 0 {
		t.Errorf("the rolled-back batch left %d erase tombstone(s)", n)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := purge(ctx, tx, datatask.BatchIn{Cursor: []byte(`"bad cursor"`), Limit: 2})
		return err
	}); err == nil {
		t.Fatal("accepted a malformed cursor")
	}
}
