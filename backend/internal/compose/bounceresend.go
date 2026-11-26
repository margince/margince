// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// "Send again" on a bounced message: the same message goes to the address the
// rep corrected. It is the one send, not a second one. This file reads the
// bounced delivery and puts the corrected address on the contact. It then
// hands the author's own message to activities.Store.SendEmail. The
// authorization, the consent gate and the write shape are the ones every other
// send runs.

import (
	"context"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bounceResendService sends a bounced message again.
type bounceResendService struct {
	bounces  *comms.Store
	contacts *contacts.Store
	sends    *activities.Store
	gate     *consent.Gate
	delivery DeliveryMachinery
}

func newBounceResendService(pool *pgxpool.Pool, delivery DeliveryMachinery, send SendPath) bounceResendService {
	db := InstallationDB(pool)
	return bounceResendService{
		bounces:  comms.NewStore(db, time.Now, activities.NewStore(db)),
		contacts: newContactsStore(db),
		sends:    sendStore(pool, send),
		gate:     consentGateFor(pool),
		delivery: delivery,
	}
}

func newBounceResendHandlers(pool *pgxpool.Pool, send SendPath) bounceResendHandlers {
	resend := newBounceResendService(pool, send.Delivery, send)
	return bounceResendHandlers{resend: &resend}
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
func (s bounceResendService) SendAgain(ctx context.Context, bounceID ids.UUID, address string) (crmcontracts.Activity, error) {
	// A person's act: it changes a contact's address and sends a message a
	// person wrote. The contract refuses an agent first; this is the same rule
	// for a caller that did not come through the contract's transport.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalHuman {
		return crmcontracts.Activity{}, apperrors.ErrPermissionDenied
	}
	source, err := s.bounces.ResendSource(ctx, bounceID)
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
	if err := s.putAddressOnContact(ctx, source, corrected); err != nil {
		return crmcontracts.Activity{}, err
	}
	in := resendInput(source, corrected)
	return s.sends.SendEmail(ctx, activities.FromActivity(ids.From[ids.ActivityKind](source.ActivityID)), in, s.gate, s.delivery)
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

// putAddressOnContact replaces the bounced address on the contact with the
// corrected one, keeping its type, place and primary mark. A contact that
// already holds the corrected address is left as it is, so a second press
// after a refused send writes nothing twice. A contact that no longer holds
// the bounced address gains the corrected one.
func (s bounceResendService) putAddressOnContact(ctx context.Context, source comms.ResendSource, corrected string) error {
	contactID := ids.From[ids.ContactKind](source.ContactID)
	contact, err := s.contacts.GetContact(ctx, contactID, storekit.LiveOnly)
	if err != nil {
		return err
	}
	emails, changed := replaceAddress(contact, source.BounceRecipient, corrected)
	if !changed {
		return nil
	}
	_, err = s.contacts.UpdateContact(ctx, contactID, contacts.UpdateContactInput{
		Emails: emails, IfVersion: contact.Version, Source: "manual",
	})
	return err
}

func replaceAddress(contact crmcontracts.Contact, bounced, corrected string) ([]contacts.ContactEmailInput, bool) {
	var held []crmcontracts.ContactEmail
	if contact.Emails != nil {
		held = *contact.Emails
	}
	out := make([]contacts.ContactEmailInput, 0, len(held)+1)
	replaced := false
	for _, email := range held {
		address := strings.ToLower(string(email.Email))
		if address == corrected {
			return nil, false
		}
		input := contacts.ContactEmailInput{Email: address, EmailType: string(email.EmailType), IsPrimary: email.IsPrimary, Position: email.Position}
		if address == strings.ToLower(strings.TrimSpace(bounced)) {
			input.Email, replaced = corrected, true
		}
		out = append(out, input)
	}
	if !replaced {
		out = append(out, contacts.ContactEmailInput{Email: corrected, EmailType: "work", Position: len(out)})
	}
	return out, true
}

// resendInput is the author's message with the bounced address swapped for the
// corrected one wherever it stood: To, cc or bcc.
func resendInput(source comms.ResendSource, corrected string) activities.SendEmailInput {
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
		AttachmentIDs:  source.AttachmentIDs,
		ConsentPurpose: source.ConsentPurpose,
		ResentFrom:     source.ID,
	}
}

// bounceResendHandlers is the transport for the bounce row's "Send again".
type bounceResendHandlers struct {
	// resend is nil in a composition with no delivery machinery, and the route
	// then answers not-implemented like every other unwired send.
	resend *bounceResendService
}

func (h bounceResendHandlers) SendBouncedMessageAgain(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	if h.resend == nil {
		httperr.NotImplemented(w, r, "SendBouncedMessageAgain")
		return
	}
	var body crmcontracts.BounceSendAgainRequest
	if !httperr.Decode(w, r, &body) {
		return
	}
	sent, err := h.resend.SendAgain(r.Context(), ids.UUID(id), string(body.Address))
	if err != nil {
		httperr.Write(w, r, fmt.Errorf("sending a bounced message again: %w", err))
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, sent)
}
