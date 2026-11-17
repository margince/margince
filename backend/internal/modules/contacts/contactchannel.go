// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// HasContactChannel also identifies imported contacts needing an identity warning.
func HasContactChannel(in CreateContactInput) bool {
	for _, email := range in.Emails {
		if strings.TrimSpace(email.Email) != "" {
			return true
		}
	}
	for _, phone := range in.Phones {
		if strings.TrimSpace(phone.Phone) != "" {
			return true
		}
	}
	return strings.TrimSpace(socialHandle(in.Social[SocialLinkedIn])) != ""
}

// RequireContactChannel is shared by the two manual creation forms; import writers retain incomplete identities.
func RequireContactChannel(in CreateContactInput) error {
	if HasContactChannel(in) {
		return nil
	}
	return httperr.Validation("emails", "contact_channel_required", "add at least one email, phone or LinkedIn URL")
}
