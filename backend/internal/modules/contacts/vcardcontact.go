// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "strings"

// VCardContactName is the name a card's contact is created with. An approval
// that shows a card shows this, so the decider sees the name that is written.
func VCardContactName(entry VCardEntry) string {
	return strings.TrimSpace(entry.FullName)
}

// ValidateVCardContacts answers whether a reviewed card's addresses and
// numbers are ones the create will accept, without writing anything. It builds
// the create's own input and runs the create's own parser over it, so the
// answer matches what the writer will say. The input is built fresh and thrown
// away, so the proposal on the review row is left as the reviewer sees it.
func ValidateVCardContacts(entry VCardEntry) error {
	contact := contactFromVCard(entry)
	return parseContactContacts(contact.Emails, contact.Phones)
}

func contactFromVCard(entry VCardEntry) CreateContactInput {
	contact := CreateContactInput{
		FullName:      VCardContactName(entry),
		sourceNames:   true,
		Source:        vcardSource,
		auditEvidence: map[string]any{auditKeySource: vcardSource},
	}
	if title := strings.TrimSpace(entry.Title); title != "" {
		contact.Title = &title
	}
	// Only a LinkedIn URL goes in the social slot, whose key is "linkedin". A
	// card's URL is usually the company site, which would show as the contact's
	// LinkedIn profile. The website lands on the profile-field sidecar, as it
	// does on the update path.
	// The URL goes through the normalizer because the slot is compared
	// verbatim: it is the exact-match dedupe key and the resolver's lookup.
	// An unnormalized spelling would file the same contact under a second identity.
	if u := strings.TrimSpace(entry.URL); u != "" {
		if normalized, err := NormalizeContactLinkedInURL(u); err == nil {
			contact.Social = map[string]any{profileURLField: normalized}
		}
	}
	// A reader who sees an address on the card expects to find it on the record.
	contact.Employer = EmployerChoice{CompanyName: new(entry.Company), Role: contact.Title, Source: vcardSource}
	contact.Address = vcardRecordAddress(entry)
	for i, email := range entry.Emails {
		contact.Emails = append(contact.Emails, ContactEmailInput{
			Email:     strings.ToLower(strings.TrimSpace(email.Value)),
			EmailType: email.Kind,
			IsPrimary: i == 0,
			Position:  i,
			// A card states an address but is not correspondence. The mail ladder
			// must not read it as a verdict on whether this contact writes to us.
			VouchedNotCorresponded: true,
		})
	}
	for i, phone := range entry.Phones {
		contact.Phones = append(contact.Phones, ContactPhoneInput{
			Phone:     strings.TrimSpace(phone.Value),
			PhoneType: phone.Kind,
			IsPrimary: i == 0,
			Position:  i,
		})
	}
	return contact
}
