// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Exactly one subject (data-model §7): the DB CHECK only rules out
// both-null, so both-set and neither-set must die at the seam — before
// admission, before any row.
func TestConsentSubjectIsExactlyOne(t *testing.T) {
	contact := ids.New[ids.ContactKind]()
	lead := ids.New[ids.LeadKind]()

	sub, err := consentSubject(RecordInput{ContactID: contact})
	if err != nil || sub.entityType != "contact" || sub.column != "contact_id" || sub.id != contact.UUID {
		t.Fatalf("contact subject resolved wrong: %+v, %v", sub, err)
	}
	sub, err = consentSubject(RecordInput{LeadID: lead})
	if err != nil || sub.entityType != "lead" || sub.column != "lead_id" || sub.id != lead.UUID {
		t.Fatalf("lead subject resolved wrong: %+v, %v", sub, err)
	}

	for name, in := range map[string]RecordInput{
		"both subjects": {ContactID: contact, LeadID: lead},
		"no subject":    {},
	} {
		if _, err := consentSubject(in); err == nil {
			t.Errorf("%s: accepted; want a ValidationError", name)
		} else {
			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Errorf("%s: got %v, want a ValidationError", name, err)
			}
		}
	}
}

// Record refuses an invalid subject before touching admission or the
// database — a nil pool proves no connection is ever consulted.
func TestRecordRefusesAnAmbiguousSubjectBeforeAnyWrite(t *testing.T) {
	store := NewStore(nil)
	_, err := store.Record(context.Background(), RecordInput{
		ContactID:  ids.New[ids.ContactKind](),
		LeadID:     ids.New[ids.LeadKind](),
		NewState:   "granted",
		PolicyText: &grantWording,
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("got %v, want a ValidationError on the subject", err)
	}
	if invalid.Field != "subject" {
		t.Fatalf("error names field %q, want subject", invalid.Field)
	}
}

// grantWording is what a fixture shows the subject.
//
// Not decoration: a grant that cannot say what was shown is refused
// (requireWordingForGrant), because Art. 7(1) asks the controller to
// demonstrate what the subject agreed TO. A fixture omitting it would model a
// request the product does not accept, and every test written against it would
// describe a product we do not ship.
var grantWording = "Yes, send me the newsletter. I can unsubscribe at any time."
