// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"

	"github.com/margince/margince/backend/internal/compose/contactbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// briefFixtureAuthor is who wrote a fixture message, in the terms of the
// server's author projection (Activity.voice). A fixture names its author
// rather than a direction, because a direction does not say who wrote it.
type briefFixtureAuthor struct {
	// Kind is an ActivityAuthorKind: contact, colleague, machine, unresolved.
	Kind string `json:"kind"`
	Name string `json:"name"`
	// Role is the page contact's part, an ActivityVoiceContactRole. A contact
	// brief only; a company brief has no page contact.
	Role string `json:"role"`
	// Reader marks the author as the reader the brief is written for.
	Reader bool `json:"reader"`
}

// voice is the projection a 360 row would carry for this author.
func (a briefFixtureAuthor) voice() (*crmcontracts.ActivityVoice, error) {
	kind := crmcontracts.ActivityAuthorKind(a.Kind)
	if !kind.Valid() {
		return nil, fmt.Errorf("author kind %q is not one the contract carries", a.Kind)
	}
	author := &crmcontracts.ActivityAuthor{Kind: kind, IsReader: a.Reader}
	if a.Name != "" {
		name := a.Name
		author.DisplayName = &name
	}
	out := &crmcontracts.ActivityVoice{Author: author}
	if a.Role != "" {
		role := crmcontracts.ActivityVoiceContactRole(a.Role)
		if !role.Valid() {
			return nil, fmt.Errorf("contact role %q is not one the contract carries", a.Role)
		}
		out.ContactRole = &role
	}
	return out, nil
}

// fixtureSpeaker folds an optional fixture author through the site's own
// SpeakerFor. No author is no speaker, as on a withheld row.
func fixtureSpeaker(author *briefFixtureAuthor) (*contactbrief.Speaker, error) {
	if author == nil {
		return nil, nil //nolint:nilnil // no author is no speaker, and not an error
	}
	voice, err := author.voice()
	if err != nil {
		return nil, err
	}
	return contactbrief.SpeakerFor(voice), nil
}
