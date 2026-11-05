// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

const vietnameseSignature = "Trân trọng,\nLê Thị Mai\nGiám đốc\nAcme VN"

func TestGateSignatureNameMapsPrintedPartsToColumns(t *testing.T) {
	t.Parallel()
	reply := `{"fields":[],"name":{"family_name":"Lê","given_names":"Mai","middle":"Thị","order":"family_first","evidence_snippet":"Lê Thị Mai","confidence":0.9}}`
	fields, dropped := gateSignatureName(reply, vietnameseSignature)
	if len(dropped) != 0 || len(fields) != 2 {
		t.Fatalf("fields %v, dropped %v; want the two name columns", fields, dropped)
	}
	got := map[string]string{}
	for _, f := range fields {
		got[f.Name] = f.Value
		if f.Display != "Lê Thị Mai" || f.Evidence != "Lê Thị Mai" || f.ClaimKey == "" {
			t.Errorf("%s: display %q evidence %q claim key %q", f.Name, f.Display, f.Evidence, f.ClaimKey)
		}
	}
	if got[contacts.FieldFirstName] != "Mai" || got[contacts.FieldLastName] != "Lê" {
		t.Errorf("columns = %v, want first Mai and last Lê", got)
	}
}

func TestGateSignatureNameWritesNothingWhenTheReadingCannotBeTrusted(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		name   string
		reason string
	}{
		"unclear order is an answer, not a refusal": {
			`{"name":{"family_name":"","given_names":"","middle":"","order":"unclear","evidence_snippet":"Lê Thị Mai","confidence":0.9}}`, "",
		},
		"snippet not on the page": {
			`{"name":{"family_name":"Lê","given_names":"Mai","middle":"","order":"family_first","evidence_snippet":"Lê Mai","confidence":0.9}}`, dropEvidenceNotOnPage,
		},
		"confidence below the floor": {
			`{"name":{"family_name":"Lê","given_names":"Mai","middle":"Thị","order":"family_first","evidence_snippet":"Lê Thị Mai","confidence":0.4}}`, dropBelowFloor,
		},
		"confidence out of range": {
			`{"name":{"family_name":"Lê","given_names":"Mai","middle":"Thị","order":"family_first","evidence_snippet":"Lê Thị Mai","confidence":1.5}}`, dropConfidenceRange,
		},
		"parts in the other order than the page prints": {
			`{"name":{"family_name":"Lê","given_names":"Mai","middle":"Thị","order":"given_first","evidence_snippet":"Lê Thị Mai","confidence":0.9}}`, dropNameUnmappable,
		},
		"no snippet": {
			`{"name":{"family_name":"Lê","given_names":"Mai","middle":"Thị","order":"family_first","evidence_snippet":"","confidence":0.9}}`, dropEmptyEvidence,
		},
	}
	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			fields, dropped := gateSignatureName(tc.name, vietnameseSignature)
			if len(fields) != 0 {
				t.Fatalf("wrote %v, want nothing", fields)
			}
			if tc.reason == "" && len(dropped) != 0 {
				t.Fatalf("dropped %v, want an unrefused abstention", dropped)
			}
			if tc.reason != "" && (len(dropped) != 1 || dropped[0].Reason != tc.reason) {
				t.Fatalf("dropped %v, want reason %s", dropped, tc.reason)
			}
		})
	}
}

func TestSignatureNameIsNeverOfferedAsAField(t *testing.T) {
	t.Parallel()
	for _, column := range []string{contacts.FieldFirstName, contacts.FieldLastName} {
		if enrichFieldNames[column] {
			t.Errorf("%s is offered to the model as a field; the name object is the only way to state it", column)
		}
	}
	if err := signatureShapeValid(`{"fields":[],"name":{"family_name":"Lê","given_names":"Mai","middle":"","order":"first_last","evidence_snippet":"x","confidence":0.9}}`); err == nil {
		t.Error("an order outside given_first, family_first and unclear passed the shape check")
	}
	if err := signatureShapeValid(`{"fields":[{"field":"first_name","value":"Mai","evidence_snippet":"Mai","confidence":0.9}]}`); err == nil {
		t.Error("first_name as a field passed the shape check")
	}
	if !strings.Contains(string(signatureEnrichSchema()), `"family_name"`) {
		t.Error("the reply schema does not carry the name object")
	}
}
