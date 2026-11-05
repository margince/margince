// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A tool write names the seats and records it changed as the inbox does. A
// view in a chat host has no way to look them up.
func TestAToolWriteNamesTheSeatsAndRecordsItChanged(t *testing.T) {
	e := integration.Setup(t)
	seats := identity.NewServiceFor(e.DB())
	for seat, name := range map[ids.UUID]string{e.Rep1: "Ana Rep", e.Rep2: "Sam Rep"} {
		if _, err := seats.SaveMyDisplayName(e.As(seat, []ids.UUID{e.Team1}, integration.AdminPerms), name); err != nil {
			t.Fatal(err)
		}
	}
	company := e.SeedCompany(t, "Harbor", &e.AdminUser)
	current := map[string]json.RawMessage{
		"owner_id":    json.RawMessage(`"` + e.Rep1.String() + `"`),
		"assignee_id": json.RawMessage(`null`),
		"company_id":  json.RawMessage(`null`),
	}
	changes := map[string]json.RawMessage{
		"owner_id":    json.RawMessage(`"` + e.Rep2.String() + `"`),
		"assignee_id": json.RawMessage(`"` + e.Rep1.String() + `"`),
		"company_id":  json.RawMessage(`"` + company.String() + `"`),
	}

	named, err := agentChangeWords(e.DB())(e.Admin(), current, changes)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string][2]string{
		"assignee_id": {"", "Ana Rep"},
		"company_id":  {"", "Harbor"},
		"owner_id":    {"Ana Rep", "Sam Rep"},
	}
	if len(named) != len(want) {
		t.Fatalf("named %d changes, want %d: %+v", len(named), len(want), named)
	}
	for _, change := range named {
		if got := [2]string{change.Before.Text, change.After.Text}; got != want[change.Field] {
			t.Errorf("%s reads %q, want %q", change.Field, got, want[change.Field])
		}
	}
}

func TestAToolWriteNamesAnAmountInItsCurrency(t *testing.T) {
	e := integration.Setup(t)
	current := map[string]json.RawMessage{"currency": json.RawMessage(`"EUR"`), "amount_minor": json.RawMessage(`null`)}
	changes := map[string]json.RawMessage{"amount_minor": json.RawMessage(`120000`)}

	named, err := agentChangeWords(e.DB())(e.Admin(), current, changes)
	if err != nil {
		t.Fatal(err)
	}

	if len(named) != 1 || named[0].After.Kind != "money" || named[0].After.Currency == nil || *named[0].After.Currency != "EUR" {
		t.Fatalf("amount reads %+v, want money in EUR", named)
	}
}
