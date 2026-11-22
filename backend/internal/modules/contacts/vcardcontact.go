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
// numbers are ones the create will accept, without writing anything.
//
// It asks by BUILDING the create's own input and running the create's own
// parser over it, rather than restating the rules: a second copy of "what is
// a valid number here" is the thing that drifts, and the whole point of
// asking early is that the answer matches what the writer will say.
//
// Nothing is normalised for the caller. The parse mutates the input it is
// given, and the input here is built fresh and thrown away — the proposal on
// the row is left exactly as the reviewer sees it.
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
	// Only a LinkedIn URL goes in the social slot, whose key IS "linkedin". A
	// card's URL is usually the company site, and filing that here showed a
	// company's home page as the contact's LinkedIn profile. The website lands
	// on the profile-field sidecar instead, by the same split the update path
	// makes — one rule, so which outcome a card gets cannot change what it
	// means.
	// Through the normalizer, like every other writer of this slot. The slot is
	// compared verbatim — the exact-match dedupe key, the provider resolver's
	// lookup — so a card that spelt the profile with http, a trailing slash or
	// a tracking query would file the same contact under a second identity.
	if u := strings.TrimSpace(entry.URL); u != "" {
		if normalized, err := NormalizeContactLinkedInURL(u); err == nil {
			contact.Social = map[string]any{profileURLField: normalized}
		}
	}
	// Parsing a field and then dropping it is the quieter defect. A reader who
	// sees an address on the card expects to find it on the record.
	contact.Employer = EmployerChoice{CompanyName: new(entry.Company), Role: contact.Title, Source: vcardSource}
	contact.Address = vcardRecordAddress(entry)
	for i, email := range entry.Emails {
		contact.Emails = append(contact.Emails, ContactEmailInput{
			Email:     strings.ToLower(strings.TrimSpace(email.Value)),
			EmailType: email.Kind,
			IsPrimary: i == 0,
			Position:  i,
			// A card STATES an address; it is not correspondence, and the mail
			// ladder must not read it as a settled verdict about whether this
			// contact writes to us.
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
