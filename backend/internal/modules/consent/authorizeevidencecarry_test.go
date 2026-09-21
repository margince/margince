// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The evidence round-trip, and the three shapes a row can hold that are not a
// round-trip at all.
//
// Unit, not integration: this is a pure encode/decode pair, and the question it
// answers — does a delivery staged before this code shipped still dispatch — is
// about what the decoder does with the bytes, not about any join.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Every id a caller can name survives the trip to the column and back.
//
// Asserted field by field rather than with a struct compare, so a field ADDED
// to Evidence and forgotten in evidenceJSON fails here naming itself, rather
// than failing as "the structs differ" somewhere a reader has to diff by eye.
func TestEveryEvidenceIDSurvivesTheRoundTrip(t *testing.T) {
	in := commsauthz.Evidence{
		ActivityID:     ids.NewV7(),
		DealID:         ids.NewV7(),
		InvoiceID:      ids.NewV7(),
		ContractID:     ids.NewV7(),
		ConsentEventID: ids.NewV7(),
		BasisID:        ids.NewV7(),
	}
	raw, err := evidenceJSON(in)
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	got := evidenceFrom(raw)

	for _, c := range []struct {
		field    string
		want, is ids.UUID
	}{
		{"activity", in.ActivityID, got.ActivityID},
		{"deal", in.DealID, got.DealID},
		{"invoice", in.InvoiceID, got.InvoiceID},
		{"contract", in.ContractID, got.ContractID},
		{"consent event", in.ConsentEventID, got.ConsentEventID},
		{"basis", in.BasisID, got.BasisID},
	} {
		if c.is != c.want {
			t.Errorf("the %s id did not survive: got %v, want %v", c.field, c.is, c.want)
		}
	}
}

// A DELIVERY STAGED BEFORE THIS CODE SHIPPED STILL DISPATCHES.
//
// communication_decision.evidence defaults to '{}', so every row written by
// the old staging writer holds an empty object rather than NULL. It must decode
// to no evidence — which is exactly the input the transmit phase had before
// this file existed, so such a delivery takes the path it always took.
//
// The unreadable case is the one that matters most and is the least likely to
// be tried by hand: a column somebody edited, or a shape written by a version
// that disagreed about the field names. It yields no evidence rather than an
// error, because failing the decode would turn one malformed row into a stuck
// delivery, and the ids are all re-validated at transmit anyway.
func TestAnUnusableEvidenceColumnYieldsNoEvidenceRatherThanAnError(t *testing.T) {
	for _, c := range []struct {
		name string
		raw  []byte
	}{
		{"the column default an old row holds", []byte(`{}`)},
		{"a NULL the driver hands over as nil", nil},
		{"an empty string", []byte(``)},
		{"not json at all", []byte(`{ruined`)},
		{"json of the wrong shape", []byte(`["an","array"]`)},
		{"an id that is not a uuid", []byte(`{"invoice_id":"not-a-uuid"}`)},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := evidenceFrom(c.raw)
			if got != (commsauthz.Evidence{}) {
				t.Errorf("decoded %v, want no evidence at all", got)
			}
		})
	}
}

// An empty Evidence renders as the column's own default rather than as a row of
// nulls, so a decision taken on no named record reads as one.
func TestNoEvidenceRendersAsTheEmptyObject(t *testing.T) {
	raw, err := evidenceJSON(commsauthz.Evidence{})
	if err != nil {
		t.Fatalf("rendering: %v", err)
	}
	if string(raw) != `{}` {
		t.Errorf("rendered %s, want {}", raw)
	}
}
