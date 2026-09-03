// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A scheduled message comes back out exactly as it went in.
//
// The freeze/thaw pair is the one place a send's own description can narrow
// without anything failing: a field added to SendEmailInput and not to
// scheduledPayload is dropped silently, and the send still goes — just
// describing itself as less than it was. That failed safe for the context
// fields (an empty claim records NULL rather than a false claim) and would not
// have for marketing_purpose, whose whole job is to scope a grant to one topic:
// a send scheduled for one minute from now would have arrived with no purpose
// to check against.
//
// So this compares field by field through reflection rather than asserting the
// handful somebody remembered. The next field added to SendEmailInput fails
// here until it is carried.

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// A fully-populated input survives the round trip with every field intact.
func TestAScheduledSendCarriesEveryFieldOfItsInput(t *testing.T) {
	in := SendEmailInput{
		Recipients:       []string{"buyer@example.test", "cc@example.test"},
		Cc:               []string{"cc@example.test"},
		Bcc:              []string{"blind@example.test"},
		Subject:          "Your quote",
		Body:             "As discussed.",
		HTMLBody:         "<p>As discussed.</p>",
		AttachmentIDs:    []ids.UUID{ids.NewV7()},
		ConsentPurpose:   "marketing_email",
		DraftRef:         "draft-7",
		Context:          commsauthz.CategoryMarketing,
		MarketingPurpose: "newsletter",
		OperatorReason:   "they asked at the fair",
		Evidence: commsauthz.Evidence{
			ActivityID:     ids.NewV7(),
			DealID:         ids.NewV7(),
			InvoiceID:      ids.NewV7(),
			ContractID:     ids.NewV7(),
			ConsentEventID: ids.NewV7(),
			BasisID:        ids.NewV7(),
		},
	}

	out, err := freezePayload(in).thaw()
	if err != nil {
		t.Fatalf("thawing a frozen send: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("a scheduled send came back changed:\n frozen: %+v\n thawed: %+v", in, out)
	}
}

// Every field of the input is actually SET by the fixture above. Without this,
// a new field would be zero on both sides, DeepEqual would pass, and the test
// meant to catch a dropped field would be the thing hiding it.
func TestTheRoundTripFixtureLeavesNoFieldUnset(t *testing.T) {
	in := SendEmailInput{
		Recipients:       []string{"buyer@example.test"},
		Cc:               []string{"cc@example.test"},
		Bcc:              []string{"blind@example.test"},
		Subject:          "Your quote",
		Body:             "As discussed.",
		HTMLBody:         "<p>As discussed.</p>",
		AttachmentIDs:    []ids.UUID{ids.NewV7()},
		ConsentPurpose:   "marketing_email",
		DraftRef:         "draft-7",
		Context:          commsauthz.CategoryMarketing,
		MarketingPurpose: "newsletter",
		OperatorReason:   "they asked at the fair",
		Evidence:         commsauthz.Evidence{ActivityID: ids.NewV7()},
	}
	v := reflect.ValueOf(in)
	for i := range v.NumField() {
		if v.Field(i).IsZero() {
			t.Errorf("SendEmailInput.%s is zero in the round-trip fixture, so the round trip proves nothing about it",
				v.Type().Field(i).Name)
		}
	}
}

// An unnamed record stays unnamed. Rendering a zero id as text would put the
// all-zeroes UUID on the row, which reads as a record that exists.
func TestAnUnnamedRecordDoesNotBecomeAZeroID(t *testing.T) {
	frozen := freezePayload(SendEmailInput{Subject: "Hello"})
	if frozen.Evidence != (frozenEvidence{}) {
		t.Errorf("evidence = %+v, want every field empty when the caller named nothing", frozen.Evidence)
	}
	out, err := frozen.thaw()
	if err != nil {
		t.Fatalf("thawing: %v", err)
	}
	if out.Evidence != (commsauthz.Evidence{}) {
		t.Errorf("evidence = %+v, want zero after a round trip that named nothing", out.Evidence)
	}
}
