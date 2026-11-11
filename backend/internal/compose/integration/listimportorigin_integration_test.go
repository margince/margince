// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestListRulesExcludeImportedChangesBeforeCountingAndLimiting(t *testing.T) {
	f := newLiveListFixture(t)
	f.clock = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	f.check(t)
	ctx := principal.SystemActing(f.e.Admin(), "system:import-origin-test")
	run, err := migration.NewRunStore(f.e.DB()).Create(ctx, migration.CreateRunInput{Connector: "csv", Source: "manual", SourceRef: "demo-customers.csv"})
	if err != nil {
		t.Fatal(err)
	}
	imported := storekit.WithImportRun(storekit.WithBatch(ctx, run.ID), run.ID)
	title := "Buyer"
	if _, err := f.e.Contacts.CreateContact(imported, contacts.CreateContactInput{
		FullName: "Imported buyer", Title: &title, Source: "manual", OwnerID: userIDPtr(&f.e.Rep1),
	}); err != nil {
		t.Fatal(err)
	}
	manual := f.contactTitled(t, "Chosen buyer", title)
	f.clock = time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
	f.check(t)
	list, err := f.store.GetList(f.e.Admin(), f.list)
	if err != nil {
		t.Fatal(err)
	}
	changes, count, err := f.store.ObservedChanges(f.e.Admin(), f.list, list.Version, f.clock, []string{"entered"}, 1, false)
	if err != nil || count != 1 || len(changes) != 1 || changes[0].EntityID != manual {
		t.Fatalf("reviewed changes = %+v, total %d, %v", changes, count, err)
	}
	legacy, count, err := f.store.ObservedChanges(f.e.Admin(), f.list, list.Version, f.clock, []string{"entered"}, 10)
	if err != nil || count != 2 || len(legacy) != 2 {
		t.Fatalf("legacy changes = %+v, total %d, %v", legacy, count, err)
	}
	if legacy[0].EventID.IsZero() {
		t.Fatal("the imported change lost its durable history")
	}
}

func TestNewListRulesSkipImportsWithoutChangingLegacyRows(t *testing.T) {
	f := newListRuleFixture(t)
	id := f.rule(t, "list_membership_notify", map[string]any{})
	rule := f.ruleState(t, id)
	var params struct {
		IncludeImported *bool `json:"include_imported_changes"`
	}
	if err := json.Unmarshal(rule.Params, &params); err != nil {
		t.Fatal(err)
	}
	if params.IncludeImported == nil || *params.IncludeImported {
		t.Fatalf("new import policy = %s", rule.Params)
	}
}

func TestReviewedEventRulesLeaveQueuedHistoryAlone(t *testing.T) {
	f := newListRuleFixture(t)
	f.clock = time.Date(2026, 12, 20, 10, 0, 0, 0, time.UTC)
	f.check(t)
	f.contactTitled(t, "Before activation", "Buyer")
	f.check(t)
	f.rules.SetClock(func() time.Time { return f.clock.Add(time.Second) })
	f.rule(t, "list_membership_notify", map[string]any{})
	if !f.deliver(t, "list.evaluated") {
		t.Fatal("the old committed check is missing")
	}
	if count := f.runs(t, "list_membership_notify"); count != 0 {
		t.Fatalf("old checks produced %d runs", count)
	}
	f.contactTitled(t, "After activation", "Buyer")
	f.check(t)
	if !f.deliver(t, "list.evaluated") {
		t.Fatal("the new committed check is missing")
	}
	if count := f.runs(t, "list_membership_notify"); count != 1 {
		t.Fatalf("new checks produced %d runs", count)
	}
}
