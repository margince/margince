// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// A configuration save that repeats the stored settings changes nothing, and
// its answer carries the same card fields the list does.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAConfigurationSaveOfTheStoredSettingsChangesNothing(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the runs environment has no actor")
	}
	actor.Permissions.Objects["integrations"] = principal.ObjectGrant{Read: true, Update: true}
	ctx := principal.WithActor(e.ctx, actor)

	standing, err := e.store.Get(ctx, "surfe")
	if err != nil {
		t.Fatal(err)
	}
	audits := func() int {
		var n int
		if err := e.owner.QueryRow(context.Background(),
			`SELECT count(*) FROM audit_log WHERE entity_type = 'provider_connection'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := audits()

	answer, err := e.store.UpdateConfig(ctx, "surfe", ConfigPatch{Mode: &standing.Mode, Preset: &standing.Preset}, standing.Version)
	if err != nil {
		t.Fatalf("saving the stored settings: %v", err)
	}
	if answer.Version != standing.Version {
		t.Errorf("a save of the stored settings answered version %d, want the unchanged %d", answer.Version, standing.Version)
	}
	if got := audits(); got != before {
		t.Errorf("a save of the stored settings wrote %d audit row(s), want none", got-before)
	}
	if len(answer.Catalog) == 0 {
		t.Error("the answer to a configuration save carries no catalog, though the list does")
	}

	flipped := !standing.AutomaticImport
	changed, err := e.store.UpdateConfig(ctx, "surfe", ConfigPatch{AutomaticImport: &flipped}, standing.Version)
	if err != nil {
		t.Fatalf("saving a real change: %v", err)
	}
	if changed.Version != standing.Version+1 {
		t.Errorf("a real change answered version %d, want %d", changed.Version, standing.Version+1)
	}
	if got := audits(); got != before+1 {
		t.Errorf("a real change wrote %d audit row(s), want 1", got-before)
	}
}

// A save that resends the pool budgets already stored is compared with the
// stored budgets, so it changes nothing either.
func TestAConfigurationSaveOfTheStoredBudgetsChangesNothing(t *testing.T) {
	e := setupRuns(t, runsConfig{ceilings: map[string]int{"email": 10}})
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the runs environment has no actor")
	}
	actor.Permissions.Objects["integrations"] = principal.ObjectGrant{Read: true, Update: true}
	ctx := principal.WithActor(e.ctx, actor)

	standing, err := e.store.Get(ctx, "surfe")
	if err != nil {
		t.Fatal(err)
	}
	ceiling := 10
	budgets := []PoolBudget{{Pool: "email", MonthlyCeiling: &ceiling}}
	answer, err := e.store.UpdateConfig(ctx, "surfe", ConfigPatch{Budgets: &budgets}, standing.Version)
	if err != nil {
		t.Fatalf("saving the stored budgets: %v", err)
	}
	if answer.Version != standing.Version {
		t.Errorf("a save of the stored budgets answered version %d, want the unchanged %d", answer.Version, standing.Version)
	}
}
