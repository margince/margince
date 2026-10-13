// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

// The further contacts one draft goes to, such as everyone at the meeting it
// follows up. The contact in the path stays the one the draft is grounded in.

import (
	"context"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MaxAlsoTo is how many further contacts one draft may name.
const MaxAlsoTo = 24

// CoRecipients reads a further contact under the caller's own row scope. One
// the caller may not read answers apperrors.ErrNotFound.
type CoRecipients interface {
	GetContact(ctx context.Context, id ids.ContactID, archived storekit.ArchivedFilter) (crmcontracts.Contact, error)
}

// CoRecipientIn is one further reader of the message. The model gets the name
// only; the address goes to the draft's To line.
type CoRecipientIn struct {
	Name  string `json:"name"`
	Email string `json:"-"`
}

// errNoCoRecipients is a composition defect, never a client error: a surface
// that takes also_to must be wired with the reader that resolves it.
var errNoCoRecipients = errors.New("contactdraft: also_to named with no contact reader wired")

// WithCoRecipients binds the read that resolves also_to.
func (s *Service) WithCoRecipients(reader CoRecipients) *Service {
	s.others = reader
	return s
}

// coRecipients resolves also_to in the order given, without the path contact
// and without repeats. One contact the caller may not read refuses the whole
// draft, so nobody gets a To line the caller could not have typed.
func (s *Service) coRecipients(ctx context.Context, to ids.ContactID, also []ids.ContactID) ([]CoRecipientIn, error) {
	if len(also) == 0 {
		return nil, nil
	}
	if s.others == nil {
		return nil, errNoCoRecipients
	}
	seen := map[ids.ContactID]bool{to: true}
	out := make([]CoRecipientIn, 0, len(also))
	for _, id := range also {
		if seen[id] {
			continue
		}
		seen[id] = true
		contact, err := s.others.GetContact(ctx, id, storekit.LiveOnly)
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, httperr.Validation("also_to", "recipient_not_on_file",
				"one of the further recipients is not a contact you can see — remove them and draft again")
		}
		if err != nil {
			return nil, err
		}
		co := CoRecipientIn{Name: contact.FullName}
		if contact.PrimaryEmail != nil {
			co.Email = string(*contact.PrimaryEmail)
		}
		out = append(out, co)
	}
	return out, nil
}
