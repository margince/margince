// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importcells

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func TestEveryCellIsCheckedAgainstItsFieldsType(t *testing.T) {
	picklist := fieldcatalog.Column{Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold", "Silver"}}
	multi := fieldcatalog.Column{Name: "cf_comp", Type: fieldcatalog.TypeMultiselect, Options: []string{"HubSpot", "Other"}}
	money := fieldcatalog.Column{Name: "cf_value", Type: fieldcatalog.TypeCurrency, Currency: "EUR"}
	vnd := fieldcatalog.Column{Name: "cf_vnd", Type: fieldcatalog.TypeCurrency, Currency: "VND"}
	noCurrency := fieldcatalog.Column{Name: "cf_amount", Type: fieldcatalog.TypeCurrency}
	date := fieldcatalog.Column{Name: "cf_renewal", Type: fieldcatalog.TypeDate}
	number := fieldcatalog.Column{Name: "cf_seats", Type: fieldcatalog.TypeNumber}
	flag := fieldcatalog.Column{Name: "cf_active", Type: fieldcatalog.TypeBoolean}
	for _, tc := range []struct {
		col     fieldcatalog.Column
		raw     string
		want    any
		problem string
	}{
		{picklist, "gold", "Gold", ""},
		{picklist, "  Silver ", "Silver", ""},
		{picklist, "Bronze", nil, "not one of the allowed values: Gold, Silver"},
		{multi, "hubspot; Other", []string{"HubSpot", "Other"}, ""},
		{multi, "HubSpot, Other", nil, "separate several values with semicolons"},
		{multi, "HubSpot; Bronze", nil, "not one of the allowed values: HubSpot, Other"},
		{money, "12000.50", float64(1200050), ""},
		{money, "12.000,50", nil, "not an amount in EUR"},
		{money, "€ 12,000", nil, "not an amount in EUR"},
		{money, "12k", nil, "not an amount in EUR"},
		{vnd, "1500000", float64(1500000), ""},
		{vnd, "1500000.5", nil, "more decimal places than VND takes. Write it like 12000."},
		{money, "12.345", nil, "more decimal places than EUR takes. Write it like 12000.50."},
		{money, "-500", nil, "is below zero"},
		{money, "1e5", nil, "not an amount in EUR. Write it like 12000.50."},
		{noCurrency, "12000.50", nil, "has no currency set"},
		{date, "2026-03-02", "2026-03-02", ""},
		{date, "03/02/2026", nil, "Write it as YYYY-MM-DD"},
		{date, "2.3.2026", nil, "Write it as YYYY-MM-DD"},
		{date, "2026-03-02 10:00", nil, "Write it as YYYY-MM-DD"},
		{date, "45351", nil, "Write it as YYYY-MM-DD"},
		{number, "24", "24", ""},
		{number, "24 seats", nil, "is not a number"},
		{flag, "Yes", true, ""},
		{flag, "0", false, ""},
		{flag, "maybe", nil, "is not yes or no"},
	} {
		got, why := FieldValue(tc.col, tc.raw, true)
		if tc.problem == "" {
			if why.Code != "" || !reflect.DeepEqual(got, tc.want) {
				t.Errorf("%s %q → %v, %+v; want %v", tc.col.Type, tc.raw, got, why, tc.want)
			}
			continue
		}
		if why.Code == "" || got != nil || !strings.Contains(why.English(), tc.problem) {
			t.Errorf("%s %q → %v, %+v; want a refusal saying %q", tc.col.Type, tc.raw, got, why, tc.problem)
		}
	}
}

func TestAPicklistProblemNamesNoOptionToACallerWithoutCustomFieldRead(t *testing.T) {
	picklist := fieldcatalog.Column{Name: "cf_tier", Type: fieldcatalog.TypePicklist, Options: []string{"Gold", "Silver"}}
	multi := fieldcatalog.Column{Name: "cf_comp", Type: fieldcatalog.TypeMultiselect, Options: []string{"HubSpot", "Other"}}
	for _, tc := range []struct {
		col fieldcatalog.Column
		raw string
	}{{picklist, "Bronze"}, {multi, "HubSpot; Bronze"}} {
		_, why := FieldValue(tc.col, tc.raw, false)
		message := why.English()
		if !strings.Contains(message, "not one of this field’s allowed values") {
			t.Fatalf("%s %q → %+v, want the refusal without the list", tc.col.Type, tc.raw, why)
		}
		for _, option := range tc.col.Options {
			if strings.Contains(message, option) && !strings.Contains(tc.raw, option) {
				t.Errorf("%s %q → %q names the option %q", tc.col.Type, tc.raw, message, option)
			}
		}
	}
}
