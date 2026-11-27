// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestPromotionRefusesAnEmailPastTheAddressLimit(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	address := strings.Repeat("x", values.MaxEmailLength) + "@legacy.test"
	id := seedHistoricalLeadWithEmail(ctx, t, e.store, CreateLeadInput{
		FullName: new("Legacy address"), Source: "manual",
	}, address)
	before, err := e.store.GetLead(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	rows := writeShapeRows(ctx, t, e)

	_, _, err = e.store.PromoteLead(ctx, id, PromoteLeadInput{Trigger: "human_qualify"})
	fault, ok := errors.AsType[*values.ParseError](err)
	if !ok || fault.Field != "email" || fault.Code != "too_long" || fault.Max != values.MaxEmailLength {
		t.Fatalf("promotion = %v, want email/too_long with max %d", err, values.MaxEmailLength)
	}
	if after := writeShapeRows(ctx, t, e); after != rows {
		t.Errorf("refused promotion changed contact, audit and event rows from %v to %v", rows, after)
	}
	held, err := e.store.GetLead(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if held.Email == nil {
		t.Fatal("refused promotion removed the lead's address")
	}
	if held.Status != before.Status || string(*held.Email) != address {
		t.Errorf("refused promotion changed the lead: status %s, email %q", held.Status, *held.Email)
	}
}
