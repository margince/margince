// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A panel that writes itself on a cache miss spends AI budget on a read. An
// agent spends only through the enrich scope. Without it the same read answers
// the deterministic floor and leaves the cache for the next reader.

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/companybrief"
	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// spendCountingLane counts the model calls a read starts. Its reply never
// parses, so every written answer is the floor and only the count differs.
type spendCountingLane struct{ calls int }

func (l *spendCountingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	return model.Response{Text: "not an answer"}, nil
}

// agentReading is ctx as an agent on its user's passport holding scopes.
func agentReading(ctx context.Context, t *testing.T, scopes ...principal.Scope) context.Context {
	t.Helper()
	p, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the context names no user to lend a passport")
	}
	p.Type, p.ID = principal.PrincipalAgent, "agent:"+ids.NewV7().String()
	p.PassportID, p.OnBehalfOf = ids.NewV7(), p.UserID
	p.Scopes = principal.NewScopeSet(scopes...)
	return principal.WithActor(ctx, p)
}

// panelRead reads one cached panel through lane and reports who wrote it.
type panelRead func(ctx context.Context, lane *spendCountingLane) (crmcontracts.WrittenBy, error)

// cachedPanels names each panel that writes itself on a cache miss.
func cachedPanels(t *testing.T, o *offeringEnv) map[string]panelRead {
	t.Helper()
	company, _ := o.customer(t, "Spend Motors")
	contact := o.SeedContact(t, "Spent Subject", &o.AdminUser)
	clock := func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }
	return map[string]panelRead{
		"the dossier": func(ctx context.Context, lane *spendCountingLane) (crmcontracts.WrittenBy, error) {
			out, err := companydossier.NewService(o.Pool, o.Contacts, lane, func() string { return "routing" }, clock).Get(ctx, company, false)
			return out.GeneratedBy, err
		},
		"the growth fit": func(ctx context.Context, lane *spendCountingLane) (crmcontracts.WrittenBy, error) {
			out, err := companydossier.NewGrowthFitService(o.Pool, o.Contacts, offeringConfirmed(o.Contacts), lane, func() string { return "routing" }, clock).Get(ctx, company, false)
			return out.GeneratedBy, err
		},
		"the company brief": func(ctx context.Context, lane *spendCountingLane) (crmcontracts.WrittenBy, error) {
			view := company360.NewService(o.Pool, o.Contacts, o.Deals, o.Projects, approvals.NewService(o.DB()), clock)
			out, err := companybrief.NewService(o.Pool, view, o.Contacts, lane, func() string { return "routing" }, clock).Get(ctx, company, false)
			return out.GeneratedBy, err
		},
		"the contact brief": func(ctx context.Context, lane *spendCountingLane) (crmcontracts.WrittenBy, error) {
			out, err := contactbrief.NewService(o.Pool, briefPageOf(contact), lane, func() string { return "routing" }, clock).Get(ctx, ids.From[ids.ContactKind](contact), false)
			return out.GeneratedBy, err
		},
	}
}

func TestAnAgentWithoutEnrichReadsAPanelWithoutStartingAModelCall(t *testing.T) {
	t.Parallel()
	o := setupOffering(t)
	for name, read := range cachedPanels(t, o) {
		t.Run(name, func(t *testing.T) {
			lane := &spendCountingLane{}
			by, err := read(agentReading(o.ctx, t, principal.ScopeRead), lane)
			if err != nil {
				t.Fatalf("the agent's read: %v", err)
			}
			if lane.calls != 0 || by != crmcontracts.WrittenByDeterministic {
				t.Errorf("an agent without enrich started %d model calls and read %q, want 0 and deterministic", lane.calls, by)
			}
			if _, err := read(o.ctx, lane); err != nil {
				t.Fatalf("the user's read: %v", err)
			}
			if lane.calls == 0 {
				t.Error("the user's read started no model call, so the agent's floor was cached or the lane is gone")
			}
		})
	}
}

// A floor written because the model call failed is served but never cached.
// The next read asks the model again, so an outage does not outlast itself.
func TestAFailedModelCallLeavesNoPanelInTheCache(t *testing.T) {
	t.Parallel()
	o := setupOffering(t)
	for name, read := range cachedPanels(t, o) {
		t.Run(name, func(t *testing.T) {
			lane := &spendCountingLane{}
			for range 2 {
				by, err := read(o.ctx, lane)
				if err != nil {
					t.Fatalf("read: %v", err)
				}
				if by != crmcontracts.WrittenByDeterministic {
					t.Fatalf("generated_by = %q after a failed call, want deterministic", by)
				}
			}
			if lane.calls != 2 {
				t.Errorf("model calls = %d over two reads, want 2: the failed call's floor was served from the cache", lane.calls)
			}
		})
	}
}

// A room brief is written anew on every read, so only a caller who may spend
// starts its model call.
func TestARoomBriefStartsAModelCallOnlyForACallerWhoMaySpend(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["deal_room"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	user := e.As(e.AdminUser, nil, perms)
	pipeline, stage, _ := integration.DealFixture(t, e)
	room, err := dealrooms.NewStore(e.DB()).CreateRoom(user, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](e.SeedDeal(t, "Spend rollout", pipeline, stage, &e.AdminUser)), Title: "Spend rollout", Source: "manual",
	})
	if err != nil {
		t.Fatalf("opening the room: %v", err)
	}
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	if _, err := dealrooms.NewStore(e.DB()).OpenThread(user, roomID, dealrooms.OpenThreadInput{Body: "Any questions on the terms?", Source: "manual"}); err != nil {
		t.Fatalf("opening a thread: %v", err)
	}
	lane := &spendCountingLane{}
	service := newRoomBriefService(e.Pool).WithLane(lane)
	if _, err := service.Get(agentReading(user, t, principal.ScopeRead), roomID); err != nil {
		t.Fatalf("the agent's read: %v", err)
	}
	if lane.calls != 0 {
		t.Errorf("an agent without enrich started %d model calls", lane.calls)
	}
	if _, err := service.Get(user, roomID); err != nil {
		t.Fatalf("the user's read: %v", err)
	}
	if lane.calls == 0 {
		t.Error("the user's read started no model call")
	}
}
