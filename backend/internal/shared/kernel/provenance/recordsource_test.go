// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestARecordSourceOutsideTheListIsRefusedNamingTheList(t *testing.T) {
	t.Parallel()
	for _, source := range RecordSources() {
		if err := AdmitRecordSource(source); err != nil {
			t.Errorf("AdmitRecordSource(%q) = %v, want nil", source, err)
		}
	}
	for _, source := range []string{"", "seed", "Manual", "connector:apollo:a-1", "manual "} {
		err := AdmitRecordSource(source)
		var refused *RecordSourceError
		if !errors.As(err, &refused) {
			t.Fatalf("AdmitRecordSource(%q) = %v, want a RecordSourceError", source, err)
		}
		var fault apperrors.FieldFault = refused
		field, code, message := fault.FieldFault()
		if field != "source" || code != "invalid_source" {
			t.Errorf("fault = %q/%q, want source/invalid_source", field, code)
		}
		for _, allowed := range RecordSources() {
			if !strings.Contains(message, allowed) {
				t.Errorf("refusal %q does not name the allowed source %q", message, allowed)
			}
		}
	}
}

func TestTheRecordSourcesAreSortedDistinctAndACopy(t *testing.T) {
	t.Parallel()
	sources := RecordSources()
	if !slices.IsSorted(sources) || len(slices.Compact(slices.Clone(sources))) != len(sources) {
		t.Errorf("RecordSources() = %v, want a sorted list with no duplicate", sources)
	}
	if !slices.Contains(sources, RecordSourceManual) {
		t.Errorf("RecordSources() = %v lacks %q", sources, RecordSourceManual)
	}
	sources[0] = "clobbered"
	if RecordSources()[0] == "clobbered" {
		t.Error("a caller's write reached the owner's own list")
	}
}

func TestTheRetiredSpellingsAreSortedAndHoldNoDuplicate(t *testing.T) {
	t.Parallel()
	retired := RetiredRecordSourceSpellings()
	if len(retired) == 0 {
		t.Fatal("an empty retired set makes every census over it pass having judged nothing")
	}
	if !slices.IsSorted(retired) {
		t.Errorf("the retired spellings are reported unsorted (%v), so a gate's findings reorder between runs", retired)
	}
	if compacted := slices.Compact(slices.Clone(retired)); len(compacted) != len(retired) {
		t.Errorf("a spelling is listed twice in %v, which reports one site as two findings", retired)
	}
}

func TestTheCanonicalWordIsNotAlsoRetired(t *testing.T) {
	t.Parallel()
	if slices.Contains(RetiredRecordSourceSpellings(), RecordSourceManual) {
		t.Errorf("%q is listed as retired, so the census would refuse the very spelling it exists to require", RecordSourceManual)
	}
}

func TestTheRetiredSetIsACopyTheCallerCannotWriteThrough(t *testing.T) {
	t.Parallel()
	first := RetiredRecordSourceSpellings()
	first[0] = "clobbered"
	if second := RetiredRecordSourceSpellings(); second[0] == "clobbered" {
		t.Error("a caller's write reached the owner's own set, so one gate can silently narrow another's corpus")
	}
}

// A wire that carries the channel alone must not send its caller to fields it
// cannot set.
func TestAChannelOnlyRefusalNamesNoOutsideIDFields(t *testing.T) {
	t.Parallel()
	if err := AdmitRecordChannel(RecordSourceManual); err != nil {
		t.Fatalf("AdmitRecordChannel(manual) = %v, want nil", err)
	}
	err := AdmitRecordChannel("signal-scan")
	var refused *RecordSourceError
	if !errors.As(err, &refused) {
		t.Fatalf("AdmitRecordChannel(signal-scan) = %v, want a RecordSourceError", err)
	}
	if _, code, message := refused.FieldFault(); code != "invalid_source" || strings.Contains(message, "source_system") {
		t.Errorf("the refusal reads %s %q, want invalid_source with no mention of source_system", code, message)
	}
}
