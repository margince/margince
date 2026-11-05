// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A note or a call is what its subject or body says, so the create wire
// refuses one that says nothing. Every other kind carries its content elsewhere.
func TestANoteOrCallWithNeitherSubjectNorBodyIsRefused(t *testing.T) {
	blank, text := "  \n", "Left a voicemail"
	for _, kind := range []crmcontracts.CreateActivityRequestKind{
		crmcontracts.CreateActivityRequestKindCreateActivityRequestKindNote,
		crmcontracts.CreateActivityRequestKindCreateActivityRequestKindCall,
	} {
		for name, req := range map[string]crmcontracts.CreateActivityRequest{
			"nothing sent":  {Kind: kind, Source: "manual"},
			"blank both":    {Kind: kind, Subject: &blank, Body: &blank, Source: "manual"},
			"blank subject": {Kind: kind, Subject: &blank, Source: "manual"},
			"blank body":    {Kind: kind, Body: &blank, Source: "manual"},
		} {
			_, err := LogActivityInputFrom(req)
			if _, ok := errors.AsType[*EmptyEntryError](err); !ok {
				t.Errorf("%s %s: err = %v, want an empty-entry refusal", kind, name, err)
			}
		}
		for name, req := range map[string]crmcontracts.CreateActivityRequest{
			"subject only": {Kind: kind, Subject: &text, Source: "manual"},
			"body only":    {Kind: kind, Body: &text, Source: "manual"},
		} {
			if _, err := LogActivityInputFrom(req); err != nil {
				t.Errorf("%s %s was refused: %v", kind, name, err)
			}
		}
	}
	for _, kind := range []crmcontracts.CreateActivityRequestKind{
		crmcontracts.CreateActivityRequestKindCreateActivityRequestKindEmail,
		crmcontracts.CreateActivityRequestKindCreateActivityRequestKindMeeting,
	} {
		if _, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: kind, Source: "manual"}); err != nil {
			t.Errorf("a %s may carry no subject or body: %v", kind, err)
		}
	}
}

func TestAnEmptyEntryNamesTheSubjectField(t *testing.T) {
	field, code, message := (&EmptyEntryError{Kind: "note"}).FieldFault()
	if field != "subject" || code != "subject_or_body_required" || message == "" {
		t.Fatalf("fault = %q %q %q, want the subject field with its own code and a sentence", field, code, message)
	}
}
