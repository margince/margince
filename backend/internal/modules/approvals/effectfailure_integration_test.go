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
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// The failure comes back to the person who decided it, and to nobody else:
// FailedForDecider binds the acting user inside List, so one rep cannot read
// another's failed decisions by asking nicely.
func TestAFailedEffectIsListedForItsDeciderAlone(t *testing.T) {
	e := setupStaging(t)
	e.svc.WithEffect(kindSiteLead, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
		return errors.New("the capture sink refused this lead")
	})
	deciding := e.asHumanWith(decidesEverything())
	org := e.organization(t)
	id := e.stageInto(deciding, t, ids.NewV7(), org, kindSiteLead, "lead-anna")
	if _, err := e.svc.Decide(deciding, id, true, nil); err == nil {
		t.Fatal("the failing effect reported no error")
	}

	rows, _, err := e.svc.ListWire(deciding, ListInput{FailedForDecider: true, Limit: 8})
	if err != nil {
		t.Fatalf("listing the decider's failures: %v", err)
	}
	if len(rows) != 1 || ids.UUID(rows[0].Id) != id.UUID {
		t.Fatalf("the decider's list = %d rows, want their one failed decision", len(rows))
	}
	if rows[0].EffectFailedAt == nil || rows[0].EffectFailure == nil {
		t.Errorf("the wire row carries no failure mark: %+v", rows[0])
	}

	// The target archived AFTER the failure must not hide the row: the lane
	// exists to tell the decider work they released did not run, and the
	// probe that would re-ask "could you decide this now" answers a
	// different question.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE organization SET archived_at = now() WHERE id = $1`, org); err != nil {
		t.Fatal(err)
	}
	rows, _, err = e.svc.ListWire(deciding, ListInput{FailedForDecider: true, Limit: 8})
	if err != nil {
		t.Fatalf("listing after the target archived: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("the failure vanished when its target archived: %d rows", len(rows))
	}

	stranger := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other Rep')`,
		stranger, "other-"+stranger.String()+"@st.test"); err != nil {
		t.Fatal(err)
	}
	otherCtx := principal.WithWorkspaceID(context.Background(), e.ws)
	otherCtx = principal.WithCorrelationID(otherCtx, ids.NewV7())
	otherCtx = principal.WithActor(otherCtx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + stranger.String(), UserID: stranger,
		Permissions: decidesEverything(),
	})
	others, _, err := e.svc.ListWire(otherCtx, ListInput{FailedForDecider: true, Limit: 8})
	if err != nil {
		t.Fatalf("listing as another rep: %v", err)
	}
	if len(others) != 0 {
		t.Errorf("another rep read %d of the decider's failures, want none", len(others))
	}
}
