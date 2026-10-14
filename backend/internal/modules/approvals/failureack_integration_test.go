// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// A failed decision leaves its decider's list when they acknowledge it, or when
// a retry runs the work. It comes back when the work fails again.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// flakyEffect fails while failing is true, and otherwise redeems as a real
// effect does.
type flakyEffect struct {
	e       *stagingEnv
	failing bool
}

func (f *flakyEffect) run(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, diffHash string) error {
	if f.failing {
		return errors.New("the capture sink refused this lead")
	}
	return f.e.redeems(ctx, id, diffHash)
}

// ownFailures is the decider's list, as the Worklist lane reads it.
func (e *stagingEnv) ownFailures(ctx context.Context, t *testing.T) []FailedDecision {
	t.Helper()
	out, err := e.svc.OwnFailedDecisions(ctx, 8)
	if err != nil {
		t.Fatalf("listing the decider's failures: %v", err)
	}
	return out
}

// failedDecision stages a site lead and approves it under the given effect,
// which must fail.
func (e *stagingEnv) failedDecision(ctx context.Context, t *testing.T, effect ApprovedEffect) ids.ApprovalID {
	t.Helper()
	e.svc.WithEffect(kindSiteLead, effect)
	id := e.stageInto(ctx, t, ids.NewV7(), e.company(t), kindSiteLead, "lead-anna")
	if _, err := e.svc.Decide(ctx, id, true, nil); err == nil {
		t.Fatal("the failing effect reported no error")
	}
	return id
}

// markOf is the failure mark the decider is shown, which acknowledging names.
func (e *stagingEnv) markOf(t *testing.T, id ids.ApprovalID) time.Time {
	t.Helper()
	at, _ := e.effectFailureOf(t, id)
	if at == nil {
		t.Fatal("the approval carries no failure mark")
	}
	return *at
}

func TestAnAcknowledgedFailureLeavesItsDecidersListAndANewFailureReturns(t *testing.T) {
	e := setupStaging(t)
	effect := &flakyEffect{e: e, failing: true}
	deciding := e.asHumanWith(decidesEverything())
	id := e.failedDecision(deciding, t, effect.run)
	first := e.markOf(t, id)

	listed := e.ownFailures(deciding, t)
	if len(listed) != 1 || !listed[0].Retryable {
		t.Fatalf("the decider's list = %+v, want their one failure, offered for retry", listed)
	}

	// A colleague cannot clear it: the failure is not on their list, and its
	// existence is not theirs to learn.
	colleague := e.asRep(e.secondRep(t))
	if err := e.svc.AcknowledgeEffectFailure(colleague, id, first); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a colleague acknowledging answered %v, want not found", err)
	}
	if got := e.ownFailures(deciding, t); len(got) != 1 {
		t.Fatalf("a colleague's acknowledgement took the failure off the decider's list: %d rows", len(got))
	}

	if err := e.svc.AcknowledgeEffectFailure(deciding, id, first); err != nil {
		t.Fatalf("acknowledging: %v", err)
	}
	if got := e.ownFailures(deciding, t); len(got) != 0 {
		t.Fatalf("an acknowledged failure is still on the decider's list: %d rows", len(got))
	}
	// The failure itself stays on the record; only the list forgets it.
	if at, _ := e.effectFailureOf(t, id); at == nil {
		t.Error("acknowledging erased the failure mark from the approval")
	}
	audits := `SELECT count(*) FROM audit_log WHERE entity_type = 'approval' AND entity_id = $1 AND action = 'update'`
	events := `SELECT count(*) FROM event_outbox WHERE envelope #>> '{entity,id}' = $1::text
	             AND envelope #>> '{type}' = 'approval.failure_acknowledged'`
	if a, ev := e.count(t, audits, id), e.count(t, events, id); a != 1 || ev != 1 {
		t.Fatalf("the acknowledgement left %d audit rows and %d events, want 1 and 1", a, ev)
	}
	if err := e.svc.AcknowledgeEffectFailure(deciding, id, first); err != nil {
		t.Fatalf("acknowledging the same failure again: %v", err)
	}
	if a := e.count(t, audits, id); a != 1 {
		t.Errorf("acknowledging the same failure twice recorded %d audit rows, want 1", a)
	}

	// A retry that fails again is a new failure, and the decider is told.
	if _, err := e.svc.Decide(deciding, id, true, nil); err == nil {
		t.Fatal("the retry's failing effect reported no error")
	}
	if again := e.markOf(t, id); again.Equal(first) {
		t.Fatalf("the failed retry left the old mark %v, want a new one", first)
	}
	if got := e.ownFailures(deciding, t); len(got) != 1 {
		t.Fatalf("a new failure after an acknowledgement is not on the list: %d rows", len(got))
	}

	// A retry that runs the work clears the failure, on the list and the record.
	effect.failing = false
	if _, err := e.svc.Decide(deciding, id, true, nil); err != nil {
		t.Fatalf("retrying with a working effect: %v", err)
	}
	if got := e.ownFailures(deciding, t); len(got) != 0 {
		t.Fatalf("a failure whose work has since run is still on the list: %d rows", len(got))
	}
	if at, sentence := e.effectFailureOf(t, id); at != nil || sentence != nil {
		t.Errorf("the record still reports a failure after the work ran (at=%v sentence=%v)", at, sentence)
	}
}

// The reader acknowledges the failure they were shown. A failure recorded
// after it is one they have not seen, so it stays.
func TestAnAcknowledgementOfAnOlderFailureIsRefused(t *testing.T) {
	e := setupStaging(t)
	deciding := e.asHumanWith(decidesEverything())
	id := e.failedDecision(deciding, t, (&flakyEffect{e: e, failing: true}).run)
	seen := e.markOf(t, id)
	if _, err := e.svc.Decide(deciding, id, true, nil); err == nil {
		t.Fatal("the retry's failing effect reported no error")
	}

	if err := e.svc.AcknowledgeEffectFailure(deciding, id, seen); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("acknowledging an older failure answered %v, want a conflict", err)
	}
	if got := e.ownFailures(deciding, t); len(got) != 1 {
		t.Fatalf("the newer failure left the list on an acknowledgement of the older one: %d rows", len(got))
	}
}

// Retry is offered only where the approve endpoint would run the work again,
// which needs a registered effect for the kind.
func TestAFailureWithNoEffectToRerunIsNotOfferedForRetry(t *testing.T) {
	e := setupStaging(t)
	deciding := e.asHumanWith(decidesEverything())
	id := e.failedDecision(deciding, t, (&flakyEffect{e: e, failing: true}).run)
	delete(e.svc.effects, kindSiteLead)

	listed := e.ownFailures(deciding, t)
	if len(listed) != 1 || ids.UUID(listed[0].Approval.Id) != id.UUID {
		t.Fatalf("the decider's list = %+v, want their one failure", listed)
	}
	if listed[0].Retryable {
		t.Error("a failure whose kind has no effect to re-run is offered for retry")
	}
}

// Past the redemption window every effect's redemption is refused. A retry
// there could only fail again, so it is not offered, and approving again is
// answered as already decided rather than run.
func TestAFailurePastTheRedemptionWindowIsNotOfferedForRetry(t *testing.T) {
	e := setupStaging(t)
	deciding := e.asHumanWith(decidesEverything())
	id := e.failedDecision(deciding, t, (&flakyEffect{e: e, failing: true}).run)
	mark := e.markOf(t, id)
	e.pastTheWindow()

	listed := e.ownFailures(deciding, t)
	if len(listed) != 1 || listed[0].Retryable {
		t.Fatalf("the decider's list = %+v, want their one failure, not offered for retry", listed)
	}
	var decided *AlreadyDecidedError
	if _, err := e.svc.Decide(deciding, id, true, nil); !errors.As(err, &decided) {
		t.Fatalf("approving again past the window answered %v, want already decided", err)
	}
	if again := e.markOf(t, id); !again.Equal(mark) {
		t.Errorf("approving again past the window moved the failure mark from %v to %v", mark, again)
	}
}

// Some effects redeem first and then call out. When the call fails the row is
// already redeemed, and its decider must still be told.
func TestAFailureAfterTheRedemptionReachesTheDecider(t *testing.T) {
	e := setupStaging(t)
	deciding := e.asHumanWith(decidesEverything())
	redeemsThenFails := func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, diffHash string) error {
		if err := e.redeems(ctx, id, diffHash); err != nil {
			return err
		}
		return errors.New("the provider refused the write")
	}
	id := e.failedDecision(deciding, t, redeemsThenFails)

	listed := e.ownFailures(deciding, t)
	if len(listed) != 1 || ids.UUID(listed[0].Approval.Id) != id.UUID {
		t.Fatalf("the decider's list = %d rows, want the failure that came after the redemption", len(listed))
	}
	if listed[0].Retryable {
		t.Error("a redeemed approval is offered for retry, which the approve endpoint refuses")
	}
}
