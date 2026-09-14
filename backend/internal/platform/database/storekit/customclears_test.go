// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func TestCustomClearsRequireAnActiveColumnAndAnExplicitNull(t *testing.T) {
	active := []fieldcatalog.Column{{Name: "cf_note", Type: fieldcatalog.TypeText}}
	for _, test := range []struct {
		name    string
		updates map[string]any
		want    []string
	}{
		{"null", map[string]any{"cf_note": nil}, []string{"title", "cf_unknown"}},
		{"missing", nil, []string{"title", "cf_note", "cf_unknown"}},
		{"value", map[string]any{"cf_note": "retained"}, []string{"title", "cf_note", "cf_unknown"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := CoreFieldClears([]string{"title", "cf_note", "cf_unknown"}, active, test.updates)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("core clears = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClearingAnUnsetCustomFieldDoesNotProduceAWrite(t *testing.T) {
	p := NewPatch()
	SetCustomFieldPatch(p, []fieldcatalog.Column{{Name: "cf_note", Type: fieldcatalog.TypeText}}, map[string]any{"cf_note": nil}, map[string]any{"cf_note": nil})
	if !p.Empty() {
		t.Fatal("clearing an unset field produced a patch")
	}
}
