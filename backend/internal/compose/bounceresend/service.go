// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package bounceresend is "Send again" on a bounced message: the same message
// goes to the address the rep corrected. It is the one send, not a second one.
// It reads the bounced delivery and puts the corrected address on the contact.
// It then hands the author's own message to activities.Store.SendEmail. The
// authorization, the consent gate and the write shape are the ones every other
// send runs.
package bounceresend

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Service sends a bounced message again.
type Service struct {
	Bounces  *comms.Store
	Contacts *contacts.Store
	Sends    *activities.Store
	Gate     activities.ConsentGate
	Delivery activities.DeliveryStager
}

// bounceAddressFault refuses an address that cannot stand in for the one that
// refused, naming the request field the rep can change.
type bounceAddressFault struct{ code, message string }

func (f *bounceAddressFault) Error() string { return f.message }

func (f *bounceAddressFault) FieldFault() (field, code, message string) {
	return "address", f.code, f.message
}

// SendAgain replaces the bounced address on the contact and sends the same
// message to the new one.
func (s Service) SendAgain(ctx context.Context, bounceID ids.UUID, address string) (crmcontracts.Activity, error) {
	// A signed-in user's act: it changes a contact's address and sends a
	// message the user wrote. The contract refuses an agent first; this is the same rule
	// for a caller that did not come through the contract's transport.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalHuman {
		return crmcontracts.Activity{}, apperrors.ErrPermissionDenied
	}
	source, err := s.Bounces.ResendSource(ctx, bounceID)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	corrected, err := usableAddress(address, source.BounceRecipient)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	// Before the contact is touched. A caller who may change the contact and
	// not send would otherwise leave the address changed behind a refusal.
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.Activity{}, err
	}
	in, err := resendInput(source, corrected)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	restore, err := s.putAddressOnContact(ctx, source, corrected)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	sent, err := s.Sends.SendEmail(ctx, activities.FromActivity(ids.From[ids.ActivityKind](source.ActivityID)), in, s.Gate, s.Delivery)
	if err != nil {
		// The consent gate reads the address from the contact, so it is written
		// first. A refusal puts the contact back as it was.
		return crmcontracts.Activity{}, errors.Join(err, restore(ctx))
	}
	return sent, nil
}

// usableAddress is the address the rep typed, trimmed and lowercased as the
// contact stores it, or the refusal that says why it cannot be used.
func usableAddress(typed, bounced string) (string, error) {
	typed = strings.TrimSpace(typed)
	parsed, err := mail.ParseAddress(typed)
	if err != nil || parsed.Address != typed {
		return "", &bounceAddressFault{code: "invalid_email", message: "Enter one email address, such as name@example.com."}
	}
	corrected := strings.ToLower(parsed.Address)
	if corrected == strings.ToLower(strings.TrimSpace(bounced)) {
		return "", &bounceAddressFault{code: "same_address", message: "That is the address that refused the message. Enter a different one."}
	}
	return corrected, nil
}

// putAddressOnContact puts the corrected address where the bounced one stood,
// keeping its type, place and primary mark. A contact that already holds the
// corrected address is left alone. The write is pinned to the version read, so
// two presses with different addresses cannot both win. The returned function
// puts the addresses back for a send refused after the write, and fails when
// someone else changed the contact since.
func (s Service) putAddressOnContact(
	ctx context.Context, source comms.ResendSource, corrected string,
) (restore func(context.Context) error, err error) {
	nothing := func(context.Context) error { return nil }
	contactID := ids.From[ids.ContactKind](source.ContactID)
	contact, err := s.Contacts.GetContact(ctx, contactID, storekit.LiveOnly)
	if err != nil {
		return nothing, err
	}
	emails, changed := replaceAddress(contact, source.BounceRecipient, corrected)
	if !changed {
		return nothing, nil
	}
	updated, err := s.Contacts.UpdateContact(ctx, contactID, contacts.UpdateContactInput{
		Emails: emails, IfVersion: contact.Version, Source: "manual",
	})
	if err != nil {
		return nothing, err
	}
	return func(ctx context.Context) error {
		_, err := s.Contacts.UpdateContact(ctx, contactID, contacts.UpdateContactInput{
			Emails: heldAddresses(contact), IfVersion: updated.Version, Source: "manual",
		})
		if err != nil {
			return fmt.Errorf("the contact's address could not be put back: %w", err)
		}
		return nil
	}, nil
}

// heldAddresses is the contact's live addresses as a write carries them.
func heldAddresses(contact crmcontracts.Contact) []contacts.ContactEmailInput {
	var held []crmcontracts.ContactEmail
	if contact.Emails != nil {
		held = *contact.Emails
	}
	out := make([]contacts.ContactEmailInput, 0, len(held))
	for _, email := range held {
		out = append(out, contacts.ContactEmailInput{
			Email: strings.ToLower(string(email.Email)), EmailType: string(email.EmailType),
			IsPrimary: email.IsPrimary, Position: email.Position,
		})
	}
	return out
}

func replaceAddress(contact crmcontracts.Contact, bounced, corrected string) ([]contacts.ContactEmailInput, bool) {
	held := heldAddresses(contact)
	bounced = strings.ToLower(strings.TrimSpace(bounced))
	replaced := false
	for i, email := range held {
		if email.Email == corrected {
			return nil, false
		}
		if email.Email == bounced {
			held[i].Email, replaced = corrected, true
		}
	}
	if !replaced {
		held = append(held, contacts.ContactEmailInput{Email: corrected, EmailType: "work", Position: len(held)})
	}
	return held, true
}

// resendInput is the author's message with the bounced address swapped for the
// corrected one wherever it stood: To, cc or bcc.
func resendInput(source comms.ResendSource, corrected string) (activities.SendEmailInput, error) {
	claims, err := activities.DecodeSendClaims(source.Claims)
	if err != nil {
		return activities.SendEmailInput{}, err
	}
	swap := func(list []string) []string {
		out := make([]string, 0, len(list))
		for _, address := range list {
			if strings.EqualFold(strings.TrimSpace(address), strings.TrimSpace(source.BounceRecipient)) {
				address = corrected
			}
			out = append(out, address)
		}
		return out
	}
	to, cc, bcc := swap(source.To), swap(source.Cc), swap(source.Bcc)
	recipients := append(append(append([]string{}, to...), cc...), bcc...)
	return activities.SendEmailInput{
		Recipients: recipients, Cc: cc, Bcc: bcc,
		Subject: source.Subject, Body: source.Body, HTMLBody: source.HTMLBody,
		AttachmentIDs:    source.AttachmentIDs,
		ConsentPurpose:   source.ConsentPurpose,
		Context:          claims.Context,
		MarketingPurpose: claims.MarketingPurpose,
		OperatorReason:   claims.OperatorReason,
		Evidence:         claims.Evidence,
		SendingEntity:    claims.SendingEntity,
		FromConnection:   source.ConnectionID,
		ResentFrom:       source.ID,
	}, nil
}

// Handlers is the transport for the bounce row's "Send again".
type Handlers struct {
	// resend is nil in a composition with no delivery machinery, and the route
	// then answers not-implemented like every other unwired send.
	Resend *Service
}

func (h Handlers) SendBouncedMessageAgain(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.Resend == nil {
		httperr.NotImplemented(w, r, "SendBouncedMessageAgain")
		return
	}
	var body crmcontracts.BounceSendAgainRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	sent, err := h.Resend.SendAgain(r.Context(), ids.UUID(id), string(body.Address))
	if err != nil {
		httperr.Write(w, r, fmt.Errorf("sending a bounced message again: %w", err))
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, sent)
}
