// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"reflect"
	"testing"
)

func TestAColumnHoldingASliceIsComparedWithoutPanicking(t *testing.T) {
	before, after := ChangedColumns(
		map[string]any{"social": []string{"linkedin"}},
		map[string]any{"social": []string{"linkedin", "x"}},
	)
	if !reflect.DeepEqual(before, map[string]any{"social": []string{"linkedin"}}) {
		t.Errorf("before image lost the prior slice: %v", before)
	}
	if !reflect.DeepEqual(after, map[string]any{"social": []string{"linkedin", "x"}}) {
		t.Errorf("after image lost the new slice: %v", after)
	}
}

func TestASliceColumnThatDidNotMoveIsNarrowedAway(t *testing.T) {
	before, after := ChangedColumns(
		map[string]any{"social": []string{"linkedin"}},
		map[string]any{"social": []string{"linkedin"}},
	)
	if len(before) != 0 || len(after) != 0 {
		t.Errorf("an unchanged slice column was published as a change: %v -> %v", before, after)
	}
}

// A cleared column moves too. The narrowing reads the union of both images, so
// a column the write emptied is a change from its old value to nothing — the
// pair that only walked `after` recorded no change at all for it.
func TestAColumnClearedToAbsenceIsStillAChange(t *testing.T) {
	before, after := ChangedColumns(
		map[string]any{"industry": "Automotive"},
		map[string]any{},
	)
	if before["industry"] != "Automotive" {
		t.Errorf("the cleared column's prior value was dropped: %v", before)
	}
	if value, present := after["industry"]; !present || value != nil {
		t.Errorf("a cleared column reads as nil in the after image, got %v (present=%t)", value, present)
	}
}

func TestAnUntouchedColumnIsNotPublishedAsAChange(t *testing.T) {
	before, after := ChangedColumns(
		map[string]any{"industry": "Automotive", "legal_name": nil},
		map[string]any{"industry": "Automotive", "legal_name": "Acme GmbH"},
	)
	if _, present := before["industry"]; present {
		t.Errorf("an untouched column reached the before image: %v", before)
	}
	if after["legal_name"] != "Acme GmbH" {
		t.Errorf("the changed column was dropped: %v", after)
	}
}

func TestAColumnAbsentFromBeforeIsAChangeFromNil(t *testing.T) {
	before, after := ChangedColumns(
		map[string]any{},
		map[string]any{"legal_name": "Acme GmbH"},
	)
	if value, present := before["legal_name"]; !present || value != nil {
		t.Errorf("a first write reads as nil in the before image, got %v (present=%t)", value, present)
	}
	if after["legal_name"] != "Acme GmbH" {
		t.Errorf("the written column was dropped: %v", after)
	}
}

func TestNilMapsNarrowToAnEmptyPairWithoutPanicking(t *testing.T) {
	before, after := ChangedColumns(nil, nil)
	if len(before) != 0 || len(after) != 0 {
		t.Errorf("two absent images narrowed to %v -> %v", before, after)
	}
}

// A column holding a nil under one spelling and an untyped nil under another is
// the same absence. Without this, a write that only re-stated an empty column
// would publish it as a change on the field-history screen.
func TestTwoSpellingsOfAnAbsentValueAreNotAChange(t *testing.T) {
	var typed []string
	before, after := ChangedColumns(
		map[string]any{"social": nil},
		map[string]any{"social": typed},
	)
	if len(before) != 0 || len(after) != 0 {
		t.Errorf("an absence was published as a change: %v -> %v", before, after)
	}
}
