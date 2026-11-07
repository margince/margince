// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asAgentOf is the user's agent on a passport: their grants and their UserID.
func (e *introEnv) asAgentOf(t *testing.T, u ids.UUID) context.Context {
	t.Helper()
	ctx := e.asUser(u)
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("no actor")
	}
	passport := ids.NewV7()
	p.Type, p.ID, p.PassportID, p.OnBehalfOf = principal.PrincipalAgent, "agent:"+passport.String(), passport, u
	return principal.WithActor(ctx, p)
}

// A user's agent reads, answers, completes and withdraws its user's asks, and
// the audit row names the agent.
func TestAUsersAgentActsOnTheirOwnAsks(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatal(err)
	}
	introducerAgent := e.asAgentOf(t, e.introducer)
	queue, err := e.store.AwaitingMyAnswer(introducerAgent, 25)
	if err != nil || len(queue) != 1 || queue[0].ID != id {
		t.Fatalf("the introducer's agent reads its queue: %+v, %v", queue, err)
	}
	if err := e.store.Decide(introducerAgent, id, StatusAccepted, "Happy to.", nil, 1); err != nil {
		t.Fatalf("the introducer's agent answers: %v", err)
	}
	var actorType, actorID string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT actor_type, actor_id FROM audit_log WHERE entity_id = $1 ORDER BY id DESC LIMIT 1`, id).Scan(&actorType, &actorID); err != nil {
		t.Fatal(err)
	}
	want, ok := principal.Actor(introducerAgent)
	if !ok || actorType != "agent" || actorID != want.ID {
		t.Errorf("the answer is audited to %s %s, want the agent %s", actorType, actorID, want.ID)
	}
	requesterAgent := e.asAgentOf(t, e.requester)
	seen, err := e.store.ForContact(requesterAgent, e.contact, 25)
	if err != nil || len(seen) != 1 {
		t.Fatalf("the requester's agent lists the ask: %+v, %v", seen, err)
	}
	if _, err := e.store.Get(requesterAgent, id); err != nil {
		t.Fatalf("the requester's agent opens the ask: %v", err)
	}
	if err := e.store.Complete(requesterAgent, id, nil, 2); err != nil {
		t.Fatalf("the requester's agent completes the ask: %v", err)
	}

	withdrawn, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.Cancel(requesterAgent, withdrawn, "", 1); err != nil {
		t.Fatalf("the requester's agent withdraws the ask: %v", err)
	}
}

// A colleague's agent is as much a stranger to an ask as the colleague is, and
// automatic work holding a party's id is no party.
func TestAnAgentOfAnotherUserAndAutomaticWorkAreStrangersToAnAsk(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatal(err)
	}
	automatic := e.asUser(e.introducer)
	p, ok := principal.Actor(automatic)
	if !ok {
		t.Fatal("no actor")
	}
	p.Type, p.ID = principal.PrincipalAgent, "agent:enrich"
	automatic = principal.WithActor(automatic, p)
	for name, ctx := range map[string]context.Context{
		"a colleague's agent": e.asAgentOf(t, e.stranger),
		"automatic work":      automatic,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := e.store.Get(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("Get = %v, want not found", err)
			}
			if err := e.store.Decide(ctx, id, StatusAccepted, "", nil, 1); err == nil {
				t.Error("Decide was admitted")
			}
			if err := e.store.Cancel(ctx, id, "", 1); err == nil {
				t.Error("Cancel was admitted")
			}
			if queue, err := e.store.AwaitingMyAnswer(ctx, 25); err == nil && len(queue) != 0 {
				t.Errorf("the queue showed %+v", queue)
			}
			if seen, err := e.store.ForContact(ctx, e.contact, 25); err == nil && len(seen) != 0 {
				t.Errorf("the list showed %+v", seen)
			}
		})
	}
}
