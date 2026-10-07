// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// contactIDField is the create body's field naming the contact a lead is
// worked from, quoted back in a refusal.
const contactIDField = "contact_id"

// fillLeadFromContact completes a lead from the contact it is worked from: the
// name, address, title, profile and employer the contact already holds fill
// whichever of those the caller left out, so a seller never retypes a contact
// the CRM knows and a lead created against a company is not left unnamed.
//
// The contact is read through GetContact, so it carries that read's gates: a
// contact the caller may not see is refused as not found, and an employer they
// may not see is simply absent. The contact itself is not linked or changed —
// a lead stays a separate record until it is qualified.
func (s *Store) fillLeadFromContact(ctx context.Context, in CreateLeadInput) (CreateLeadInput, error) {
	if in.FromContactID == nil {
		return in, nil
	}
	contact, err := s.GetContact(ctx, *in.FromContactID, storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) {
		return in, httperr.Validation(contactIDField, "not_found",
			"no contact you can read has this id; pick one from the contacts you can see")
	}
	if err != nil {
		return in, err
	}
	in.FullName = keptOrContact(in.FullName, contact.FullName)
	in.Title = keptOrContactPtr(in.Title, contact.Title)
	if contact.PrimaryEmail != nil {
		in.Email = keptOrContact(in.Email, string(*contact.PrimaryEmail))
	}
	if contact.Employer != nil {
		in.CompanyName = keptOrContact(in.CompanyName, contact.Employer.CompanyName)
	}
	if contact.Social != nil {
		if raw, ok := (*contact.Social)["linkedin"].(string); ok {
			if profile, isProfile := linkedInProfileOf(raw); isProfile {
				in.LinkedInURL = keptOrContact(in.LinkedInURL, profile)
			}
		}
	}
	return in, nil
}

// linkedInProfileOf answers the contact's LinkedIn value as a profile URL, or
// false when it is not one. A contact's slot may hold a bare handle, and on a
// lead the profile URL is an exact dedupe key: a handle read as "https://jane"
// would claim an identity nobody has.
func linkedInProfileOf(raw string) (string, bool) {
	normalized, err := NormalizeLinkedInURL(raw)
	if err != nil {
		return "", false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return "", false
	}
	host := strings.TrimPrefix(parsed.Hostname(), "www.")
	for _, slot := range LinkedInSlotHosts() {
		if host == slot || strings.HasSuffix(host, "."+slot) {
			return normalized, true
		}
	}
	return "", false
}

// keptOrContact keeps what the caller sent and falls back to the contact's value.
func keptOrContact(sent *string, fromContact string) *string {
	if sent != nil && strings.TrimSpace(*sent) != "" {
		return sent
	}
	if strings.TrimSpace(fromContact) == "" {
		return sent
	}
	return &fromContact
}

func keptOrContactPtr(sent, fromContact *string) *string {
	if fromContact == nil {
		return sent
	}
	return keptOrContact(sent, *fromContact)
}
