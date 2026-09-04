// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package overlay

// What counts as "this field moved".
//
// The two sides come from different places — the mirror row's decoded JSONB and
// the adapter's mapping of the incumbent's answer — so the comparison has to
// see through REPRESENTATION without seeing through TYPE. Getting either wrong
// re-opens the defect this file's settledImages exists to close, in one
// direction or the other.

import (
	"encoding/json"
	"testing"
)

func TestOneValueInTwoRepresentationsIsNotAChange(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		a, b any
	}{
		{"float64 against json.Number", float64(1), json.Number("1")},
		{"json.Number against float64", json.Number("42"), float64(42)},
		{"two equal strings", "Ada", "Ada"},
		{"both absent", nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if !sameFieldValue(c.a, c.b) {
				t.Errorf("%#v and %#v read as a change — the trail would report a "+
					"field moving that nobody moved", c.a, c.b)
			}
		})
	}
}

// The inverse, and the reason this is not fmt: "%v" renders the string "1" and
// the number 1 identically, so a field that genuinely changed type would be
// dropped from audit_log and from the update event — silently, which is the
// worse half of the same defect.
func TestAChangeOfTypeIsStillAChange(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		a, b any
	}{
		{"the string 1 against the number 1", "1", float64(1)},
		{"the string 1 against json.Number 1", "1", json.Number("1")},
		{"true against the string true", true, "true"},
		{"a value against its absence", "Ada", nil},
		{"an absence against a value", nil, "Ada"},
		{"two different strings", "Ada", "Grace"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if sameFieldValue(c.a, c.b) {
				t.Errorf("%#v and %#v read as unchanged — the field would be "+
					"dropped from the trail", c.a, c.b)
			}
		})
	}
}
