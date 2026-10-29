// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Per rep at the published tier. Pending cards fill more than two 50-row
// pages. Applied cards are the day's receipts; expired ones count nowhere.
const (
	dailyPendingPerRep = 130
	dailyAppliedPerRep = 40
	dailyExpiredPerRep = 20
)

// The seed's two actors, as the close-date sweep has them: a system pass
// stages the cards, and the owner's agent applies the ones on automatic.
const (
	dailyStagingActor  = "system:bench-close-date"
	dailyApprovalActor = "agent:bench-auto-apply"
)

// DailyApprovals is what seedDailyApprovals wrote, for the record and the
// census, and the cards applyDailyApprovals is still to apply.
type DailyApprovals struct {
	Pending, Applied, Expired int
	toApply                   []dailyApply
}

// dailyApply is one staged card and the rep's agent that applies it.
type dailyApply struct {
	agent context.Context
	id    ids.ApprovalID
}

// seedDailyApprovals stages close-date cards on each rep's own open deals the
// way the close-date sweep does, through the approvals service's own staging
// path. Expiry is the one fact set behind the product's back: a card ages
// into it after a week, and the bench cannot wait one.
func seedDailyApprovals(t *testing.T, e *apptest.AppEnv, seats Seats, scale float64) DailyApprovals {
	t.Helper()
	ctx := context.Background()
	ws := apptest.InstallationWorkspaceUUID(ctx, t, e.Pool)
	svc, users := compose.NewApprovalsService(e.DB()), identity.NewServiceFor(e.DB())
	sweep := principal.SystemActing(principal.WithWorkspaceID(ctx, ws), dailyStagingActor)
	perRep := func(n int) int { return max(2, int(float64(n)*scale)) }
	pending, applied, expired := perRep(dailyPendingPerRep), perRep(dailyAppliedPerRep), perRep(dailyExpiredPerRep)
	var made DailyApprovals
	for _, rep := range seats.Reps {
		repID := ids.MustParse(rep.UserID)
		agent := dailyRepAgent(t, users, ws, repID)
		for i, deal := range dailyOpenDeals(t, e, repID, pending+applied+expired) {
			id := stageDailyCloseDate(sweep, t, svc, deal, i)
			switch {
			case i < applied:
				made.toApply = append(made.toApply, dailyApply{agent, id})
			case i < applied+expired:
				if _, err := e.Owner.Exec(ctx, `UPDATE approval SET expires_at = now() - interval '1 hour' WHERE id = $1`, id); err != nil {
					t.Fatalf("expiring close-date card %s: %v", id, err)
				}
				made.Expired++
			default:
				made.Pending++
			}
		}
	}
	return made
}

// applyDailyApprovals applies the cards each rep put on automatic, as that
// rep's agent. It runs after the morning briefs, because "Done for you" lists
// what ran since the reader's brief, and the day's automatic work comes after it.
func applyDailyApprovals(t *testing.T, e *apptest.AppEnv, made DailyApprovals) DailyApprovals {
	t.Helper()
	// The production service, without the close-date effect: compose keeps the
	// auto-apply wiring that registers it to itself. The receipt is real; the
	// deal's date does not move.
	svc := compose.NewApprovalsService(e.DB())
	for _, card := range made.toApply {
		if _, err := svc.ApplyUnderPolicy(card.agent, card.id); err != nil {
			t.Fatalf("applying close-date card %s under policy: %v", card.id, err)
		}
		made.Applied++
	}
	made.toApply = nil
	t.Logf("seed: approvals %d pending, %d applied today, %d expired", made.Pending, made.Applied, made.Expired)
	return made
}

// dailyRepAgent is the principal the auto-apply sweep acts as: the rep's own
// grants, seat and teams, carried under an agent that may write.
func dailyRepAgent(t *testing.T, users *identity.Service, ws, rep ids.UUID) context.Context {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	rbac, seat, err := users.EffectiveAuthority(ctx, ws, rep)
	if err != nil {
		t.Fatalf("reading the authority of rep %s: %v", rep, err)
	}
	return principal.WithActor(principal.WithCorrelationID(ctx, ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: dailyApprovalActor, UserID: rep, OnBehalfOf: rep,
		SeatType: seat, TeamIDs: rbac.TeamIDs, Permissions: rbac.Permissions,
		Scopes: principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
	})
}

// dailyOpenDeals returns up to n of the rep's open deals, oldest first.
func dailyOpenDeals(t *testing.T, e *apptest.AppEnv, rep ids.UUID, n int) []ids.UUID {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(),
		`SELECT id FROM deal WHERE owner_id = $1 AND status = 'open' AND archived_at IS NULL ORDER BY id LIMIT $2`, rep, n)
	if err != nil {
		t.Fatalf("listing rep %s's open deals: %v", rep, err)
	}
	defer rows.Close()
	var deals []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("listing rep %s's open deals: %v", rep, err)
		}
		deals = append(deals, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing rep %s's open deals: %v", rep, err)
	}
	return deals
}

// stageDailyCloseDate stages one "is this date right" card for a deal. The
// i-th card proposes a date i days out, so no two cards carry one diff.
func stageDailyCloseDate(sweep context.Context, t *testing.T, svc *approvals.Service, deal ids.UUID, i int) ids.ApprovalID {
	t.Helper()
	raw, err := json.Marshal(deals.CloseDateCorrection{
		DealID:            ids.From[ids.DealKind](deal),
		ExpectedCloseDate: time.Now().AddDate(0, 0, 14+i).Format(time.DateOnly),
		Asking:            deals.AskingIsThisDateRight,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Stage(sweep, approvals.StageInput{
		Kind: deals.CloseDateCorrectionKind, ProposedChange: canonical, DiffHash: hash,
		TargetType: "deal", TargetID: deal, Summary: "Move the close date",
	})
	if err != nil {
		t.Fatalf("staging a close-date card on deal %s: %v", deal, err)
	}
	return id
}
