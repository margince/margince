// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// walkedSuggestions follows next_cursor through the decisions view to its end
// and answers how many rows of one source it was served.
func walkedSuggestions(
	ctx context.Context, t *testing.T, e *integration.Env, scope string, source crmcontracts.WorklistItemSource,
) int {
	t.Helper()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now)
	seen := map[string]bool{}
	cursor := ""
	for page := 0; ; page++ {
		if page > 20 {
			t.Fatalf("%s: the walk did not end in 20 pages", scope)
		}
		day, err := svc.Worklist(ctx, scope, "decisions", ids.Nil, 2, cursor)
		if err != nil {
			t.Fatalf("%s page %d: %v", scope, page, err)
		}
		for _, row := range day.Queue {
			if row.Source != source {
				continue
			}
			if seen[row.Id] {
				t.Fatalf("%s: %s %s was served twice", scope, source, row.Id)
			}
			seen[row.Id] = true
		}
		if day.NextCursor == nil {
			return len(seen)
		}
		cursor = *day.NextCursor
	}
}

// distinctBuyerPair is buyerPair with a buyer of its own name, so each account's
// buyer is one person employed there and no employment is ambiguous.
func (e *scoutEnv) distinctBuyerPair(t *testing.T, company ids.UUID, name string) {
	t.Helper()
	buyer := e.employee(t, name, company)
	subject, body, direction, at := "Platform request", "We need a platform migration. Please send a proposal.", "inbound", e.daysAgo(6)
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.signal(t, "new_opportunity", company, ids.UUID(logged.Id), e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", company, ids.UUID(logged.Id), e.now, ids.Nil)
}

// The store's open suggestion count, which a cut lane states as its total, is
// the number of suggestion rows the list holds in each scope that reads them.
// A dismissed suggestion and one on another team's account count in neither.
func TestTheSuggestionCountMatchesTheWalkInEveryScopeThatReadsIt(t *testing.T) {
	e := setupScout(t)
	for i, owner := range []*ids.UUID{&e.Rep1, &e.Rep2, &e.Rep3, nil, &e.Rep1} {
		company := e.SeedCompany(t, fmt.Sprintf("Account %d", i), owner)
		e.distinctBuyerPair(t, company, fmt.Sprintf("Buyer %d", i))
	}
	e.pass(t)
	store := deals.NewStore(InstallationDB(e.Pool), DealsInstallation())
	open, _, err := store.ListSuggestions(e.signalReader(), deals.SuggestionQuery{Limit: 10})
	if err != nil || len(open) != 5 {
		t.Fatalf("%d open suggestions (%v), want one per account", len(open), err)
	}
	if _, err := e.decider().DismissSuggestion(e.signalReader(), open[0].ID); err != nil {
		t.Fatalf("dismissing a suggestion: %v", err)
	}

	perms := leadRepPerms
	perms.RowScope = principal.RowScopeTeam
	perms.Objects = maps.Clone(leadRepPerms.Objects)
	perms.Objects["company"] = principal.ObjectGrant{Read: true}
	perms.Objects["signal"] = principal.ObjectGrant{Read: true}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	checks := []struct {
		name, scope string
		ctx         context.Context
	}{
		{"team", "team", rep1},
		{"unassigned", "unassigned", rep1},
		{"all", "all", e.signalReader()},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			counted, err := attentionDealSuggestions{store: store}.CountOpen(check.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if walked := walkedSuggestions(check.ctx, t, e.Env, check.scope, crmcontracts.WorklistItemSourceDealSuggestion); walked != counted || walked == 0 {
				t.Errorf("the store counts %d open suggestions, the walk served %d", counted, walked)
			}
		})
	}
	if counted, _ := (attentionDealSuggestions{store: store}).CountOpen(e.signalReader()); counted != 4 {
		t.Errorf("the admin's count is %d, want the four open suggestions", counted)
	}
}

// The tag suggestion count is likewise the walk's row count in every scope
// that reads it, without a dismissed suggestion.
func TestTheTagSuggestionCountMatchesTheWalkInEveryScopeThatReadsIt(t *testing.T) {
	e := setupTagScout(t)
	e.tag(t, "Product X", "product x pricing", true)
	for i, owner := range []*ids.UUID{&e.Rep1, &e.Rep2, &e.Rep3, nil, &e.Rep1} {
		contact := e.SeedContact(t, fmt.Sprintf("Buyer %d", i), owner)
		e.note(e.Admin(), t, "note", "Asked about Product X pricing.", contact, time.Now().Add(-time.Hour))
	}
	if pass := e.pass(t, time.Now()); pass.Raised != 5 {
		t.Fatalf("the pass raised %d suggestions, want one per contact", pass.Raised)
	}
	open, err := e.tags.OpenTagSuggestions(e.Admin(), 10)
	if err != nil || len(open) != 5 {
		t.Fatalf("%d open tag suggestions (%v), want 5", len(open), err)
	}
	if _, err := e.tags.DismissTagSuggestion(e.Admin(), open[0].ID); err != nil {
		t.Fatalf("dismissing: %v", err)
	}
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	for _, scope := range []string{"team", "unassigned", "all"} {
		t.Run(scope, func(t *testing.T) {
			counted, err := attentionTagSuggestions{store: e.tags}.CountOpenTagSuggestions(rep1)
			if err != nil {
				t.Fatal(err)
			}
			walked := walkedSuggestions(rep1, t, e.Env, scope, crmcontracts.WorklistItemSourceTagSuggestion)
			if walked != counted || counted != 4 {
				t.Errorf("the store counts %d open tag suggestions, the walk served %d, want 4", counted, walked)
			}
		})
	}
}
