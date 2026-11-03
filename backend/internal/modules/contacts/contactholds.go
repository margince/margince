// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Somebody is a lead or a contact, never both. A lead cannot take an identity
// key held by a live contact. LockIdentityHolders asks the shared question;
// this file answers it for leads.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CodeContactHoldsEmail is the wire code of every contact-held refusal.
const CodeContactHoldsEmail = "contact_holds_email"

const contactIDField = "contact_id"

// ContactHoldsIdentityError refuses a lead over a key a live contact holds.
// ContactID is set only when the caller may read that contact. OwnerNotified
// says the contact's owner was given a Worklist task about the arrival.
type ContactHoldsIdentityError struct {
	Field         string
	ContactID     ids.ContactID
	OwnerNotified bool
	holder        ids.ContactID
}

func (e *ContactHoldsIdentityError) Error() string {
	return "they are already a contact; open a deal with the contact instead of a lead"
}

// Is maps the refusal onto the shared conflict sentinel, as the other
// duplicate refusals do.
func (e *ContactHoldsIdentityError) Is(target error) bool { return target == apperrors.ErrConflict }

// MessageFault names the code for a door that maps no contacts error itself.
func (e *ContactHoldsIdentityError) MessageFault() (code, message string) {
	return CodeContactHoldsEmail, e.Error()
}

// leadKeys are the identity keys a lead write gives a lead.
func leadKeys(email, linkedInURL *string) storekit.IdentityKeys {
	var keys storekit.IdentityKeys
	if email != nil && strings.TrimSpace(*email) != "" {
		keys.Emails = []string{*email}
	}
	if linkedInURL != nil && *linkedInURL != "" {
		keys.LinkedInURLs = []string{*linkedInURL}
	}
	return keys
}

func refuseRequestedContact(ctx context.Context, tx pgx.Tx, contactID *ids.ContactID, keys storekit.IdentityKeys) error {
	if contactID == nil {
		return nil
	}
	holders, err := storekit.LockIdentityHolders(ctx, tx, keys)
	if err != nil {
		return err
	}
	if len(holders.Contacts) == 0 {
		return httperr.Validation(contactIDField, "not_found", "contact_id does not match the lead identity")
	}
	return contactHoldsRefusal(ctx, tx, holders, keys, holders.Contacts[0])
}

// refuseContactHolders asks, under the identity locks, whether a live contact
// holds a key this lead write gives. workedFrom is the contact a lead worked
// from an existing contact names: its own keys are that lead's by design.
func refuseContactHolders(ctx context.Context, tx pgx.Tx, keys storekit.IdentityKeys, workedFrom *ids.ContactID) error {
	if len(keys.Emails) == 0 && len(keys.LinkedInURLs) == 0 {
		return nil
	}
	holders, err := storekit.LockIdentityHolders(ctx, tx, keys)
	if err != nil {
		return err
	}
	for _, contact := range holders.Contacts {
		if workedFrom != nil && contact == *workedFrom {
			continue
		}
		return contactHoldsRefusal(ctx, tx, holders, keys, contact)
	}
	return nil
}

// contactHoldsRefusal names the contact only to a caller who may read it, as
// a held address does (claimedEmailRefusal). A hidden holder is told no field.
func contactHoldsRefusal(ctx context.Context, tx pgx.Tx, holders storekit.IdentityHolders, keys storekit.IdentityKeys, contact ids.ContactID) error {
	refusal := &ContactHoldsIdentityError{holder: contact}
	visible, err := auth.VisibleTo(ctx, tx, entityContact, contact.UUID)
	if err != nil || !visible {
		return errors.Join(err, refusal)
	}
	refusal.ContactID = contact
	refusal.Field = linkedinURLField
	for _, email := range keys.Emails {
		if holders.ContactHoldsEmail(email) {
			refusal.Field = leadEmailColumn
		}
	}
	return refusal
}

// LeadArrival is a lead that was not created because its subject is already a
// contact. It carries what arrived and where it came from, for the contact's
// owner to read.
type LeadArrival struct {
	ContactID ids.ContactID
	OwnerID   ids.UUID
	// Key is the arrival's own key. The same arrival twice files one task.
	Key    string
	Origin string
	Fields LeadArrivalFields
}

// LeadArrivalFields are the submission's identity fields, as they arrived.
type LeadArrivalFields struct {
	FullName, Email, Phone, Title, CompanyName, LinkedInURL string
}

// LeadArrivals is the Worklist's side of a lead for an existing contact.
// Compose binds it.
type LeadArrivals interface {
	FileTx(ctx context.Context, tx pgx.Tx, arrival LeadArrival) error
}

// WithLeadArrivals wires the Worklist. Unwired, the lead is still refused and
// nobody is told.
func (s *Store) WithLeadArrivals(arrivals LeadArrivals) *Store {
	s.leadArrivals = arrivals
	return s
}

// TellContactOwnerOfLead is the one follow-up to a contact-held refusal. It
// files a task for the contact's owner in a transaction of its own and returns
// err with the outcome on it. Any other error passes through.
//
// It runs after the refused transaction rolled back, as AskHolderToShare does,
// so no contact row is locked after the subject locks the refusal took.
// origin names the door in words the owner reads.
func (s *Store) TellContactOwnerOfLead(ctx context.Context, err error, in CreateLeadInput, origin string) error {
	held, ok := errors.AsType[*ContactHoldsIdentityError](err)
	if !ok {
		return err
	}
	notified, fileErr := s.TellContactOwnerOfArrival(ctx, held.holder, in, origin)
	if fileErr != nil {
		return fileErr
	}
	held.OwnerNotified = notified
	return err
}

// TellContactOwnerOfArrival files the task for a door that found the contact
// itself and wrote no lead, after its own transaction. It answers whether the
// owner was told: an ownerless contact, or one the caller owns, tells nobody.
func (s *Store) TellContactOwnerOfArrival(ctx context.Context, contact ids.ContactID, in CreateLeadInput, origin string) (bool, error) {
	if s.leadArrivals == nil {
		return false, nil
	}
	arrival := LeadArrival{ContactID: contact, Origin: origin, Fields: arrivalFields(in), Key: arrivalKey(in, contact)}
	notified := false
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		owner, owned, err := lockHolder(ctx, tx, contact)
		if err != nil || !owned || callerIs(ctx, owner) {
			return err
		}
		arrival.OwnerID = owner
		if err := s.leadArrivals.FileTx(principal.SystemActing(ctx, leadArrivalActor), tx, arrival); err != nil {
			return err
		}
		notified = true
		return nil
	}); err != nil {
		return false, fmt.Errorf("contacts: telling a contact's owner about a lead: %w", err)
	}
	return notified, nil
}

// leadArrivalActor is the system principal the task is written as.
const leadArrivalActor = "system:lead-for-contact"

// callerIs says the human behind this write, or behind the agent making it, is user.
func callerIs(ctx context.Context, user ids.UUID) bool {
	caller, ok := shareRequesterOf(ctx)
	return ok && caller == user
}

func arrivalFields(in CreateLeadInput) LeadArrivalFields {
	return LeadArrivalFields{
		FullName: deref(in.FullName), Email: deref(in.Email), Phone: deref(in.Phone),
		Title: deref(in.Title), CompanyName: deref(in.CompanyName), LinkedInURL: deref(in.LinkedInURL),
	}
}

// arrivalKey is the lead's own source key when it has one. Without one, it is
// a digest of what arrived for which contact, so a resent form files nothing new.
func arrivalKey(in CreateLeadInput, contact ids.ContactID) string {
	if in.SourceSystem != nil && in.SourceID != nil {
		return *in.SourceSystem + ":" + *in.SourceID
	}
	f := arrivalFields(in)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		contact.String(), in.Source, f.FullName, f.Email, f.Phone, f.Title, f.CompanyName, f.LinkedInURL,
	}, "\x00")))
	return "digest:" + hex.EncodeToString(sum[:])
}

// refuseContactHeldLeadEdit asks about the address and the profile an edit
// gives the lead anew. The keys it already holds are not asked about again, so
// a save of an untouched form still lands. It runs before the erasure check, so
// the LinkedIn lock comes before the subject lock, as on a create.
func refuseContactHeldLeadEdit(ctx context.Context, tx pgx.Tx, current crmcontracts.Lead, in UpdateLeadInput) error {
	var keys storekit.IdentityKeys
	if in.Email != nil && (current.Email == nil || !strings.EqualFold(string(*current.Email), strings.TrimSpace(*in.Email))) {
		keys.Emails = []string{*in.Email}
	}
	if in.LinkedInURL != nil && *in.LinkedInURL != "" {
		if url, err := leadLinkedInForUpdate(*in.LinkedInURL, current.LinkedinUrl); err == nil &&
			(current.LinkedinUrl == nil || url != *current.LinkedinUrl) {
			keys.LinkedInURLs = []string{url}
		}
	}
	return refuseContactHolders(ctx, tx, keys, contactIDFrom(current.FromContactId))
}

// contactIDFrom reads the contact a lead was worked from off its contract shape.
func contactIDFrom(id *openapi_types.UUID) *ids.ContactID {
	if id == nil {
		return nil
	}
	contact := ids.From[ids.ContactKind](ids.UUID(*id))
	return &contact
}

// WithLeadArrivals mirrors the store option for the HTTP surface.
func (h Handlers) WithLeadArrivals(arrivals LeadArrivals) Handlers {
	h.store = h.store.WithLeadArrivals(arrivals)
	return h
}

// WithLeadArrivals mirrors the store option for the agent surface.
func (p *Provider) WithLeadArrivals(arrivals LeadArrivals) *Provider {
	p.store = p.store.WithLeadArrivals(arrivals)
	return p
}
