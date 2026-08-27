// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"errors"
	"testing"
)

// A cleared field's audit image must say what it was cleared FROM. The row is
// gone after the write, so the image is the only place the old value survives.
func TestApplyClearsRecordsWhatItCleared(t *testing.T) {
	p := NewPatch()
	if err := ApplyClears(p, []string{"title"}, map[string]Clearable{
		"title": {Column: "title", Current: "Head of Ops"},
	}); err != nil {
		t.Fatalf("ApplyClears: %v", err)
	}
	if got := p.Before()["title"]; got != "Head of Ops" {
		t.Fatalf("before image title = %v, want %q", got, "Head of Ops")
	}
	got, present := p.After()["title"]
	if !present {
		t.Fatal("after image has no title key; a clear must record the NULL it wrote")
	}
	if got != nil {
		t.Fatalf("after image title = %v, want nil", got)
	}
}

// A field the map does not hold is refused by name, carrying the wire code the
// 422 seam publishes. Accepting it would answer 200 having changed nothing —
// a success the caller cannot trust.
func TestApplyClearsRefusesAFieldItCannotClear(t *testing.T) {
	err := ApplyClears(NewPatch(), []string{"full_name"}, map[string]Clearable{})
	var refusal *NotClearableError
	if !errors.As(err, &refusal) {
		t.Fatalf("ApplyClears error = %v, want *NotClearableError", err)
	}
	field, code, message := refusal.FieldFault()
	if field != "full_name" {
		t.Errorf("FieldFault field = %q, want %q", field, "full_name")
	}
	if code != "field_not_clearable" {
		t.Errorf("FieldFault code = %q, want %q", code, "field_not_clearable")
	}
	if message == "" {
		t.Error("FieldFault message is empty; a refusal must say what to do instead")
	}
}

// The refusal stops at the first field it cannot clear and writes nothing.
// A half-applied clear would report a failure having already changed the row.
func TestApplyClearsWritesNothingWhenOneFieldIsRefused(t *testing.T) {
	p := NewPatch()
	err := ApplyClears(p, []string{"full_name", "title"}, map[string]Clearable{
		"title": {Column: "title", Current: "Head of Ops"},
	})
	if err == nil {
		t.Fatal("ApplyClears accepted a field it cannot clear")
	}
	if !p.Empty() {
		t.Fatalf("patch holds %v after a refusal, want empty", p.After())
	}
}
