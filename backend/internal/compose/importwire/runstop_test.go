// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// A refused test run keeps the code and values of its sentence; a refusal that
// names no field keeps the sentence alone.
func TestATestRunStopKeepsTheCodeOfItsFileRefusal(t *testing.T) {
	t.Parallel()
	damaged := migration.InEntry("leads.csv", &migration.FileFault{
		Kind:    migration.ErrSourceUnreadable,
		Message: "Line 7 has more or fewer fields than the header row.",
		Params:  migration.Params{"reason": "field_count", "line": "7"},
	})
	fault, ok := httperr.Classify(Problem(damaged))
	if !ok {
		t.Fatalf("a damaged line classifies as no fault: %v", damaged)
	}
	stop := StopOf(fault, damaged)
	want := migration.StopRefusal{
		Field: "file", Code: "unreadable",
		Params: map[string]string{"reason": "field_count", "line": "7", "file": "leads.csv"},
	}
	if stop.Cause != fault.Detail || stop.Refusal == nil || !reflect.DeepEqual(*stop.Refusal, want) {
		t.Errorf("the stop of a damaged line = %q with %+v, want %q with %+v", stop.Cause, stop.Refusal, fault.Detail, want)
	}
	plain := httperr.Fault{Status: 404, Detail: "the uploaded file is no longer stored"}
	if got := StopOf(plain, errors.New("gone")); got.Refusal != nil {
		t.Errorf("a refusal naming no field keeps %+v, want none", got.Refusal)
	}
}

func TestARunCarriesItsRefusalBesideItsSentence(t *testing.T) {
	t.Parallel()
	got := RunRefusal(&migration.StopRefusal{Field: "file", Code: "unreadable", Params: map[string]string{"line": "7"}})
	if got == nil || got.Field != "file" || got.Code != "unreadable" || got.Max != nil || got.Params == nil || (*got.Params)["line"] != "7" {
		t.Errorf("error_refusal = %+v, want file/unreadable with line 7 and no max", got)
	}
	if limited := RunRefusal(&migration.StopRefusal{Field: "file", Code: "too_many_rows", Max: 5000}); limited.Max == nil || *limited.Max != 5000 || limited.Params != nil {
		t.Errorf("a refusal at a limit = %+v, want max 5000 and no params", limited)
	}
	if RunRefusal(nil) != nil {
		t.Error("a run with a sentence only answers an error_refusal, want none")
	}
}
