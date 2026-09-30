// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The backstop pass, against a real database.
//
// Three of the five routes to a terminal approval — supersession, withdrawal
// and privacy erasure — write the row and emit nothing, by explicit design.
// Nothing downstream can be told, so every case here drives one of those
// ROUTES rather than stamping a status of its own: a pass proved against a
// state no writer produces proves nothing about the states they do.

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// overtakenKind is the one kind every proposal below is staged under, so a
// re-proposal matches the kind+target+identity a real supersession matches on.
const overtakenKind = "advance_deal"

// reProposeUnderOneIdentity stages a fresh proposal carrying the identity every
// other one here carries, which is what makes staging supersede the live one.
// The real route, because supersession is the case: it writes the terminal
// status inside the staging transaction and announces nothing.
func reProposeUnderOneIdentity(
	t *testing.T, e *integration.Env, svc *approvals.Service, summary string,
) ids.ApprovalID {
	t.Helper()
	id, err := svc.Stage(e.Admin(), approvals.StageInput{
		Kind:           overtakenKind,
		ProposedChange: json.RawMessage(`{"deal":"one","summary":"` + summary + `"}`),
		DiffHash:       "overtake-" + ids.NewV7().String(),
		Summary:        summary,
		JoinPending:    true,
		Identity:       json.RawMessage(`{"deal":"one"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// noticeFanOutCtx is what the fan-out writes a line under: the installation's
// workspace and the announcing system actor, never the seat being told.
func noticeFanOutCtx(e *integration.Env) context.Context {
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), approvals.NotifyActor)
}

// approvalLineFor is the line one seat holds about one approval — the dedupe
// key composed the way the fan-out composes it, because a key spelled by hand
// in a test matches nothing and the case would pass having swept air.
func approvalLineFor(approval ids.ApprovalID, seat ids.UUID) notices.NewNotice {
	return notices.NewNotice{
		Recipient: ids.From[ids.UserKind](seat),
		Kind:      notices.KindApprovalPending,
		Subject:   "a proposal is waiting for your decision",
		DedupeKey: notices.ApprovalDedupeKey(approval),
	}
}

// standingLine records that line through the store the fan-out records it with.
func standingLine(t *testing.T, e *integration.Env, approval ids.ApprovalID, seat ids.UUID) {
	t.Helper()
	ctx := noticeFanOutCtx(e)
	if _, err := notices.NewStore(e.DB()).Create(ctx, approvalLineFor(approval, seat)); err != nil {
		t.Fatal(err)
	}
}

// linesStanding counts the lines about these approvals that still say a
// decision waits on their reader.
func linesStanding(t *testing.T, e *integration.Env, approvals ...ids.ApprovalID) int {
	t.Helper()
	keys := make([]string, len(approvals))
	for i, approval := range approvals {
		keys[i] = notices.ApprovalDedupeKey(approval)
	}
	return e.WsCount(t, `SELECT count(*) FROM notice
		WHERE dedupe_key = ANY($1) AND overtaken_at IS NULL AND read_at IS NULL`, keys)
}

// runOvertakeSweep runs the pass exactly as the tick runs it.
func runOvertakeSweep(t *testing.T, e *integration.Env) {
	t.Helper()
	worker := newNoticeOvertakeWorker(e.DB(), identity.NewService(e.Pool), slog.Default())
	if err := worker.Work(context.Background(), &river.Job[NoticeOvertakeArgs]{}); err != nil {
		t.Fatalf("the overtaken-notice sweep: %v", err)
	}
}

// Staging replaced a stale card. That emits no event by design, so only a pass
// that reads the rows can take the lines back.
func TestTheSweepTakesBackLinesForAnApprovalNobodyAnnounced(t *testing.T) {
	e := integration.Setup(t)
	svc := approvals.NewService(e.DB())
	stale := reProposeUnderOneIdentity(t, e, svc, "the stale proposal")
	standingLine(t, e, stale, e.Rep1)
	standingLine(t, e, stale, e.Rep2)

	fresh := reProposeUnderOneIdentity(t, e, svc, "the proposal that replaced it")
	if fresh == stale {
		t.Fatal("the second staging joined the first instead of replacing it — nothing was superseded, so this case is about a route it never took")
	}
	standingLine(t, e, fresh, e.Rep1)
	// The premise, asserted rather than assumed: supersession is event-free, so
	// no consumer of approval.decided can ever cover this route.
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'approval.decided'`); n != 0 {
		t.Fatalf("supersession announced %d decision(s) — a consumer would reach this route and the backstop is not what this case proves", n)
	}

	runOvertakeSweep(t, e)

	if n := linesStanding(t, e, stale); n != 0 {
		t.Errorf("%d line(s) still say a decision waits on a proposal that was replaced", n)
	}
	// Nobody decided a supersession, so no colleague's name is stamped on the
	// lines it took back.
	if n := e.WsCount(t, `SELECT count(*) FROM notice
		WHERE dedupe_key = $1 AND overtaken_by IS NOT NULL`, notices.ApprovalDedupeKey(stale)); n != 0 {
		t.Error("a replaced proposal's lines name a deciding colleague — nobody decided it")
	}
	if n := linesStanding(t, e, fresh); n != 1 {
		t.Errorf("the replacement's own line is %d standing, want 1 — the sweep took back a decision that is still open", n)
	}
}

// A pending approval's lines are left exactly where they are.
func TestTheSweepLeavesALineWhoseApprovalIsStillWaiting(t *testing.T) {
	e := integration.Setup(t)
	svc := approvals.NewService(e.DB())
	waiting := reProposeUnderOneIdentity(t, e, svc, "still waiting on somebody")
	standingLine(t, e, waiting, e.Rep1)

	runOvertakeSweep(t, e)

	if n := linesStanding(t, e, waiting); n != 1 {
		t.Errorf("%d of 1 line(s) survive a sweep over an approval that is still decidable", n)
	}
}

// plantOverAChunk writes more standing references than one chunk holds: a full
// chunk of proposals still waiting, and a chunk and one withdrawn — withdrawal
// being the second route that writes the terminal status and announces nothing.
//
// MORE STALE REFERENCES THAN A CHUNK HOLDS is what makes the case decide
// anything. The references come back in the database's order and not the
// test's, so a single stale one among a crowd would only sometimes fall outside
// a bounded pass's first chunk; with a chunk and one, a pass that examined a
// single chunk must leave at least one behind whatever that order turns out to
// be.
func plantOverAChunk(t *testing.T, e *integration.Env) (stale, waiting []ids.ApprovalID) {
	t.Helper()
	svc := approvals.NewService(e.DB())
	human := e.Admin()
	stageOne := func(tx pgx.Tx) (ids.ApprovalID, error) {
		return svc.StageInTx(human, tx, approvals.StageInput{
			Kind:           overtakenKind,
			ProposedChange: json.RawMessage(`{"deal":"one of a crowd"}`),
			DiffHash:       "overtake-crowd-" + ids.NewV7().String(),
			Summary:        "one proposal in a crowd of them",
		})
	}
	if err := e.DB().Tx(human, func(tx pgx.Tx) error {
		for range noticeOvertakeChunk {
			id, err := stageOne(tx)
			if err != nil {
				return err
			}
			waiting = append(waiting, id)
		}
		for range noticeOvertakeChunk + 1 {
			id, err := stageOne(tx)
			if err != nil {
				return err
			}
			if _, err := svc.WithdrawInTx(human, tx, id, "the question stopped being one"); err != nil {
				return err
			}
			stale = append(stale, id)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	fanOut := noticeFanOutCtx(e)
	store := notices.NewStore(e.DB())
	if err := e.DB().Tx(fanOut, func(tx pgx.Tx) error {
		for _, approval := range append(append([]ids.ApprovalID{}, waiting...), stale...) {
			if _, err := store.CreateTx(fanOut, tx, approvalLineFor(approval, e.Rep1)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return stale, waiting
}

// Under-recognition is the one way this must not break: it would read a smaller
// set, report success, and leave no failing assertion. Plant a stale reference
// behind a full chunk of still-pending approvals and prove it still settles.
func TestTheSweepReachesAStaleLineBehindAFullChunkOfPendingOnes(t *testing.T) {
	e := integration.Setup(t)
	stale, waiting := plantOverAChunk(t, e)

	runOvertakeSweep(t, e)

	if n := linesStanding(t, e, stale...); n != 0 {
		t.Errorf("%d of %d withdrawn proposals still have a line standing — the pass bounded what it examined rather than the statement it examined it with", n, len(stale))
	}
	if n := linesStanding(t, e, waiting...); n != len(waiting) {
		t.Errorf("%d of %d lines survive for proposals that are still decidable", n, len(waiting))
	}
}
