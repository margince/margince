// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func populationIdentityWriter(t *testing.T, e *forecastEnv) (*identity.Service, identity.Identity, context.Context) {
	t.Helper()
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	for _, object := range []string{"team_admin", "user_admin"} {
		actor.Permissions.Objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	ctx = principal.WithActor(ctx, actor)
	who := identity.Identity{UserID: ids.From[ids.UserKind](e.Rep1), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		SeatType: string(principal.SeatFull), Roles: []string{"admin"}, Permissions: actor.Permissions}
	return identity.NewServiceFor(InstallationDB(e.Pool)), who, ctx
}

func TestReportingPopulationWarmFactsDoNotAuthorizeAnotherLens(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	users, admin, ctx := populationIdentityWriter(t, e)
	if err := users.SetTeamMember(ctx, admin, e.Team1, e.Rep3, true); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	actor.Permissions.RowScope = principal.RowScopeTeam
	actor.TeamIDs = []ids.UUID{e.Team2, e.Team1, e.Team1}
	ctx = principal.WithActor(ctx, actor)
	requested := RequestedScope{Kind: ScopeKindOwner, ID: &e.Rep3}
	if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		counts.reset()
		before := append([]ids.UUID(nil), actor.TeamIDs...)
		if _, err := ResolveAnalyticsScope(cached, tx, requested); err != nil {
			return err
		}
		if !reflect.DeepEqual(actor.TeamIDs, before) {
			t.Fatal("membership key changed the actor's team list")
		}
		actor.TeamIDs = []ids.UUID{e.Team1, e.Team2}
		reordered := principal.WithActor(cached, actor)
		if _, err := ResolveAnalyticsScope(reordered, tx, requested); err != nil {
			return err
		}
		if counts.count("membership") != 1 || counts.count("owner") != 1 {
			t.Fatalf("equivalent team sets read membership=%d owner=%d", counts.count("membership"), counts.count("owner"))
		}
		actor.Permissions.RowScope = principal.RowScopeOwn
		if _, err := ResolveAnalyticsScope(principal.WithActor(cached, actor), tx, requested); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("warm facts admitted a narrower actor: %v", err)
		}
		actor.Permissions.RowScope = principal.RowScopeTeam
		actor.TeamIDs = []ids.UUID{ids.NewV7()}
		if _, err := ResolveAnalyticsScope(principal.WithActor(cached, actor), tx, requested); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("warm membership admitted another team set: %v", err)
		}
		if counts.count("membership") != 2 {
			t.Fatal("different team membership reused another key")
		}
		if _, err := ResolveAnalyticsScope(auth.WithIndividualPerformance(reordered, false), tx, requested); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Fatalf("warm subject bypassed the individual-performance switch: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReportingPopulationLivenessAgreesWithinAndAfterSnapshots(t *testing.T) {
	for _, subject := range []string{"team", "owner"} {
		t.Run(subject, func(t *testing.T) {
			e := setupForecast(t)
			traceReportingPopulation(t, e)
			users, admin, ctx := populationIdentityWriter(t, e)
			requested := RequestedScope{Kind: ScopeKindTeam, ID: &e.Team2}
			change := func() error {
				archived := true
				_, err := users.UpdateTeam(ctx, admin, e.Team2, identity.UpdateTeamInput{Archived: &archived})
				return err
			}
			if subject == "owner" {
				requested = RequestedScope{Kind: ScopeKindOwner, ID: &e.Rep3}
				change = func() error {
					return users.DeactivateUser(ctx, admin, identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep3)})
				}
			}
			if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
				cached, release := withReportingPopulationReads(ctx, tx)
				defer release()
				first, err := ResolveAnalyticsScope(cached, tx, requested)
				if err != nil {
					return err
				}
				// The writer commits on a separate connection after this snapshot's first read.
				if err := change(); err != nil {
					return err
				}
				warm, err := ResolveAnalyticsScope(cached, tx, requested)
				if err != nil {
					return err
				}
				uncached, err := ResolveAnalyticsScope(ctx, tx, requested)
				if err != nil {
					return err
				}
				if !reflect.DeepEqual(first, warm) || !reflect.DeepEqual(warm, uncached) {
					t.Fatal("cached subject disagreed with the original reader in the same snapshot")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
				cached, release := withReportingPopulationReads(ctx, tx)
				defer release()
				_, err := ResolveAnalyticsScope(cached, tx, requested)
				return err
			})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("next snapshot reused an inactive subject: %v", err)
			}
		})
	}
}

func TestReportingPopulationMembershipChangesReachTheNextSnapshot(t *testing.T) {
	e := setupForecast(t)
	traceReportingPopulation(t, e)
	users, admin, writer := populationIdentityWriter(t, e)
	if err := users.SetTeamMember(writer, admin, e.Team1, e.Rep3, true); err != nil {
		t.Fatal(err)
	}
	actor, ok := principal.Actor(writer)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	actor.Permissions.RowScope, actor.TeamIDs = principal.RowScopeTeam, []ids.UUID{e.Team1}
	ctx := principal.WithActor(writer, actor)
	requested := RequestedScope{Kind: ScopeKindOwner, ID: &e.Rep3}
	if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		if _, err := ResolveAnalyticsScope(cached, tx, requested); err != nil {
			return err
		}
		if err := users.SetTeamMember(writer, admin, e.Team1, e.Rep3, false); err != nil {
			return err
		}
		if _, err := ResolveAnalyticsScope(cached, tx, requested); err != nil {
			return err
		}
		_, err := ResolveAnalyticsScope(ctx, tx, requested)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		_, err := ResolveAnalyticsScope(cached, tx, requested)
		return err
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("fresh snapshot retained removed membership: %v", err)
	}
}

func TestReportingPopulationMissingSubjectsAreNotCached(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	missing := ids.NewV7()
	if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		counts.reset()
		for range 2 {
			for _, kind := range []string{ScopeKindTeam, ScopeKindOwner} {
				_, err := ResolveAnalyticsScope(cached, tx, RequestedScope{Kind: kind, ID: &missing})
				if !errors.Is(err, apperrors.ErrNotFound) {
					t.Fatalf("missing %s returned %v", kind, err)
				}
			}
		}
		if counts.count("team") != 2 || counts.count("owner") != 2 {
			t.Fatalf("missing subjects were cached: team=%d owner=%d", counts.count("team"), counts.count("owner"))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestReportingPopulationDistinctSubjectsDoNotShareLabelProofs(t *testing.T) {
	e := setupForecast(t)
	counts := traceReportingPopulation(t, e)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	if err := InstallationDB(e.Pool).TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		cached, release := withReportingPopulationReads(ctx, tx)
		defer release()
		counts.reset()
		for range 2 {
			for _, subject := range []struct {
				kind string
				id   ids.UUID
			}{
				{ScopeKindOwner, e.Rep1}, {ScopeKindOwner, e.Rep3}, {ScopeKindTeam, e.Team1}, {ScopeKindTeam, e.Team2},
			} {
				if _, err := ResolveAnalyticsScope(cached, tx, RequestedScope{Kind: subject.kind, ID: &subject.id}); err != nil {
					return err
				}
			}
		}
		if counts.count("owner") != 2 || counts.count("team") != 2 || counts.count("isolation") != 1 {
			t.Fatalf("distinct subject reads: owner=%d team=%d isolation=%d", counts.count("owner"), counts.count("team"), counts.count("isolation"))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A whole evaluation through the reporting service still refuses a subject that
// stopped being live before it began: the evaluation-wide reads start empty.
func TestReportingPopulationEvaluationsRefuseSubjectsThatLeft(t *testing.T) {
	e := setupForecast(t)
	traceReportingPopulation(t, e)
	users, admin, writer := populationIdentityWriter(t, e)
	service := newReportingService(e.Pool, time.Now)
	ctx := auth.WithIndividualPerformance(reportingActor(e), true)
	evaluate := func(scope crmcontracts.ReportingScope) error {
		_, err := service.Evaluate(ctx, crmcontracts.ReportingSelection{
			Scope: scope, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open",
			Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "open_pipeline"},
		})
		return err
	}
	team := crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team2)}
	owner := crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}
	for _, scope := range []crmcontracts.ReportingScope{team, owner} {
		if err := evaluate(scope); err != nil {
			t.Fatalf("live %s scope: %v", scope.Kind, err)
		}
	}
	archived := true
	if _, err := users.UpdateTeam(writer, admin, e.Team2, identity.UpdateTeamInput{Archived: &archived}); err != nil {
		t.Fatal(err)
	}
	if err := users.DeactivateUser(writer, admin, identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep3)}); err != nil {
		t.Fatal(err)
	}
	unknown := ids.NewV7()
	for _, scope := range []crmcontracts.ReportingScope{team, owner, {Kind: "team", Id: ptrUUID(unknown)}} {
		if err := evaluate(scope); !errors.Is(err, apperrors.ErrNotFound) {
			t.Fatalf("%s scope that is archived, inactive or unknown evaluated: %v", scope.Kind, err)
		}
	}
}
