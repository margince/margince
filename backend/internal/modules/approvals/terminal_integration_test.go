// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// Which approvals stopped being decidable, and who decided them.
//
// The answer has to hold for a window that closed and was never stamped, which
// is why it is asked of a live Postgres rather than a fake: expires_at is set
// by the database's own clock, and a row read as pending until a sweep catches
// up is exactly the false "a decision waits on you" this seam exists to end.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// overtaking binds one of the two passes this seam admits.
func (e *stagingEnv) overtaking(actor string) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: actor})
}

// stageWindowed stages one agent call with its own approvable window.
//
// The hash varies per call because it is the proposal's identity: four
// stagings of one kind against one company are four distinct proposals here,
// and sharing a hash would make them one.
func (e *stagingEnv) stageWindowed(t *testing.T, passport, company ids.UUID, hash string, ttl *time.Duration) ids.ApprovalID {
	t.Helper()
	in := e.agentCall(company)
	in.DiffHash = hash
	in.TTL = ttl
	id, err := e.svc.Stage(e.asPassport(passport), in)
	if err != nil {
		t.Fatalf("staging %s: %v", hash, err)
	}
	return id
}

// decidedBy indexes an answer by approval id, so a case can ask about one row
// without depending on the order the rows came back in.
func decidedBy(terminal []Terminal) map[ids.ApprovalID]*ids.UserID {
	by := make(map[ids.ApprovalID]*ids.UserID, len(terminal))
	for _, one := range terminal {
		by[one.ID] = one.DecidedBy
	}
	return by
}

// A pending approval is absent from the answer; a decided one is present and
// names its decider; an expired one is present and names nobody; and a row
// whose window closed with no sweep to stamp it is ALREADY terminal, because
// nobody can decide it either.
func TestTerminalAmongReportsEveryRowThatStoppedBeingDecidable(t *testing.T) {
	e := setupStaging(t)
	passport, company := e.seedPassport(t), e.seedCompany(t)
	shortWindow, longerWindow := time.Hour, 3*time.Hour

	stillWaiting := e.stageWindowed(t, passport, company, "hash-still-waiting", nil)
	decided := e.stageWindowed(t, passport, company, "hash-decided", nil)
	unstamped := e.stageWindowed(t, passport, company, "hash-window-closed", &longerWindow)
	stamped := e.stageWindowed(t, passport, company, "hash-swept", &shortWindow)
	e.approve(t, decided)

	// Past the short window only, so the sweep stamps one row and leaves the
	// longer one for the reading to catch.
	e.svc.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := e.svc.ExpireDue(e.sweeping()); err != nil {
		t.Fatalf("expiring the row whose window closed first: %v", err)
	}
	if status := e.statusOf(t, stamped); status != StatusExpired {
		t.Fatalf("the swept row reads %s, want expired — the case this fixture exists for is not set up", status)
	}
	if status := e.statusOf(t, unstamped); status != StatusPending {
		t.Fatalf("the unswept row reads %s, want a stored pending — the whole point is that the column still says so", status)
	}

	e.svc.now = func() time.Time { return time.Now().Add(4 * time.Hour) }
	terminal, err := e.svc.TerminalAmong(e.overtaking(OvertakenSweepActor),
		[]ids.ApprovalID{stillWaiting, decided, unstamped, stamped})
	if err != nil {
		t.Fatalf("asking which approvals stopped being decidable: %v", err)
	}
	by := decidedBy(terminal)
	if len(by) != 3 {
		t.Fatalf("reported %d approval(s) as terminal, want the three nobody can decide any more: %v", len(by), terminal)
	}
	if _, present := by[stillWaiting]; present {
		t.Error("an approval that is still pending was reported terminal — every seat's line would be taken back while the decision still waits")
	}
	if who := by[decided]; who == nil || who.UUID != e.rep {
		t.Errorf("the decided approval names %v, want the colleague who decided it (%s)", who, e.rep)
	}
	// Named nobody, both ways a window closes. A human's name on a refusal
	// they never made is the one thing this must not answer.
	if who := by[stamped]; who != nil {
		t.Errorf("the swept approval names %s; the clock decided it and nobody else did", who)
	}
	if who := by[unstamped]; who != nil {
		t.Errorf("the unswept approval names %s; the clock decided it and nobody else did", who)
	}
}

func TestTerminalAmongRefusesAnActorItDoesNotAdmit(t *testing.T) {
	e := setupStaging(t)
	passport, company := e.seedPassport(t), e.seedCompany(t)
	decided := e.stageWindowed(t, passport, company, "hash-refusal", nil)
	e.approve(t, decided)
	candidates := []ids.ApprovalID{decided}

	// A human, and a system pass presenting some other name. Each would learn
	// that this approval exists and which colleague decided it, which is why
	// "some system principal" is not the claim being checked.
	refused := map[string]context.Context{
		"a signed-in human": e.as(),
		"the expiry sweep":  e.sweeping(),
	}
	for who, ctx := range refused {
		got, err := e.svc.TerminalAmong(ctx, candidates)
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s asking which approvals are terminal → (%v, %v), want ErrPermissionDenied", who, got, err)
		}
		if got != nil {
			t.Errorf("%s was answered with %v rather than nothing", who, got)
		}
	}

	// The fan-out consumer is admitted on the same terms as the sweep: it
	// takes lines back the instant a decision lands.
	if _, err := e.svc.TerminalAmong(e.overtaking(NotifyActor), candidates); err != nil {
		t.Errorf("the notify consumer was refused: %v", err)
	}
}
