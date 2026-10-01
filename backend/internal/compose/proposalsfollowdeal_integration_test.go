// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A pending proposal follows its deal to the new owner, over real migrated
// Postgres. Every card here is staged by the real writer — the nightly sweep or
// the transcript reader — and every reassignment goes through a real deal owner
// writer, because a card seeded by hand would prove the move against a row
// production does not write.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// reassign moves the deal's owner through the ordinary update path, as the
// admin seat: a manager handing a deal over.
func (e *reconcileEnv) reassign(t *testing.T, dealID ids.UUID, to ids.UUID) {
	t.Helper()
	next := ids.From[ids.UserKind](to)
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](dealID),
		deals.UpdateDealInput{OwnerID: &next}); err != nil {
		t.Fatalf("reassigning the deal: %v", err)
	}
}

// seatOf reads who an approval is staged for; nil is nobody.
func (e *reconcileEnv) seatOf(t *testing.T, id ids.ApprovalID) *ids.UUID {
	t.Helper()
	var seat *ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT on_behalf_of FROM approval WHERE id = $1`, id).Scan(&seat); err != nil {
		t.Fatalf("reading the approval's seat: %v", err)
	}
	return seat
}

func (e *reconcileEnv) statusOf(t *testing.T, id ids.ApprovalID) string {
	t.Helper()
	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM approval WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("reading the approval's status: %v", err)
	}
	return status
}

// seatMoves counts the audit rows recording this approval's seat moving to one
// member.
func (e *reconcileEnv) seatMoves(t *testing.T, id ids.ApprovalID, to ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'approval' AND entity_id = $1 AND after->>'on_behalf_of' = $2`,
		id, to.String()).Scan(&n); err != nil {
		t.Fatalf("counting the seat moves: %v", err)
	}
	return n
}

func wantSeat(t *testing.T, what string, got *ids.UUID, want *ids.UUID) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("%s is filed for %s, want nobody", what, *got)
	case want != nil && got == nil:
		t.Errorf("%s is filed for nobody, want %s", what, *want)
	case want != nil && *got != *want:
		t.Errorf("%s is filed for %s, want %s", what, *got, *want)
	}
}

// The card moves with the deal: the new owner sees and decides it, the old one
// no longer can, and the audit trail says the seat moved and who moved it.
func TestAFollowUpFollowsTheDealToItsNewOwner(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Handed over", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	id, _ := e.followUpApproval(t, deal)
	wantSeat(t, "the staged card", e.seatOf(t, id), &e.Rep1)

	e.reassign(t, deal, e.Rep2)

	wantSeat(t, "the card after the reassignment", e.seatOf(t, id), &e.Rep2)
	if got := e.seatMoves(t, id, e.Rep2); got != 1 {
		t.Errorf("%d audit rows record the card moving to the new owner, want 1", got)
	}
	var actor string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT actor_id FROM audit_log
		 WHERE entity_type = 'approval' AND entity_id = $1 AND after->>'on_behalf_of' = $2`,
		id, e.Rep2.String()).Scan(&actor); err != nil {
		t.Fatalf("reading the seat move's audit row: %v", err)
	}
	if want := "human:" + e.AdminUser.String(); actor != want {
		t.Errorf("the seat move is attributed to %q, want the member who reassigned the deal %q", actor, want)
	}

	previous := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(previous, id, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("the previous owner deciding → %v, want ErrNotFound: the card is no longer theirs", err)
	}
	current := e.As(e.Rep2, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(current, id, true, nil); err != nil {
		t.Fatalf("the new owner deciding: %v", err)
	}
	if got, _, _, _ := e.dealTasks(t, deal); got != 1 {
		t.Errorf("the new owner's yes created %d tasks, want 1", got)
	}
}

// The move is part of the owner change: a reassignment that fails after the
// move takes the move back with it.
func TestAReassignmentThatFailsLeavesTheCardWithThePreviousOwner(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Reassignment rolled back", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	id, _ := e.followUpApproval(t, deal)

	admin := e.Admin()
	active, err := e.Deals.ActiveDealColumns(admin)
	if err != nil {
		t.Fatal(err)
	}
	forced := errors.New("forced failure after the owner change")
	next := ids.From[ids.UserKind](e.Rep2)
	err = e.DB().Tx(admin, func(tx pgx.Tx) error {
		if _, err := e.Deals.UpdateDealTx(admin, tx, ids.From[ids.DealKind](deal),
			deals.UpdateDealInput{OwnerID: &next}, active); err != nil {
			return err
		}
		var seat *ids.UUID
		if err := tx.QueryRow(admin, `SELECT on_behalf_of FROM approval WHERE id = $1`, id).Scan(&seat); err != nil {
			return err
		}
		// Inside the writer's transaction the card has already moved, which
		// is what makes the rollback below mean something.
		wantSeat(t, "the card inside the reassigning transaction", seat, &e.Rep2)
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatalf("the reassigning transaction → %v, want the forced failure", err)
	}
	wantSeat(t, "the card after the rolled-back reassignment", e.seatOf(t, id), &e.Rep1)
	var owner ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT owner_id FROM deal WHERE id = $1`, deal).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	if owner != e.Rep1 {
		t.Errorf("the deal is owned by %s after the rollback, want %s", owner, e.Rep1)
	}
	if got := e.seatMoves(t, id, e.Rep2); got != 0 {
		t.Errorf("%d audit rows record a seat move that was rolled back, want 0", got)
	}
}

// What a human answered, and what the window closed on, belong to the seat they
// were asked of. Only a PENDING card follows the deal.
func TestADecidedOrExpiredCardStaysWithTheSeatItWasAskedOf(t *testing.T) {
	e := setupReconcile(t)
	rejected := e.SeedDeal(t, "Declined before the handover", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, rejected, "call", "Discovery call", 3)
	lapsed := e.SeedDeal(t, "Lapsed before the handover", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, lapsed, "call", "Scoping call", 3)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	rejectedID, _ := e.followUpApproval(t, rejected)
	lapsedID, _ := e.followUpApproval(t, lapsed)

	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(rep1, rejectedID, false, nil); err != nil {
		t.Fatalf("rejecting: %v", err)
	}
	e.WsExec(t, `UPDATE approval SET expires_at = now() - interval '1 minute' WHERE id = $1`, lapsedID)
	sweep := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:approval-expiry")
	if _, err := e.svc.ExpireDue(sweep); err != nil {
		t.Fatalf("running the expiry sweep: %v", err)
	}
	if got := e.statusOf(t, lapsedID); got != "expired" {
		t.Fatalf("the lapsed card is %q, want expired — the case below would prove nothing", got)
	}

	e.reassign(t, rejected, e.Rep2)
	e.reassign(t, lapsed, e.Rep2)

	for name, id := range map[string]ids.ApprovalID{"the rejected card": rejectedID, "the expired card": lapsedID} {
		wantSeat(t, name, e.seatOf(t, id), &e.Rep1)
		if got := e.seatMoves(t, id, e.Rep2); got != 0 {
			t.Errorf("%s: %d seat moves recorded, want 0", name, got)
		}
	}
}

// A drafted reply is text composed under the previous owner's authority, to go
// out from their mailbox. It is withdrawn rather than handed on, and the next
// sweep drafts one for the new owner.
func TestADraftForThePreviousOwnerIsWithdrawnAndRedraftedForTheNewOne(t *testing.T) {
	e := setupReconcile(t)
	e.grantOwner(t, e.Rep2, reconcileOwnerPolicy)
	deal := e.SeedDeal(t, "Drafted, then handed over", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, deal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	old, _ := e.heldDraftFor(t, deal)
	wantSeat(t, "the first draft", e.seatOf(t, old), &e.Rep1)

	e.reassign(t, deal, e.Rep2)

	if got := e.statusOf(t, old); got != "expired" {
		t.Errorf("the previous owner's draft is %q after the handover, want expired (withdrawn)", got)
	}
	wantSeat(t, "the withdrawn draft", e.seatOf(t, old), &e.Rep1)
	var withdrawals int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'approval' AND entity_id = $1
		   AND after->>'status' = 'expired' AND evidence->>'reason' LIKE 'the deal changed owner%'`,
		old).Scan(&withdrawals); err != nil {
		t.Fatal(err)
	}
	if withdrawals != 1 {
		t.Errorf("%d audit rows record the withdrawal and its reason, want 1", withdrawals)
	}

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	fresh, _ := e.heldDraftFor(t, deal)
	if fresh == old {
		t.Fatal("the sweep did not draft a new reply after the withdrawal")
	}
	wantSeat(t, "the redrafted reply", e.seatOf(t, fresh), &e.Rep2)
}

// readTranscriptFor drives one whole reading the way the worker does, asked
// for by one member, and returns the pending card it staged on the meeting.
func (e *reconcileEnv) readTranscriptFor(
	t *testing.T, requester ids.UUID, activity ids.ActivityID, brain completer,
) ids.ApprovalID {
	t.Helper()
	requestedBy := "human:" + requester.String()
	started, _, err := e.Activities.StartTranscriptReadQueued(e.Admin(), activity, requestedBy, nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	quiet := slog.New(slog.NewTextHandler(os.Stderr, nil))
	proposer := NewTranscriptProposer(e.Pool, brain, e.svc, time.Now, quiet)
	worker := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), requestedBy, started.ID)
	if err := proposer.Read(worker, e.Activities, started.ID, activity); err != nil {
		t.Fatalf("reading the transcript: %v", err)
	}
	var id ids.ApprovalID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT id FROM approval WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`,
		TranscriptProposalKind, activity.UUID).Scan(&id); err != nil {
		t.Fatalf("no transcript card staged on the meeting: %v", err)
	}
	return id
}

// logTranscript files a meeting transcript linked to the given deals.
func (e *reconcileEnv) logTranscript(t *testing.T, subject string, dealIDs ...ids.UUID) ids.ActivityID {
	t.Helper()
	body, source := transcriptBody, "transcript"
	in := activities.LogActivityInput{Kind: "meeting", Subject: &subject, Body: &body, SourceSystem: &source, Source: "manual"}
	for _, deal := range dealIDs {
		in.Links = append(in.Links, activities.ActivityLinkInput{EntityType: "deal", EntityID: deal})
	}
	logged, _, err := e.Activities.LogActivity(e.Admin(), in)
	if err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(logged.Id))
}

// oneShotBrain answers like cannedBrain and runs during() on its first call —
// the moment the reading is out with the model and holds no lock.
type oneShotBrain struct {
	cannedBrain
	once   *sync.Once
	during func()
}

func (b oneShotBrain) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	b.once.Do(b.during)
	return b.cannedBrain.Complete(ctx, req)
}

// A transcript card is staged for the owner of the deal it lands on, whoever
// asked for the reading, and every card on the deal follows it — including one
// a colleague asked for. A card about another meeting is not touched.
func TestEveryTranscriptCardOnTheDealFollowsIt(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Meeting on this deal", e.pipeline, e.open, &e.Rep1)
	partner := e.SeedDeal(t, "The other side's deal", e.pipeline, e.open, &e.Rep3)
	brain := cannedBrain{reply: groundedReply(t, 3, 0.95)}

	owners := e.readTranscriptFor(t, e.Rep2, e.logTranscript(t, "Rollout call", deal), brain)
	// Two owners, so the reading stays with the colleague who asked for it:
	// the shape of a card on the deal that names a seat other than its owner.
	colleagues := e.readTranscriptFor(t, e.Rep2, e.logTranscript(t, "Joint call", deal, partner), brain)
	unrelated := e.readTranscriptFor(t, e.Rep1, e.logTranscript(t, "Unrelated call"), brain)
	wantSeat(t, "the card a colleague asked for on the owner's deal", e.seatOf(t, owners), &e.Rep1)
	wantSeat(t, "the card on two owners' deals", e.seatOf(t, colleagues), &e.Rep2)

	e.reassign(t, deal, e.AdminUser)

	wantSeat(t, "the owner's transcript card", e.seatOf(t, owners), &e.AdminUser)
	wantSeat(t, "the colleague's transcript card", e.seatOf(t, colleagues), &e.AdminUser)
	wantSeat(t, "the card about another meeting", e.seatOf(t, unrelated), &e.Rep1)
	if got := e.seatMoves(t, owners, e.AdminUser); got != 1 {
		t.Errorf("%d audit rows record the transcript card moving, want 1", got)
	}
}

// The transcript staging race: the deal changes hands while the reading is out
// with the model. The hand-over finds nothing to move, so the reader itself has
// to stage for the owner the deal has when it stages.
func TestATranscriptReadWhileTheDealChangesHandsIsFiledForTheNewOwner(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Handed over mid-reading", e.pipeline, e.open, &e.Rep1)
	meeting := e.logTranscript(t, "Rollout call", deal)
	brain := oneShotBrain{
		cannedBrain: cannedBrain{reply: groundedReply(t, 3, 0.95)},
		once:        &sync.Once{},
		during:      func() { e.reassign(t, deal, e.Rep2) },
	}

	card := e.readTranscriptFor(t, e.Rep1, meeting, brain)

	wantSeat(t, "the card staged after the handover", e.seatOf(t, card), &e.Rep2)
}

// Every writer of deal.owner_id hands the pending card on. The list is the
// writers the deals module has: the update path (set and clear, reached by the
// HTTP handler, the agent tools and automation through the provider, and the
// extraction accept) and the claim.
func TestEveryDealOwnerWriterHandsOnThePendingProposals(t *testing.T) {
	e := setupReconcile(t)
	provider := deals.NewProvider(e.DB(), installseam.Deals())
	writers := []struct {
		name   string
		owner  *ids.UUID
		write  func(deal ids.UUID) error
		wantTo *ids.UUID
	}{
		{"update sets a new owner", &e.Rep1, func(deal ids.UUID) error {
			next := ids.From[ids.UserKind](e.Rep2)
			_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &next})
			return err
		}, &e.Rep2},
		{"update clears the owner", &e.Rep1, func(deal ids.UUID) error {
			_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{Clear: []string{"owner_id"}})
			return err
		}, nil},
		{"automation reassigns through the provider", &e.Rep1, func(deal ids.UUID) error {
			_, err := provider.Update(e.Admin(), datasource.UpdateInput{
				Ref:   datasource.EntityRef{Type: datasource.EntityDeal, ID: deal},
				Patch: json.RawMessage(fmt.Sprintf(`{"owner_id":%q}`, e.Rep2)),
			})
			return err
		}, &e.Rep2},
		{"an admin claims a colleague's deal", &e.Rep1, func(deal ids.UUID) error {
			_, err := e.Deals.ClaimDeal(e.Admin(), ids.From[ids.DealKind](deal), nil)
			return err
		}, &e.AdminUser},
		{"an admin claims an unowned deal", nil, func(deal ids.UUID) error {
			_, err := e.Deals.ClaimDeal(e.Admin(), ids.From[ids.DealKind](deal), nil)
			return err
		}, &e.AdminUser},
	}
	for _, w := range writers {
		t.Run(w.name, func(t *testing.T) {
			deal := e.SeedDeal(t, w.name, e.pipeline, e.open, &e.Rep1)
			if w.owner == nil {
				e.WsExec(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, deal)
			}
			e.seedInteraction(t, deal, "call", "Call before "+w.name, 3)
			if err := e.reconcile(); err != nil {
				t.Fatal(err)
			}
			id, _ := e.followUpApproval(t, deal)
			wantSeat(t, "the staged card", e.seatOf(t, id), w.owner)
			if err := w.write(deal); err != nil {
				t.Fatalf("the owner write: %v", err)
			}
			wantSeat(t, "the card after "+w.name, e.seatOf(t, id), w.wantTo)
		})
	}
}

// The staging race: a reassignment in flight when the sweep reads the owner.
//
// The reassignment holds the deal row, uncommitted. The sweep must wait for it
// rather than read the owner it is replacing — so the test proves the sweep
// BLOCKED, then lets the reassignment commit, and the card names the new owner.
// A sweep that read the owner in a transaction of its own would finish without
// blocking and file the card for the seat the deal was leaving.
func TestTheSweepWaitsForAReassignmentInFlightAndFilesForTheNewOwner(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Reassigned mid-sweep", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)

	admin := e.Admin()
	active, err := e.Deals.ActiveDealColumns(admin)
	if err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	reassigned := make(chan error, 1)
	go func() {
		next := ids.From[ids.UserKind](e.Rep2)
		reassigned <- e.DB().Tx(admin, func(tx pgx.Tx) error {
			if _, err := e.Deals.UpdateDealTx(admin, tx, ids.From[ids.DealKind](deal),
				deals.UpdateDealInput{OwnerID: &next}, active); err != nil {
				close(held)
				return err
			}
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	swept := make(chan error, 1)
	sweepDone := make(chan struct{})
	go func() {
		defer close(sweepDone)
		swept <- e.reconcile()
	}()
	e.waitForRowLockWaiter(t, sweepDone,
		"the sweep finished while the reassignment was still uncommitted — it read the owner without waiting for it, which is the race this test exists to catch")
	close(release)
	if err := <-reassigned; err != nil {
		t.Fatalf("the reassignment: %v", err)
	}
	if err := <-swept; err != nil {
		t.Fatalf("the sweep: %v", err)
	}

	id, _ := e.followUpApproval(t, deal)
	wantSeat(t, "the card the sweep staged across the reassignment", e.seatOf(t, id), &e.Rep2)
}

// Two reassignments without If-Match, the second arriving while the first is
// uncommitted. The second must read the owner the first left — it is what the
// hand-over moves cards from and what its audit row says the deal was taken
// from — not the one both of them saw before either committed.
func TestConcurrentReassignmentsHandTheCardOnFromTheOwnerEachFinds(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Reassigned twice at once", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	id, _ := e.followUpApproval(t, deal)

	admin := e.Admin()
	active, err := e.Deals.ActiveDealColumns(admin)
	if err != nil {
		t.Fatal(err)
	}
	held, release := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		next := ids.From[ids.UserKind](e.Rep2)
		first <- e.DB().Tx(admin, func(tx pgx.Tx) error {
			defer close(held)
			if _, err := e.Deals.UpdateDealTx(admin, tx, ids.From[ids.DealKind](deal),
				deals.UpdateDealInput{OwnerID: &next}, active); err != nil {
				return err
			}
			held <- struct{}{}
			<-release
			return nil
		})
	}()
	<-held

	second := make(chan error, 1)
	secondDone := make(chan struct{})
	go func() {
		defer close(secondDone)
		last := ids.From[ids.UserKind](e.AdminUser)
		_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &last})
		second <- err
	}()
	e.waitForRowLockWaiter(t, secondDone,
		"the second reassignment finished while the first was uncommitted — it never waited on the deal")
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("the first reassignment: %v", err)
	}
	if err := <-second; err != nil {
		t.Fatalf("the second reassignment: %v", err)
	}
	wantSeat(t, "the card after both reassignments", e.seatOf(t, id), &e.AdminUser)
	var from string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT before->>'owner_id' FROM audit_log
		 WHERE entity_type = 'deal' AND entity_id = $1 AND after->>'owner_id' = $2`,
		deal, e.AdminUser.String()).Scan(&from); err != nil {
		t.Fatalf("reading the second reassignment's audit row: %v", err)
	}
	if from != e.Rep2.String() {
		t.Errorf("the second reassignment took the deal from %s, want %s — the owner the first one left", from, e.Rep2)
	}
}

// waitForRowLockWaiter returns once a backend in this database waits on a row
// lock. A row lock waits on the holder's transaction id, which pg_locks files
// under no database, so the waiter is scoped through its backend instead.
func (e *reconcileEnv) waitForRowLockWaiter(t *testing.T, done <-chan struct{}, finishedEarly string) {
	t.Helper()
	testdb.WaitForContention(t, done, finishedEarly,
		fmt.Sprintf("no backend waited on a row lock within %s — the racer never reached the deal, so this run proved nothing", testdb.ProbeBudget),
		func(ctx context.Context) (bool, error) {
			var waiting bool
			err := e.owner.QueryRow(ctx, `
				SELECT EXISTS (SELECT 1 FROM pg_locks l
				  JOIN pg_stat_activity a ON a.pid = l.pid
				 WHERE NOT l.granted AND l.locktype IN ('tuple', 'transactionid')
				   AND a.datname = current_database())`).Scan(&waiting)
			return waiting, err
		})
}

// reassigningDrafter drafts through the real adapter, after handing the deal
// to someone else — the window between composing a draft and staging it.
type reassigningDrafter struct {
	followUpReplySeam
	during func()
}

func (d reassigningDrafter) DraftEmail(ctx context.Context, anchor ids.UUID, intent string) (string, string, error) {
	d.during()
	return d.followUpReplySeam.DraftEmail(ctx, anchor, intent)
}

// A draft composed for the owner the deal has just left is dropped, and
// nothing takes its place: a task staged instead would keep the next sweep from
// drafting for the new owner until somebody decided it.
func TestADraftComposedForAnOwnerWhoLeftIsDroppedAndRedraftedNextSweep(t *testing.T) {
	e := setupReconcile(t)
	e.grantOwner(t, e.Rep2, reconcileOwnerPolicy)
	deal := e.SeedDeal(t, "Handed over mid-draft", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, deal, "Kickoff")
	stager := followUpStager{
		svc: e.svc,
		draft: reassigningDrafter{
			followUpReplySeam: newCommsAdapter(e.Pool, nil, SendPath{}),
			during:            func() { e.reassign(t, deal, e.Rep2) },
		},
		owner: dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())},
		pool:  e.Pool,
	}
	quiet := slog.New(slog.NewTextHandler(os.Stderr, nil))
	sweep := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:overnight")
	if err := deals.NewFollowUpReconciler(e.DB(), stager, quiet).ReconcileWorkspace(sweep); err != nil {
		t.Fatal(err)
	}
	var pending int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM approval WHERE target_entity_id = $1 AND status = 'pending'`, deal).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Fatalf("%d cards staged across the handover, want 0 — the stale draft must be dropped and nothing staged in its place", pending)
	}

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	fresh, _ := e.heldDraftFor(t, deal)
	wantSeat(t, "the draft the next sweep composed", e.seatOf(t, fresh), &e.Rep2)
}

// stagedFollowUp seeds a deal owned by Rep1 whose nightly card is waiting.
func (e *reconcileEnv) stagedFollowUp(t *testing.T, name string) (ids.UUID, ids.ApprovalID) {
	t.Helper()
	deal := e.SeedDeal(t, name, e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Call on "+name, 3)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	id, _ := e.followUpApproval(t, deal)
	return deal, id
}

func (e *reconcileEnv) dealOwner(t *testing.T, deal ids.UUID) ids.UUID {
	t.Helper()
	var owner ids.UUID
	if err := e.owner.QueryRow(context.Background(), `SELECT owner_id FROM deal WHERE id = $1`, deal).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	return owner
}

// A store built without the hand-over refuses to move an owner rather than
// commit a reassignment that leaves the cards pinned to the previous seat — on
// both writers. Re-sending the owner the deal already has moves nothing, so it
// needs no hand-over and goes through.
func TestAStoreWithoutTheHandOverRefusesToMoveTheOwner(t *testing.T) {
	e := setupReconcile(t)
	deal, id := e.stagedFollowUp(t, "No hand-over wired")
	unwired := deals.NewStore(e.DB(), deals.Installation{})
	dealID := ids.From[ids.DealKind](deal)

	next := ids.From[ids.UserKind](e.Rep2)
	if _, err := unwired.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{OwnerID: &next}); err == nil {
		t.Fatal("an update with no hand-over wired moved the owner")
	}
	if _, err := unwired.ClaimDeal(e.Admin(), dealID, nil); err == nil {
		t.Fatal("a claim with no hand-over wired moved the owner")
	}
	if got := e.dealOwner(t, deal); got != e.Rep1 {
		t.Errorf("the deal is owned by %s after two refused reassignments, want %s", got, e.Rep1)
	}
	wantSeat(t, "the card after the refused reassignments", e.seatOf(t, id), &e.Rep1)

	same := ids.From[ids.UserKind](e.Rep1)
	if _, err := unwired.UpdateDeal(e.Admin(), dealID, deals.UpdateDealInput{OwnerID: &same}); err != nil {
		t.Fatalf("re-sending the current owner asked for a hand-over: %v", err)
	}
	if got := e.seatMoves(t, id, e.Rep1); got != 0 {
		t.Errorf("re-sending the current owner recorded %d seat moves, want 0", got)
	}
}

// An archived deal is frozen: reassigning it is refused as absent, and its
// waiting card stays where it was.
func TestAnArchivedDealIsNotHandedOver(t *testing.T) {
	e := setupReconcile(t)
	deal, id := e.stagedFollowUp(t, "Archived with a card waiting")
	if _, err := e.Deals.ArchiveDeal(e.Admin(), ids.From[ids.DealKind](deal), nil); err != nil {
		t.Fatalf("archiving: %v", err)
	}
	next := ids.From[ids.UserKind](e.Rep2)
	_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{OwnerID: &next})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reassigning an archived deal → %v, want ErrNotFound", err)
	}
	wantSeat(t, "the archived deal's card", e.seatOf(t, id), &e.Rep1)
}

// Every seat move and every withdrawal is audited, and the audit row names who
// did it. A hand-over run with nobody to name is refused for both — the card
// neither moves nor is withdrawn.
func TestAHandOverNobodyCanBeNamedForIsRefused(t *testing.T) {
	e := setupReconcile(t)
	e.grantOwner(t, e.Rep2, reconcileOwnerPolicy)
	taskDeal, task := e.stagedFollowUp(t, "Task card, nobody named")
	draftDeal := e.SeedDeal(t, "Draft card, nobody named", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, draftDeal, "Kickoff")
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	draft, _ := e.heldDraftFor(t, draftDeal)

	anonymous := principal.WithWorkspaceID(context.Background(), e.WS)
	for name, deal := range map[string]ids.UUID{"the task card": taskDeal, "the drafted reply": draftDeal} {
		err := e.DB().Tx(anonymous, func(tx pgx.Tx) error {
			return approvals.FollowDealOwnerInTx(anonymous, tx, deal, &e.Rep1, &e.Rep2)
		})
		if err == nil {
			t.Errorf("%s: a hand-over with no actor to audit went through", name)
		}
	}
	wantSeat(t, "the task card", e.seatOf(t, task), &e.Rep1)
	if got := e.statusOf(t, draft); got != "pending" {
		t.Errorf("the drafted reply is %q after a refused hand-over, want pending", got)
	}
}

// failingDrafter stands in for a drafting engine that is down.
type failingDrafter struct {
	followUpReplySeam
}

func (failingDrafter) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return "", "", errors.New("the drafting engine is unavailable")
}

// A drafting failure fails the pass for that deal. It is not quietly turned
// into a task proposal, which would hide the fault and — through the pending
// check — keep the reply from ever being drafted.
func TestADraftingFailureStagesNothingInsteadOfATask(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Drafter down", e.pipeline, e.open, &e.Rep1)
	e.seedAnswerableThread(t, deal, "Kickoff")
	stager := followUpStager{
		svc:   e.svc,
		draft: failingDrafter{followUpReplySeam: newCommsAdapter(e.Pool, nil, SendPath{})},
		owner: dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())},
		pool:  e.Pool,
	}
	quiet := slog.New(slog.NewTextHandler(os.Stderr, nil))
	sweep := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "agent:overnight")
	if err := deals.NewFollowUpReconciler(e.DB(), stager, quiet).ReconcileWorkspace(sweep); err == nil {
		t.Fatal("the pass reported success while the drafter was down")
	}
	var pending int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM approval WHERE target_entity_id = $1 AND status = 'pending'`, deal).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 0 {
		t.Errorf("%d cards staged while the drafter was down, want 0", pending)
	}
}
