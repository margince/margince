// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func optionColumn(name, typ string, options ...string) fieldcatalog.Column {
	return fieldcatalog.Column{Name: name, Type: typ, Options: options}
}

func TestEveryRefusedCustomKeyIsNamedInOneAnswer(t *testing.T) {
	active := []fieldcatalog.Column{col("cf_score", fieldcatalog.TypeNumber)}
	_, err := CustomBindValues(active, map[string]any{"cf_nmae": "x", "cf_score": "n/a", "cf_other": 1})
	refused, ok := errors.AsType[CustomFieldErrors](err)
	if !ok || len(refused) != 3 {
		t.Fatalf("got %v, want all three keys refused", err)
	}
	faults := refused.FieldFaults()
	if faults[0].Field != "cf_nmae" || faults[1].Field != "cf_other" || faults[2].Field != "cf_score" {
		t.Fatalf("the keys came back as %+v, want them in key order", faults)
	}
	if refused.Error() == refused[0].Error() {
		t.Fatalf("the summary %q hides that more keys were refused", refused.Error())
	}
}

func TestAPicklistOrMultiselectValueMustBeOneOfTheCatalogsOptions(t *testing.T) {
	active := []fieldcatalog.Column{
		optionColumn("cf_tier", fieldcatalog.TypePicklist, "gold", "silver"),
		optionColumn("cf_tags", fieldcatalog.TypeMultiselect, "a", "b"),
	}
	for name, values := range map[string]map[string]any{
		"a picklist outside the options":    {"cf_tier": "bronze"},
		"a multiselect choice outside them": {"cf_tags": []any{"a", "z"}},
	} {
		if err := CheckCustomValues(active, values); err == nil {
			t.Errorf("%s was accepted", name)
		} else if refused, ok := errors.AsType[CustomFieldErrors](err); !ok || refused[0].Code != CustomFieldInvalid {
			t.Errorf("%s was answered %v, want invalid_value", name, err)
		}
	}
	if err := CheckCustomValues(active, map[string]any{"cf_tier": "gold", "cf_tags": []any{"a", "b"}}); err != nil {
		t.Fatalf("a listed option was refused: %v", err)
	}
}

func TestANullForAKeyNoColumnNamesIsHarmless(t *testing.T) {
	bound, err := CustomBindValues(nil, map[string]any{"cf_retired": nil})
	if err != nil {
		t.Fatalf("a form that cached a retired field could not save: %v", err)
	}
	values, err := CreateFieldValues(nil, map[string]any{"cf_retired": nil})
	if err != nil || len(values) != 0 {
		t.Fatalf("a create stored %v (%v), want nothing for a null", values, err)
	}
	if _, kept := bound["cf_retired"]; !kept || bound["cf_retired"] != nil {
		t.Fatalf("the null was not carried as a clear: %v", bound)
	}
}
