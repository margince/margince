// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

// A unit offers the installation's custom deal fields with the deal. A value
// that differs is written, a repeat changes nothing, and a field a user last
// edited stays as the user left it and is named in Withheld.
func TestALandedDealSetsCustomFieldsAndKeepsOneAUserEdited(t *testing.T) {
	e := setupCRMIngress(t)
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	column := func(label string, kind string) string {
		field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{
			Object: "deal", Label: label, Type: kind, Source: "manual",
		})
		if err != nil {
			t.Fatalf("create the %s field: %v", label, err)
		}
		return *field.ColumnName
	}
	route, vehicle := column("Route", customfields.TypeText), column("Vehicle", customfields.TypeText)
	offer := func(routeValue string) extension.Record {
		return e.dealRecord("opp-1", func(d *extension.DealFields) {
			d.CustomFields = map[string]any{route: routeValue, vehicle: "VH-001"}
		})
	}

	made := e.mustIngest(t, offer("FRA to NBO"))
	id := ids.From[ids.DealKind](ids.MustParse(made.Ref.ID))
	read := func(key string) any {
		deal, err := e.Deals.WithFieldCatalog(catalog).GetDeal(e.Admin(), id, storekit.LiveOnly)
		if err != nil {
			t.Fatalf("read the deal: %v", err)
		}
		return deal.AdditionalProperties[key]
	}
	if got := read(route); got != "FRA to NBO" {
		t.Fatalf("route = %v after the first landing, want the offered value", got)
	}

	if res := e.mustIngest(t, offer("FRA to NBO")); len(res.Withheld) != 0 {
		t.Errorf("withheld = %v on a repeat, want none", res.Withheld)
	}
	e.mustIngest(t, offer("FRA to MBA"))
	if got := read(route); got != "FRA to MBA" {
		t.Errorf("route = %v after the source changed it, want the new value", got)
	}

	if _, err := e.Deals.WithFieldCatalog(catalog).UpdateDeal(e.Admin(), id,
		deals.UpdateDealInput{CustomFields: map[string]any{route: "typed by a user"}}); err != nil {
		t.Fatalf("editing as a user: %v", err)
	}
	res := e.mustIngest(t, offer("FRA to LOS"))
	if !slices.Equal(res.Withheld, []string{route}) {
		t.Errorf("withheld = %v, want only the field the user edited", res.Withheld)
	}
	if got := read(route); got != "typed by a user" {
		t.Errorf("route = %v, want the user's value kept", got)
	}
}
