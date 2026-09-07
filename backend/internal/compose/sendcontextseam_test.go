// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The seam between the tool surface's send arguments and the module's.
//
// It is one mapping function, and it silently dropped evidence: the tool
// surface offered no evidence field, so the seam had nothing to carry and the
// module's own field sat unfilled. An agent claiming invoice_or_payment,
// contract_notice or active_deal_followup — three categories this surface
// admits and whose validators read the named record — resolved to `review` and
// parked, which reads to a rep as the tool being broken.
//
// Nothing tested this mapping, which is how the drop survived. A field added to
// either side and forgotten here is invisible in exactly the same way, so the
// census below asks the question structurally rather than field by field.

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

func TestTheSendContextSeamCarriesEveryEvidenceField(t *testing.T) {
	t.Parallel()

	args := agents.SendContextArgs{
		CommunicationContext: "invoice_or_payment",
		MarketingPurpose:     "newsletter",
		OperatorReason:       "they asked",
		Evidence: agents.SendEvidenceArgs{
			InvoiceID:  "01a05500-0000-7000-8000-000000000001",
			ContractID: "01a05500-0000-7000-8000-000000000002",
			DealID:     "01a05500-0000-7000-8000-000000000003",
		},
	}
	got := sendContextOf(args)

	if got.Context != args.CommunicationContext ||
		got.MarketingPurpose != args.MarketingPurpose ||
		got.OperatorReason != args.OperatorReason {
		t.Errorf("the seam dropped a context field: %+v", got)
	}
	if got.Evidence.InvoiceID != args.Evidence.InvoiceID ||
		got.Evidence.ContractID != args.Evidence.ContractID ||
		got.Evidence.DealID != args.Evidence.DealID {
		t.Errorf("the seam dropped an evidence field: %+v", got.Evidence)
	}

	// EVERY field, not the four named above. A fifth added to the tool surface
	// and forgotten in the seam is the defect this test exists for, and naming
	// them one by one would leave it exactly as invisible as it was.
	//
	// Both sides are flat structs of strings, so a non-empty input mapping to a
	// non-empty output is the whole question: a field the seam never assigns
	// stays at its zero value.
	in := reflect.ValueOf(args.Evidence)
	out := reflect.ValueOf(got.Evidence)
	if in.NumField() != out.NumField() {
		t.Fatalf("the two evidence shapes carry %d and %d fields: the seam cannot map "+
			"one onto the other and this census has stopped meaning anything",
			in.NumField(), out.NumField())
	}
	// PAIRED BY NAME, not by index. Matching on position would pass a seam that
	// mapped InvoiceID onto ContractID — the two are both non-empty strings, so
	// every value would arrive and none would arrive where it belonged.
	for i := range in.NumField() {
		name := in.Type().Field(i).Name
		if in.Field(i).String() == "" {
			t.Fatalf("%s is empty in the fixture, so this census does not read it", name)
		}
		paired := out.FieldByName(name)
		if !paired.IsValid() {
			t.Errorf("%s has no field of that name on the module's shape: the seam "+
				"cannot be carrying it", name)
			continue
		}
		if paired.String() != in.Field(i).String() {
			t.Errorf("%s reached the module as %q, want %q: the seam drops or mis-wires it",
				name, paired.String(), in.Field(i).String())
		}
	}
}
