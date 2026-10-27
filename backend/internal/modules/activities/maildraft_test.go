// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADraftIsHeldToTheContractsSubjectAndAddressCaps(t *testing.T) {
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeActivity, ID: ids.NewV7()}
	for name, tc := range map[string]struct {
		content MailDraftContent
		field   string
	}{
		"a 998-character subject":        {MailDraftContent{Subject: strings.Repeat("é", 998)}, ""},
		"a 999-character subject":        {MailDraftContent{Subject: strings.Repeat("a", 999)}, "subject"},
		"a 320-character address":        {MailDraftContent{To: []string{strings.Repeat("é", 320)}}, ""},
		"a 321-character address in to":  {MailDraftContent{To: []string{strings.Repeat("a", 321)}}, "to"},
		"a 321-character address in cc":  {MailDraftContent{Cc: []string{"ok@example.com", strings.Repeat("a", 321)}}, "cc"},
		"a 321-character address in bcc": {MailDraftContent{Bcc: []string{strings.Repeat("a", 321)}}, "bcc"},
		"more than 100 addresses":        {MailDraftContent{To: make([]string, 101)}, "to"},
		"an empty composer":              {MailDraftContent{}, ""},
	} {
		err := validateDraft(anchor, tc.content)
		var refused *InvalidMailDraftError
		switch {
		case tc.field == "" && err != nil:
			t.Errorf("%s: refused with %v", name, err)
		case tc.field != "" && (!errors.As(err, &refused) || refused.Field != tc.field):
			t.Errorf("%s: %v, want a refusal naming %s", name, err, tc.field)
		}
	}
}

// A draft serves the records it was written from.
//
// Without them on the wire the composer rebuilds links from whichever page
// opened it. A draft grounded in a deal then sends under the contact alone.
func TestADraftServesTheRecordsItWasWrittenFrom(t *testing.T) {
	t.Parallel()
	deal, project := ids.NewV7(), ids.NewV7()
	out := mailDraftResponse(MailDraft{
		Grounding: []MailDraftAnchor{
			{Type: crmcontracts.MailDraftAnchorTypeDeal, ID: deal},
			{Type: crmcontracts.MailDraftAnchorTypeProject, ID: project},
		},
	})
	if out.Grounding == nil {
		t.Fatal("a grounded draft served nil, which serializes as null")
	}
	served := *out.Grounding
	if len(served) != 2 {
		t.Fatalf("served %d grounds, want the two the draft carried", len(served))
	}
	if served[0].AnchorType != crmcontracts.MailDraftAnchorTypeDeal ||
		ids.UUID(served[0].AnchorId) != deal {
		t.Errorf("the first ground is %v %v, want the deal", served[0].AnchorType, served[0].AnchorId)
	}
	if served[1].AnchorType != crmcontracts.MailDraftAnchorTypeProject {
		t.Errorf("the second ground is %v, want the project", served[1].AnchorType)
	}
}

// A human draft serves an empty list, never a null.
//
// The contract declares an array, so a nil would reach a strict client as
// `null` and a reader that cannot iterate it.
func TestAnUngroundedDraftServesAnEmptyList(t *testing.T) {
	t.Parallel()
	out := mailDraftResponse(MailDraft{})
	if out.Grounding == nil {
		t.Fatal("an ungrounded draft served nil, which serializes as null")
	}
	if len(*out.Grounding) != 0 {
		t.Errorf("an ungrounded draft served %d grounds", len(*out.Grounding))
	}
}
