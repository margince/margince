// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// Taking a line back against a real database. One approval's notice is written
// once per seat that could decide it, so a decision has to reach all of them
// and only them, and has to leave alone the seat who already opened theirs.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sweepCtx is who runs the pass. The actor name is spelled here rather than
// imported: approvals is a sibling module and the guard this crosses asks the
// principal TYPE, so the string is the caller's identity and not a contract.
func (e *noticeEnv) sweepCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	return principal.SystemActing(ctx, "system:notice-overtaken-sweep")
}

// stageApprovalLine writes one seat's line about approval through the module's
// own writer, keyed the way the fan-out keys it.
func (e *noticeEnv) stageApprovalLine(t *testing.T, seat ids.UserID, approval ids.ApprovalID) ids.UUID {
	t.Helper()
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: seat, Kind: KindApprovalPending,
		Subject: "A discount needs a decision", DedupeKey: ApprovalDedupeKey(approval),
	})
	if err != nil {
		t.Fatalf("staging the seat's line: %v", err)
	}
	return id
}

// noticeStanding is the read-state of one line: when its reader opened it, when
// somebody else's decision took it back, and whose decision that was.
type noticeStanding struct {
	readAt      *time.Time
	overtakenAt *time.Time
	overtakenBy *ids.UUID
}

func (e *noticeEnv) standingOf(t *testing.T, id ids.UUID) noticeStanding {
	t.Helper()
	var s noticeStanding
	if err := e.owner.QueryRow(context.Background(),
		`SELECT read_at, overtaken_at, overtaken_by FROM notice WHERE id = $1`, id).
		Scan(&s.readAt, &s.overtakenAt, &s.overtakenBy); err != nil {
		t.Fatalf("reading the line's standing: %v", err)
	}
	return s
}

// Two seats hold a line for one approval. One decision takes both back, and
// names the colleague who decided on each.
func TestOvertakingTakesEverySeatsLineForOneApproval(t *testing.T) {
	e := setupNotices(t)
	approval := ids.New[ids.ApprovalKind]()
	mine := e.stageApprovalLine(t, e.recipient, approval)
	theirs := e.stageApprovalLine(t, e.other, approval)
	decider := e.other

	moved, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: approval, By: &decider}})
	if err != nil {
		t.Fatalf("overtaking: %v", err)
	}
	if moved != 2 {
		t.Fatalf("the pass moved %d line(s), want both seats' — a decision one colleague "+
			"cannot see taken back is the whole defect", moved)
	}
	for _, id := range []ids.UUID{mine, theirs} {
		got := e.standingOf(t, id)
		if got.overtakenAt == nil {
			t.Errorf("line %s still stands after the approval was decided", id)
		}
		if got.overtakenBy == nil || *got.overtakenBy != decider.UUID {
			t.Errorf("line %s names %v as the decider, want %s", id, got.overtakenBy, decider)
		}
		if got.readAt != nil {
			t.Errorf("line %s reads as opened; nobody opened it", id)
		}
	}

	// The audit row is the only record of WHEN each line stopped standing and
	// under whose pass, because the rows themselves are all alike afterwards.
	var audited int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'notice' AND action = 'update'
		   AND entity_id = ANY($1) AND evidence->>'overtaken' = 'true'`,
		[]ids.UUID{mine, theirs}).Scan(&audited); err != nil {
		t.Fatalf("counting the ledger entries: %v", err)
	}
	if audited != 2 {
		t.Errorf("%d ledger entries for 2 lines taken back", audited)
	}
}

// A reader who already opened the line keeps their own read_at and gains no
// overtaken_at: what they did is not overwritten by what happened later.
func TestOvertakingLeavesALineTheReaderAlreadyOpened(t *testing.T) {
	e := setupNotices(t)
	approval := ids.New[ids.ApprovalKind]()
	opened := e.stageApprovalLine(t, e.recipient, approval)
	standing := e.stageApprovalLine(t, e.other, approval)
	if err := e.store.MarkRead(e.asUser(e.recipient), opened); err != nil {
		t.Fatalf("the reader opening their own line: %v", err)
	}
	decider := e.other

	moved, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: approval, By: &decider}})
	if err != nil {
		t.Fatalf("overtaking: %v", err)
	}
	if moved != 1 {
		t.Fatalf("the pass moved %d line(s), want only the one still standing", moved)
	}
	was := e.standingOf(t, opened)
	if was.readAt == nil {
		t.Error("the reader's own read_at was cleared by a pass that is not theirs")
	}
	if was.overtakenAt != nil {
		t.Error("a line its reader had already opened was also marked overtaken — the two " +
			"answer different questions, and a reader who acted did not need taking back")
	}
	if e.standingOf(t, standing).overtakenAt == nil {
		t.Error("the seat who had not opened theirs still holds a line that waits on nobody")
	}
}

// Nobody decided it, so nobody is named. This is the case a careless
// implementation gets wrong by reaching into audit_log for a name.
func TestOvertakingNamesNobodyWhenNobodyDecided(t *testing.T) {
	e := setupNotices(t)
	approval := ids.New[ids.ApprovalKind]()
	line := e.stageApprovalLine(t, e.recipient, approval)

	moved, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: approval}})
	if err != nil {
		t.Fatalf("overtaking: %v", err)
	}
	if moved != 1 {
		t.Fatalf("the pass moved %d line(s), want the one that expired unanswered", moved)
	}
	got := e.standingOf(t, line)
	if got.overtakenAt == nil {
		t.Fatal("a line about an approval nobody can decide any more still stands")
	}
	if got.overtakenBy != nil {
		t.Errorf("the line names %s as a decider; the window closed and nobody decided", got.overtakenBy)
	}
}

// Running the pass twice moves nothing the second time.
func TestOvertakingASecondTimeMovesNothing(t *testing.T) {
	e := setupNotices(t)
	approval := ids.New[ids.ApprovalKind]()
	e.stageApprovalLine(t, e.recipient, approval)
	decider := e.other
	taking := []Overtaking{{Approval: approval, By: &decider}}

	first, err := e.store.OvertakeApprovalNotices(e.sweepCtx(), taking)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	if first != 1 {
		t.Fatalf("the first pass moved %d line(s), want 1", first)
	}
	again, err := e.store.OvertakeApprovalNotices(e.sweepCtx(), taking)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if again != 0 {
		t.Errorf("the second pass moved %d line(s), want none — a backstop that re-stamps "+
			"every line it already took writes a ledger entry per run forever", again)
	}
}

// The references are the approvals whose lines still stand — read ones and
// already-overtaken ones are not asked about again.
func TestStandingApprovalReferencesNamesOnlyTheLinesStillClaimingSomething(t *testing.T) {
	e := setupNotices(t)
	standing := ids.New[ids.ApprovalKind]()
	opened := ids.New[ids.ApprovalKind]()
	takenBack := ids.New[ids.ApprovalKind]()

	// Two seats on the standing one, so a reference asked about twice is still
	// one question for the caller to carry to approvals.
	e.stageApprovalLine(t, e.recipient, standing)
	e.stageApprovalLine(t, e.other, standing)
	alreadyOpened := e.stageApprovalLine(t, e.recipient, opened)
	if err := e.store.MarkRead(e.asUser(e.recipient), alreadyOpened); err != nil {
		t.Fatalf("the reader opening their own line: %v", err)
	}
	e.stageApprovalLine(t, e.recipient, takenBack)
	if _, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: takenBack}}); err != nil {
		t.Fatalf("taking the third approval's line back: %v", err)
	}
	// A notice of another kind carrying its own key is not an approval to ask
	// about, however standing it is.
	if _, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "lead_sla", Subject: "Response overdue",
		DedupeKey: "lead_sla:" + ids.NewV7().String(),
	}); err != nil {
		t.Fatalf("staging an unrelated notice: %v", err)
	}

	refs, err := e.store.StandingApprovalReferences(e.sweepCtx())
	if err != nil {
		t.Fatalf("reading the standing references: %v", err)
	}
	if len(refs) != 1 || refs[0] != standing {
		t.Fatalf("the standing references are %v, want exactly [%s] — an approval nobody is "+
			"still waiting on costs the pass a question, and one that is missing never gets asked",
			refs, standing)
	}
}
