// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webformlead

import (
	"github.com/margince/margince/backend/internal/modules/webforms"
	"testing"
)

func TestOnlyTheSubmittedSalutationSetsTheLeadChoice(t *testing.T) {
	for input, want := range map[string]string{"Herr": "mr", "Mr": "mr", "Mr.": "mr", "Frau": "ms", "Ms": "ms", "Mrs": "ms", "Dr.": "none", "none": "none"} {
		t.Run(input, func(t *testing.T) {
			in := LeadInput(webforms.Landing{Enquiry: webforms.Enquiry{Email: "buyer@example.test", FullName: "Dana Example", Salutation: input}})
			if in.Salutation == nil || string(*in.Salutation) != want {
				t.Fatalf("%q mapped to %v, want %s", input, in.Salutation, want)
			}
		})
	}
	in := LeadInput(webforms.Landing{Enquiry: webforms.Enquiry{Email: "buyer@example.test", FullName: "Mr Dana Example"}})
	if in.Salutation != nil {
		t.Fatal("inferred a salutation from the name")
	}
}
