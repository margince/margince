// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// An approved row whose effect did not run must say so on the row itself.
// Nothing else can carry it: the row is not pending, so the decision lane
// skips it, and it names a human decider, so the receipts lane does too.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// effectFailureOf reads the mark straight from the table — what the decision
// LEFT is the question, not what it returned.
func (e *stagingEnv) effectFailureOf(t *testing.T, id ids.ApprovalID) (*time.Time, *string) {
	t.Helper()
	var at *time.Time
	var sentence *string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT effect_failed_at, effect_failure FROM approval WHERE id = $1`,
		id).Scan(&at, &sentence); err != nil {
		t.Fatalf("reading the effect-failure mark: %v", err)
	}
	return at, sentence
}

func TestAnApprovedEffectThatFailsIsMarkedOnTheRow(t *testing.T) {
	e := setupStaging(t)
	e.svc.WithEffect(kindSiteLead, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
		return errors.New("the capture sink refused this lead: relation lead_intake, host db-3")
	})
	ctx := e.asHumanWith(decidesEverything())
	org := e.organization(t)
	id := e.stageInto(ctx, t, ids.NewV7(), org, kindSiteLead, "lead-anna")

	_, decideErr := e.svc.Decide(ctx, id, true, nil)

	if decideErr == nil {
		t.Fatal("the failing effect's error was swallowed — the decider was told it worked")
	}
	if status := e.statusOf(t, id); status != approvalStatusApproved {
		t.Errorf("stored status = %s, want approved — the decision committed before the effect ran", status)
	}
	at, sentence := e.effectFailureOf(t, id)
	if at == nil || sentence == nil {
		t.Fatalf("the approved-but-failed row carries no mark (at=%v sentence=%v); it is unreachable by every lane", at, sentence)
	}
	// The stored sentence is the reader's, never the executor's error, which
	// can name a table or a host.
	if want := "this was approved, but the work it released did not run"; *sentence != want {
		t.Errorf("stored sentence = %q, want %q", *sentence, want)
	}
}

func TestAnApprovedEffectThatRunsLeavesNoFailureMark(t *testing.T) {
	e := setupStaging(t)
	e.svc.WithEffect(kindSiteLead, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
		return nil
	})
	ctx := e.asHumanWith(decidesEverything())
	org := e.organization(t)
	id := e.stageInto(ctx, t, ids.NewV7(), org, kindSiteLead, "lead-anna")

	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("deciding: %v", err)
	}

	if at, sentence := e.effectFailureOf(t, id); at != nil || sentence != nil {
		t.Errorf("a clean effect left a failure mark (at=%v sentence=%v)", at, sentence)
	}
}
