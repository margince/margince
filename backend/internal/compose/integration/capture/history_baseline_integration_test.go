// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func listedBaseline(ctx context.Context, t *testing.T, registry *capturemod.Registry) *capturemod.HistoryBaseline {
	t.Helper()
	views, err := registry.Connections(ctx)
	if err != nil {
		t.Fatalf("listing connections: %v", err)
	}
	for _, v := range views {
		if v.Provider == "graph" {
			return v.HistoryBaseline
		}
	}
	t.Fatal("the connected mailbox is not in the list")
	return nil
}

func TestAMailboxStartsWithoutAHistoryBaseline(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)
	if got := listedBaseline(owner, t, registry); got != nil {
		t.Errorf("a new mailbox has baseline %+v, want none", *got)
	}
}

func TestAMailboxOwnerSetsReadsAndClearsTheHistoryBaseline(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)
	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	view, err := registry.SetHistoryBaseline(owner, "graph", &date, "Europe/Berlin")
	if err != nil {
		t.Fatalf("setting the baseline: %v", err)
	}
	if view.HistoryBaseline == nil || !view.HistoryBaseline.Date.Equal(date) || view.HistoryBaseline.Timezone != "Europe/Berlin" {
		t.Fatalf("the write answered %+v, want 2026-03-01 in Europe/Berlin", view.HistoryBaseline)
	}
	listed := listedBaseline(owner, t, registry)
	if listed == nil || !listed.Date.Equal(date) || listed.SetBy != "human:"+e.Rep1.String() || listed.SetAt.IsZero() {
		t.Fatalf("the listed baseline = %+v, want the date and the acting human", listed)
	}

	if _, err := registry.SetHistoryBaseline(owner, "graph", nil, ""); err != nil {
		t.Fatalf("clearing the baseline: %v", err)
	}
	if got := listedBaseline(owner, t, registry); got != nil {
		t.Errorf("the baseline after clearing = %+v, want none", *got)
	}
}

func TestSettingTheHistoryBaselineAuditsBothSides(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)
	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := registry.SetHistoryBaseline(owner, "graph", &date, "Europe/Berlin"); err != nil {
		t.Fatalf("setting the baseline: %v", err)
	}
	var before, after, actor *string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT before->>'history_baseline_date', after->>'history_baseline_date', actor_id
			  FROM audit_log
			 WHERE entity_type = 'capture_settings' AND after->>'provider' = 'graph'
			   AND after ? 'history_baseline_date'
			 ORDER BY occurred_at DESC LIMIT 1`).Scan(&before, &after, &actor)
	}); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if before != nil || after == nil || *after != "2026-03-01" {
		t.Errorf("audit images before=%v after=%v, want none then 2026-03-01", before, after)
	}
	if actor == nil || *actor != "human:"+e.Rep1.String() {
		t.Errorf("audit actor = %v, want the mailbox owner", actor)
	}
}

func TestOnlyAHumanSetsAHistoryBaseline(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, _ := connectedMailbox(t, e)
	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, kind := range []principal.PrincipalType{principal.PrincipalAgent, principal.PrincipalConnector, principal.PrincipalSystem} {
		ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
			Type: kind, ID: string(kind) + ":caller", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		})
		if _, err := registry.SetHistoryBaseline(ctx, "graph", &date, "Europe/Berlin"); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("a %s caller got %v, want permission denied", kind, err)
		}
	}
}

func TestAHistoryBaselineNeedsAZoneTheProductKnows(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)
	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, zone := range []string{"", "Local", "Mars/Olympus", " Europe/Berlin"} {
		_, err := registry.SetHistoryBaseline(owner, "graph", &date, zone)
		if _, ok := errors.AsType[*values.ParseError](err); !ok {
			t.Errorf("zone %q got %v, want a field error", zone, err)
		}
	}
	if got := listedBaseline(owner, t, registry); got != nil {
		t.Errorf("a refused zone stored baseline %+v", *got)
	}
}

func TestAColleagueCannotReachAnotherSeatsHistoryBaseline(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)
	colleague := humanWithScopes(e, e.Rep3, []principal.Scope{principal.ScopeRead})
	date := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := registry.SetHistoryBaseline(colleague, "graph", &date, "Europe/Berlin"); !errors.Is(err, capturemod.ErrNoConnection) {
		t.Errorf("a colleague's write got %v, want no connection", err)
	}
	if got := listedBaseline(owner, t, registry); got != nil {
		t.Errorf("the owner's mailbox gained baseline %+v from a colleague's write", *got)
	}
}
