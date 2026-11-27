// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package provenance

import (
	"strings"
	"testing"
)

func TestAdmitSourceID(t *testing.T) {
	long := strings.Repeat("é", SourceIDMax+1)
	cases := []struct {
		name         string
		system, id   *string
		wantCode     string
		wantFieldKey string
	}{
		{name: "no id is no claim", system: new("hubspot")},
		{name: "a pair is admitted", system: new("hubspot"), id: new("42")},
		{name: "an id at the cap is admitted", system: new("hubspot"), id: new(strings.Repeat("é", SourceIDMax))},
		{name: "an id without a system", id: new("42"), wantCode: "source_id_needs_a_source", wantFieldKey: "source_id"},
		{name: "an id with an empty system", system: new(""), id: new("42"), wantCode: "source_id_needs_a_source", wantFieldKey: "source_id"},
		{name: "an empty id", system: new("hubspot"), id: new(""), wantCode: "source_id_invalid", wantFieldKey: "source_id"},
		{name: "an id over the cap", system: new("hubspot"), id: new(long), wantCode: "source_id_invalid", wantFieldKey: "source_id"},
		{name: "a system over the cap", system: new(long), wantCode: "source_system_invalid", wantFieldKey: "source_system"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AdmitSourceID(tc.system, tc.id)
			if tc.wantCode == "" {
				if err != nil {
					t.Fatalf("AdmitSourceID = %v, want admitted", err)
				}
				return
			}
			fault, ok := err.(*SourceIDError)
			if !ok {
				t.Fatalf("AdmitSourceID = %v, want a *SourceIDError", err)
			}
			if field, code, _ := fault.FieldFault(); field != tc.wantFieldKey || code != tc.wantCode {
				t.Fatalf("refused as %s/%s, want %s/%s", field, code, tc.wantFieldKey, tc.wantCode)
			}
		})
	}
}

func TestASourceIDRefusalReadsAsItsMessage(t *testing.T) {
	err := CheckSourceValues(nil, new(""))
	if err == nil || err.Error() == "" {
		t.Fatalf("an empty source_id = %v, want a refusal that states itself", err)
	}
}

func TestASourcePastTheCapIsRefusedWithItsLimit(t *testing.T) {
	if err := RefuseSource(strings.Repeat("é", SourceIDMax)); err != nil {
		t.Fatalf("a source at the cap = %v, want admitted", err)
	}
	fault, ok := RefuseSource(strings.Repeat("é", SourceIDMax+1)).(*SourceIDError)
	if !ok {
		t.Fatal("a source past the cap was admitted, want a *SourceIDError")
	}
	if field, code, _ := fault.FieldFault(); field != "source" || code != "too_long" || fault.FieldMax() != SourceIDMax {
		t.Errorf("refused as %s/%s max %d, want source/too_long max %d", field, code, fault.FieldMax(), SourceIDMax)
	}
	if _, reserved := RefuseSource(ReservedSourceSystemPrefix + "crm").(*ReservedError); !reserved {
		t.Error("a short source in the reserved namespace must still be refused as reserved")
	}
}
