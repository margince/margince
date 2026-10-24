// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The scope POLICY, tested without a database.
//
// Every case here is a decision the resolver makes before it reads anything:
// which population an omitted request means, and which requested population is
// refused. Those are the cases a reviewer has to trust, and they are exactly
// the ones a fixture would obscure — a test that seeds two teams to prove a rep
// cannot name one is testing the seed.
//
// The paths that DO read (labelling a team, checking a colleague's membership)
// are covered by the integration test beside this file, which needs Postgres
// for the same reason they do.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func actorWithScope(scope principal.RowScope, teams ...ids.UUID) principal.Principal {
	return principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.NewV7(),
		TeamIDs:     teams,
		Permissions: principal.Permissions{RowScope: scope},
	}
}

// An omitted scope is the case the whole file exists for: it must mean the
// caller's own lens, never the workspace. The workspace default is what shipped
// and what a rep saw as somebody else's pipeline.
func TestAnOmittedScopeResolvesToTheCallersOwnLens(t *testing.T) {
	team := ids.NewV7()
	cases := []struct {
		name  string
		actor principal.Principal
		want  string
	}{
		{"a rep measures themselves", actorWithScope(principal.RowScopeOwn), ScopeKindOwner},
		{"a team manager measures their teams", actorWithScope(principal.RowScopeTeam, team), ScopeKindManagedTeams},
		{"management measures the workspace", actorWithScope(principal.RowScopeAll), ScopeKindWorkspace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), tc.actor)
			// The decision reads nothing — labelling does, and happens after —
			// so a nil tx proves the policy never depended on a database.
			got, err := resolveAnalyticsScope(ctx, nil, tc.actor, RequestedScope{})
			if err != nil {
				t.Fatalf("resolving an omitted scope: %v", err)
			}
			if got.Kind != tc.want {
				t.Errorf("omitted scope resolved to %q, want %q", got.Kind, tc.want)
			}
			if tc.want == ScopeKindOwner && (got.ID == nil || *got.ID != tc.actor.UserID) {
				t.Errorf("a rep's own scope named %v, want themselves", got.ID)
			}
		})
	}
}

// A team-lens principal who manages nothing is the case that decides whether
// "team" is read as a tier or as a fact. It must narrow to themselves: reading
// it as a tier would hand somebody managing no team the whole installation.
func TestATeamLensWithNoTeamsDoesNotWiden(t *testing.T) {
	actor := actorWithScope(principal.RowScopeTeam)
	ctx := principal.WithActor(context.Background(), actor)

	got, err := resolveAnalyticsScope(ctx, nil, actor, RequestedScope{})
	if err != nil {
		t.Fatalf("resolving a teamless manager's default: %v", err)
	}
	if got.Kind != ScopeKindOwner {
		t.Fatalf("a manager of no teams resolved to %q, want their own records", got.Kind)
	}
}

func TestAScopeOutsideTheCallersLensIsRefused(t *testing.T) {
	other := ids.NewV7()
	cases := []struct {
		name      string
		actor     principal.Principal
		requested RequestedScope
		wantErr   error
	}{
		{
			"a rep may not name a colleague",
			actorWithScope(principal.RowScopeOwn),
			RequestedScope{Kind: ScopeKindOwner, ID: &other},
			apperrors.ErrNotFound,
		},
		{
			"a population nobody defined is refused rather than ignored",
			actorWithScope(principal.RowScopeAll),
			RequestedScope{Kind: "everyone"},
			apperrors.ErrInvalidArgument,
		},
		{
			"the managed-teams kind cannot be asked for from the wire",
			actorWithScope(principal.RowScopeTeam, ids.NewV7()),
			RequestedScope{Kind: ScopeKindManagedTeams},
			apperrors.ErrInvalidArgument,
		},
		{
			"a workspace scope naming a subject is malformed",
			actorWithScope(principal.RowScopeAll),
			RequestedScope{Kind: ScopeKindWorkspace, ID: &other},
			apperrors.ErrInvalidArgument,
		},
		{
			"a team scope naming no team is malformed",
			actorWithScope(principal.RowScopeAll),
			RequestedScope{Kind: ScopeKindTeam},
			apperrors.ErrInvalidArgument,
		},
		{
			"an owner scope naming no owner is malformed",
			actorWithScope(principal.RowScopeAll),
			RequestedScope{Kind: ScopeKindOwner},
			apperrors.ErrInvalidArgument,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := principal.WithActor(context.Background(), tc.actor)
			_, err := resolveAnalyticsScope(ctx, nil, tc.actor, tc.requested)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("refusal was %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// A refusal over a named team or contact says "not found", never "denied": the
// second sentence confirms the id exists, which is the disclosure the lens was
// supposed to prevent.
func TestARefusalOverANamedSubjectDoesNotConfirmItExists(t *testing.T) {
	other := ids.NewV7()
	actor := actorWithScope(principal.RowScopeOwn)
	ctx := principal.WithActor(context.Background(), actor)

	for _, requested := range []RequestedScope{
		{Kind: ScopeKindOwner, ID: &other},
	} {
		_, err := resolveAnalyticsScope(ctx, nil, actor, requested)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("%s refusal admitted the subject exists", requested.Kind)
		}
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s refusal was %v, want not-found", requested.Kind, err)
		}
	}
}

// Yourself is inside every lens, including a rep's, and asking for it by id
// must not cost a membership read that would refuse it.
func TestNamingYourselfIsAlwaysWithinYourOwnLens(t *testing.T) {
	actor := actorWithScope(principal.RowScopeOwn)
	ctx := principal.WithActor(context.Background(), actor)
	me := actor.UserID

	got, err := resolveAnalyticsScope(ctx, nil, actor, RequestedScope{Kind: ScopeKindOwner, ID: &me})
	if err != nil {
		t.Fatalf("naming yourself was refused: %v", err)
	}
	if got.Kind != ScopeKindOwner || got.ID == nil || *got.ID != me {
		t.Fatalf("naming yourself resolved to %q/%v", got.Kind, got.ID)
	}
}

// A manager's default population clause must also admit a row nobody owns
// yet: an unrouted lead or an unassigned deal is on the SAME worklist row
// scope already hands a team manager, so a population clause that excludes it
// would disagree with the manager's own list about a row that is in both.
//
// arg is a no-op collector here — the assertion is on the RENDERED SQL shape,
// not on a live query — because this is the ManagedTeams path, the one kind
// labelScope never touches a database for (analyticsUserLabel/analyticsTeamLabel
// both need one), so this stays a pure policy test like its neighbours.
func TestManagedTeamsPopulationTreatsAnUnownedRowAsShared(t *testing.T) {
	team := ids.NewV7()
	actor := actorWithScope(principal.RowScopeTeam, team)
	ctx := principal.WithActor(context.Background(), actor)

	n := 0
	arg := func(any) int { n++; return n }
	resolved, clause, err := AnalyticsPopulationClause(ctx, nil, RequestedScope{}, "t", arg, unownedIsPartOfDefault)
	if err != nil {
		t.Fatalf("rendering a manager's default population clause: %v", err)
	}
	if resolved.Kind != ScopeKindManagedTeams {
		t.Fatalf("resolved to %q, want %q", resolved.Kind, ScopeKindManagedTeams)
	}
	if !strings.Contains(clause, "t.owner_id IS NULL") {
		t.Errorf("clause %q does not admit an unowned row", clause)
	}
}

// A manager's default is its own population and must not be reported as the
// workspace.
//
// The standing forecast call is looked up BY the scope, so flattening this to
// "workspace" — which an earlier spelling did — handed a team manager
// management's own call: its amount, its author and its note, printed beside
// totals covering only the manager's teams. Asking for the workspace explicitly
// would have been refused, which is what makes the flattened answer a
// disclosure rather than a mislabel.
func TestAManagersDefaultIsNotReportedAsTheWorkspace(t *testing.T) {
	resolved := ResolvedScope{Kind: ScopeKindManagedTeams, Label: managedTeamsLabel}

	got := forecastScopeFromResolved(resolved)

	if got.Kind == forecasting.ScopeWorkspace {
		t.Fatal("a manager's teams were reported as the whole workspace")
	}
	if got.Kind != forecasting.ScopeManagedTeams {
		t.Fatalf("resolved scope reported as %q", got.Kind)
	}
	if got.ID != nil {
		t.Errorf("the managed-teams population names a subject: %v", got.ID)
	}
}

// Totals are open to every seat that reads deals: the whole company and any
// team resolve for a rep, while a named colleague still does not.
func TestEverySeatMayMeasureTotalsForTheWorkspaceAndAnyTeam(t *testing.T) {
	team := ids.NewV7()
	for _, scope := range []principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll} {
		actor := actorWithScope(scope, ids.NewV7())
		ctx := principal.WithActor(context.Background(), actor)
		for _, requested := range []RequestedScope{
			{Kind: ScopeKindWorkspace},
			{Kind: ScopeKindTeam, ID: &team},
		} {
			got, err := resolveAnalyticsScope(ctx, nil, actor, requested)
			if err != nil || got.Kind != requested.Kind {
				t.Errorf("%v seat asking %s got %+v, %v", scope, requested.Kind, got, err)
			}
		}
	}
}

// A standing forecast or a share is a commitment about a population, so it
// keeps the tier rules even though the same seat may read the totals.
func TestAssertingAboutAPopulationKeepsTheTierRules(t *testing.T) {
	foreign := ids.NewV7()
	cases := []struct {
		name      string
		actor     principal.Principal
		requested RequestedScope
		resolved  ResolvedScope
		wantErr   error
	}{
		{"a rep may not assert about the workspace", actorWithScope(principal.RowScopeOwn),
			RequestedScope{Kind: ScopeKindWorkspace}, ResolvedScope{Kind: ScopeKindWorkspace}, apperrors.ErrPermissionDenied},
		{"a rep may not assert about a team", actorWithScope(principal.RowScopeOwn),
			RequestedScope{Kind: ScopeKindTeam, ID: &foreign}, ResolvedScope{Kind: ScopeKindTeam, ID: &foreign}, apperrors.ErrNotFound},
		{"a manager may not assert about the workspace", actorWithScope(principal.RowScopeTeam, ids.NewV7()),
			RequestedScope{Kind: ScopeKindWorkspace}, ResolvedScope{Kind: ScopeKindWorkspace}, apperrors.ErrPermissionDenied},
		{"a manager may not assert about a team they do not hold", actorWithScope(principal.RowScopeTeam, ids.NewV7()),
			RequestedScope{Kind: ScopeKindTeam, ID: &foreign}, ResolvedScope{Kind: ScopeKindTeam, ID: &foreign}, apperrors.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := assertionLensRefusal(tc.actor, tc.requested, tc.resolved); !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
	admin := actorWithScope(principal.RowScopeAll)
	if err := assertionLensRefusal(admin, RequestedScope{Kind: ScopeKindWorkspace}, ResolvedScope{Kind: ScopeKindWorkspace}); err != nil {
		t.Errorf("an all-scope seat was refused: %v", err)
	}
	rep := actorWithScope(principal.RowScopeOwn)
	if err := assertionLensRefusal(rep, RequestedScope{}, ResolvedScope{Kind: ScopeKindOwner}); err != nil {
		t.Errorf("a rep's own default was refused: %v", err)
	}
}

// The owner chart narrows for every request except an owner scope, which is
// already inside the lens.
func TestOnlyAnOwnerScopeIsExemptFromTheOwnerBreakdownNarrowing(t *testing.T) {
	id := ids.NewV7()
	for kind, want := range map[string]bool{
		"": true, ScopeKindWorkspace: true, ScopeKindTeam: true, ScopeKindOwner: false,
	} {
		if got := narrowsOwnerBreakdown(RequestedScope{Kind: kind, ID: &id}, true, false); got != want {
			t.Errorf("kind %q narrows=%v, want %v", kind, got, want)
		}
	}
	if narrowsOwnerBreakdown(RequestedScope{}, true, true) || narrowsOwnerBreakdown(RequestedScope{}, false, false) {
		t.Error("a pinned owner or a non-breakdown must not narrow")
	}
}
