// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// A paid run started by hand says what it buys, and a configuration patch says
// what it changes. Both are refused before they spend money or write an audit row.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func TestAManualRunThatLeavesOutWhatItBuysIsRefusedBeforeTheMoney(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	var before int
	if err := e.owner.QueryRow(e.ctx, `SELECT count(*) FROM provider_run`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		in    provider.QueueInput
		field string
	}{
		{"no provider", provider.QueueInput{}, "provider"},
		{"a provider of spaces", provider.QueueInput{Provider: "  "}, "provider"},
		{"an empty category list", provider.QueueInput{Provider: "surfe", Categories: []provider.Category{}}, "categories"},
	} {
		tc.in.ContactID, tc.in.Trigger = e.mine.String(), provider.TriggerManual
		_, err := e.store.QueueRun(e.ctx, tc.in)
		var refusal *UnnamedPurchaseError
		if !errors.As(err, &refusal) || refusal.Field != tc.field {
			t.Errorf("%s -> %v, want a refusal naming %s", tc.name, err, tc.field)
		}
	}
	var after int
	if err := e.owner.QueryRow(e.ctx, `SELECT count(*) FROM provider_run`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("%d run(s) were queued by requests that left out what they buy", after-before)
	}
}

func TestAManualRunStillQueuesWhenItOmitsItsCategoriesOrPadsItsProvider(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	run, err := e.store.QueueRun(e.ctx, provider.QueueInput{
		ContactID: e.mine.String(), Provider: " surfe ", Trigger: provider.TriggerManual,
	})
	if err != nil || run.State != provider.RunQueued {
		t.Fatalf("an omitted category list and a padded provider -> %v %v, want a queued run", run.State, err)
	}
}

func TestAConfigurationPatchThatNamesNoSettingIsRefused(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	actor, ok := principal.Actor(e.ctx)
	if !ok {
		t.Fatal("the runs environment has no actor")
	}
	actor.Permissions.Objects["integrations"] = principal.ObjectGrant{Read: true, Update: true}
	_, err := e.store.UpdateConfig(principal.WithActor(e.ctx, actor), "surfe", ConfigPatch{}, 0)
	if _, ok := errors.AsType[*EmptyConfigurationError](err); !ok {
		t.Fatalf("an empty configuration patch -> %v, want EmptyConfigurationError", err)
	}
}
