// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestALeadWithNoIdentifyingFieldIsRefused(t *testing.T) {
	blank := "  "
	for name, in := range map[string]CreateLeadInput{
		"empty body":         {},
		"only a title":       {Title: new("VP Sales")},
		"blank name":         {FullName: &blank},
		"blank company":      {CompanyName: &blank},
		"only a source":      {Source: "manual"},
		"only an industry":   {Industry: new("software")},
		"blank name + title": {FullName: new(""), Title: new("CTO")},
	} {
		_, err := normalizedCreateLeadInput(in)
		detailed, ok := errors.AsType[*httperr.DetailedError](err)
		if !ok || len(detailed.Fields) != 1 || detailed.Fields[0].Code != "lead_identity_required" {
			t.Errorf("%s: err = %v, want one lead_identity_required field refusal", name, err)
		}
	}
}

func TestALeadWithAnyOneIdentifyingFieldIsAccepted(t *testing.T) {
	for name, in := range map[string]CreateLeadInput{
		"name":     {FullName: new("Dana Buyer")},
		"email":    {Email: new("dana@example.test")},
		"phone":    {Phone: new("+49 30 1234567")},
		"linkedin": {LinkedInURL: new("https://www.linkedin.com/in/dana-buyer")},
		"company":  {CompanyName: new("Example GmbH")},
	} {
		if _, err := normalizedCreateLeadInput(in); err != nil {
			t.Errorf("%s: refused: %v", name, err)
		}
	}
}
