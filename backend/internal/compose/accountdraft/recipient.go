// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func recipientOf(contact crmcontracts.Company360Contact) RecipientIn {
	out := RecipientIn{
		ID:        contact.ContactId.String(),
		Name:      contact.FullName,
		FirstName: firstName(contact.FullName),
		LastName:  lastName(contact.FullName),
	}
	if contact.Title != nil {
		out.Title = *contact.Title
	}
	if contact.PrimaryEmail != nil {
		out.Email = *contact.PrimaryEmail
	}
	out.Bucket = string(contact.Strength.Bucket)
	if contact.Strength.LastInteraction != nil {
		out.LastInteraction = contact.Strength.LastInteraction.UTC().Format(rfc3339)
	}
	return out
}

// firstName is what the greeting uses. Everything before the first space, or
// the whole name when it has none, since a one-word name is still a name.
func firstName(full string) string {
	if cut, _, found := strings.Cut(strings.TrimSpace(full), " "); found && cut != "" {
		return cut
	}
	return strings.TrimSpace(full)
}

// lastName is what follows the first space. A one-word name yields empty, so
// no formal greeting is addressed to a first name.
//
// Where the guess is wrong, the prompt protects the output: a formal greeting
// needs a given surname, and the model never invents one.
//
// contactdraft.surname answers this from a Contact's own last_name. The two
// stay apart because this fold has no Contact to pass.
func lastName(full string) string {
	if _, rest, found := strings.Cut(strings.TrimSpace(full), " "); found {
		return strings.TrimSpace(rest)
	}
	return ""
}

// findContact resolves the named recipient within the caller's own 360, so
// the lookup is also a permission check. A contact the caller cannot see is
// not in the view, and the refusal matches a contact id that names nobody.
// A separate contacts read would find contacts the 360 withheld.
func findContact(
	view crmcontracts.Company360, contactID string,
) (crmcontracts.Company360Contact, error) {
	if view.Contacts == nil {
		return crmcontracts.Company360Contact{}, fieldError("contact_id",
			"the account's contacts are not readable by you, so there is nobody here to write to")
	}
	for _, contact := range view.Contacts.Data {
		if contact.ContactId.String() == contactID {
			return contact, nil
		}
	}
	return crmcontracts.Company360Contact{}, fieldError("contact_id",
		"that contact is not a contact you can see on this account")
}
