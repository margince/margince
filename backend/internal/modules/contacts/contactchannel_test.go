// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestContactCreationRequiresAChannelOnEveryWireCaller(t *testing.T) {
	for name, convert := range map[string]func(crmcontracts.CreateContactRequest) (CreateContactInput, error){
		"human or agent": ContactCreationInput, "declared importer": contactCreateInputFromImporter,
	} {
		t.Run(name, func(t *testing.T) {
			for _, test := range []struct {
				request  crmcontracts.CreateContactRequest
				accepted bool
			}{
				{request: crmcontracts.CreateContactRequest{FullName: "Buyer", Source: "manual"}},
				{request: crmcontracts.CreateContactRequest{FullName: "Buyer", Source: "manual", LinkedinUrl: new(" ")}},
				{request: crmcontracts.CreateContactRequest{FullName: "Buyer", Source: "manual", LinkedinUrl: new("https://www.linkedin.com/in/buyer")}, accepted: true},
				{request: crmcontracts.CreateContactRequest{FullName: "Buyer", Source: "manual", Emails: &[]crmcontracts.ContactEmailInput{{Email: "buyer@example.test"}}}, accepted: true},
				{request: crmcontracts.CreateContactRequest{FullName: "Buyer", Source: "manual", Phones: &[]crmcontracts.ContactPhoneInput{{Phone: "+491234567890"}}}, accepted: true},
			} {
				_, err := convert(test.request)
				if (err == nil) != test.accepted {
					t.Fatalf("%+v: %v, accepted=%v", test.request, err, test.accepted)
				}
			}
		})
	}
}

func TestAContactChannelPreservesItsStatedAcquisition(t *testing.T) {
	for _, convert := range []func(crmcontracts.CreateContactRequest) (CreateContactInput, error){ContactCreationInput, contactCreateInputFromImporter} {
		in, err := convert(crmcontracts.CreateContactRequest{
			FullName: "Buyer", Source: "manual", LinkedinUrl: new("https://www.linkedin.com/in/buyer"),
			AcquisitionSource: new(crmcontracts.ContactStatedSourceOther), AcquisitionNote: new(" A referral "),
		})
		if err != nil || in.Acquisition.StatedSource != "other" || in.Acquisition.Note != "A referral" {
			t.Fatalf("stated acquisition: %+v, %v", in.Acquisition, err)
		}
	}
}
