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

func TestClearingAPopulatedCustomFieldWritesTheNull(t *testing.T) {
	// The direction that actually broke. An explicit null over a STORED value
	// used to be dropped, so the clear answered 200 and the old value stayed in
	// the column — a reader who emptied a field, reloaded, and found it full
	// again. Its sibling above covers null-over-null, which is the half that was
	// always right, and a suite holding only that half reports green over this.
	p := NewPatch()
	SetCustomFieldPatch(
		p,
		[]fieldcatalog.Column{{Name: "cf_note", Type: fieldcatalog.TypeText}},
		map[string]any{"cf_note": nil},
		map[string]any{"cf_note": "was here"},
	)
	if p.Empty() {
		t.Fatal("clearing a populated field produced no write")
	}
	if after, ok := p.After()["cf_note"]; !ok || after != nil {
		t.Fatalf("cf_note written as %v, want an explicit null", after)
	}
	if before := p.Before()["cf_note"]; before != "was here" {
		t.Fatalf("audit before = %v, want the value that was cleared", before)
	}
}
