// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"testing"
)

// limitedNote is a typed length refusal that states its limit.
type limitedNote struct{ max int }

func (e *limitedNote) Error() string { return "note is too long" }
func (e *limitedNote) FieldFault() (field, code, message string) {
	return "note", "too_long", e.Error()
}
func (e *limitedNote) FieldMax() int { return e.max }

// unlimitedNote is the same refusal from a type that states no limit.
type unlimitedNote struct{}

func (e *unlimitedNote) Error() string { return "note is too long" }
func (e *unlimitedNote) FieldFault() (field, code, message string) {
	return "note", "too_long", e.Error()
}

// firstFieldFault is details.errors[0] of a decoded 422 body.
func firstFieldFault(t *testing.T, err error) map[string]any {
	t.Helper()
	status, body := writeAndDecode(t, err)
	if status != 422 {
		t.Fatalf("status = %d, want 422", status)
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("body has no details: %v", body)
	}
	faults, ok := details["errors"].([]any)
	if !ok || len(faults) != 1 {
		t.Fatalf("details.errors = %v, want one entry", details["errors"])
	}
	fault, ok := faults[0].(map[string]any)
	if !ok {
		t.Fatalf("details.errors[0] = %v, want an object", faults[0])
	}
	return fault
}

func TestTooLongPutsTheLimitOnTheWire(t *testing.T) {
	t.Parallel()
	fault := firstFieldFault(t, TooLong("note", 10000, 10001))
	if fault["code"] != "too_long" || fault["field"] != "note" {
		t.Errorf("fault = %v, want field note and code too_long", fault)
	}
	if fault["max"] != float64(10000) {
		t.Errorf("max = %v, want 10000", fault["max"])
	}
}

func TestRequireWithinStatesItsLimit(t *testing.T) {
	t.Parallel()
	fault := firstFieldFault(t, RequireWithin("note", "abcdef", 5))
	if fault["max"] != float64(5) {
		t.Errorf("max = %v, want 5", fault["max"])
	}
}

func TestATypedFieldFaultStatesItsLimitWhenItHasOne(t *testing.T) {
	t.Parallel()
	if fault := firstFieldFault(t, &limitedNote{max: 2000}); fault["max"] != float64(2000) {
		t.Errorf("max = %v, want 2000", fault["max"])
	}
	if fault := firstFieldFault(t, &unlimitedNote{}); fault["max"] != nil {
		t.Errorf("max = %v, want no key for a refusal that states no limit", fault["max"])
	}
}

func TestTooLongRefusesALimitThatStatesNone(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("TooLong with limit %d returned; want a panic, since the wire would carry no max", limit)
				}
			}()
			if err := TooLong("note", limit, 1); err != nil {
				t.Errorf("TooLong with limit %d returned %v; want a panic", limit, err)
			}
		}()
	}
}
