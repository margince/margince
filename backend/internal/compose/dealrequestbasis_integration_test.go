// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCachedDealCardRefreshesTheRequestEvaluationWithoutRebuildingItsWords(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
	mailAt := at.AddDate(0, 0, -200)
	direction, subject := "inbound", "Please send the scope"
	source, _, err := store.LogActivity(reader, activities.LogActivityInput{
		Kind: "email", Source: "manual", Direction: &direction, Subject: &subject,
		OccurredAt: &mailAt, ThreadKey: "request-evaluation",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.booked}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestID := ids.UUID(source.Id)
	if _, _, err := store.LogActivity(reader, activities.LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &requestID}); err != nil {
		t.Fatal(err)
	}
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	first, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.booked))
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestBasis == nil || first.RequestBasis.EvaluatedAt != at || first.RequestBasis.SourceVersion == nil {
		t.Fatalf("fresh card lost current evidence: %+v", first.RequestBasis)
	}
	if first.MailCoverage == nil || first.MailCoverage.State != crmcontracts.MailCaptureCoverageStateUnknown || first.RequestBasis.MailCoverage == nil {
		t.Fatalf("absent capture was not qualified: %+v", first.MailCoverage)
	}
	at = at.Add(time.Minute)
	second, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.booked))
	if err != nil {
		t.Fatal(err)
	}
	if second.MailCoverage == nil || second.MailCoverage.EvaluatedAt != at {
		t.Fatalf("cached coverage was not refreshed: %+v", second.MailCoverage)
	}
	if second.GeneratedAt != first.GeneratedAt {
		t.Fatal("unchanged facts rebuilt the cached words")
	}
	if second.RequestBasis == nil || second.RequestBasis.EvaluatedAt != at || second.RequestBasis.SourceActivityId != source.Id {
		t.Fatalf("cached card reused an old evaluation: %+v", second.RequestBasis)
	}
	moves, err := cards.CurrentMoves(reader, []ids.UUID{e.booked})
	if err != nil || len(moves) != 1 {
		t.Fatalf("human cached move = %+v, %v", moves, err)
	}
	actor, ok := principal.Actor(reader)
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Type, actor.ID = principal.PrincipalAgent, "agent:"+ids.NewV7().String()
	actor.PassportID, actor.OnBehalfOf = ids.NewV7(), actor.UserID
	agent := principal.WithActor(reader, actor)
	moves, err = cards.CurrentMoves(agent, []ids.UUID{e.booked})
	if err != nil {
		t.Fatal(err)
	}
	if len(moves) != 1 {
		t.Fatalf("capture observation removed admitted work: %+v", moves)
	}
	for _, move := range moves {
		if move.MailCoverage == nil || move.MailCoverage.State != crmcontracts.MailCaptureCoverageStateWithheld || move.MailCoverage.LastSuccessAt != nil {
			t.Fatalf("agent reused human capture metadata: %+v", move)
		}
	}
	agentCard, err := cards.Get(agent, ids.From[ids.DealKind](e.booked), true)
	if err != nil || agentCard.MailCoverage == nil || agentCard.MailCoverage.State != crmcontracts.MailCaptureCoverageStateWithheld {
		t.Fatalf("agent observation = %+v, %v", agentCard.MailCoverage, err)
	}
	humanCard, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.booked))
	if err != nil || humanCard.GeneratedAt != first.GeneratedAt {
		t.Fatalf("agent refresh replaced the human briefing: %+v, %v", humanCard, err)
	}
	cards.WithMailCoverage(func(context.Context, *time.Time, time.Time) (crmcontracts.MailCaptureCoverage, error) {
		return crmcontracts.MailCaptureCoverage{}, errors.New("fixture coverage boundary failed")
	})
	moves, err = cards.CurrentMoves(reader, []ids.UUID{e.booked})
	if err != nil || len(moves) != 1 {
		t.Fatalf("capture failure removed recorded work: %+v, %v", moves, err)
	}
	for _, move := range moves {
		if move.MailCoverage == nil || move.MailCoverage.State != crmcontracts.MailCaptureCoverageStateUnknown {
			t.Fatalf("capture failure was not qualified: %+v", move.MailCoverage)
		}
	}
}
