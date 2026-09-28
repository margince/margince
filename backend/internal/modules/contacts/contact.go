// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// DuplicateEmailError carries the existing contact for the 409 dedupe
// contract (data-model §3.2: "create with an existing email returns 409 +
// existing id").
type DuplicateEmailError struct {
	Email      string
	ExistingID ids.ContactID
}

func (e *DuplicateEmailError) Error() string {
	return "contact with email " + e.Email + " already exists"
}
func (e *DuplicateEmailError) Is(target error) bool { return target == apperrors.ErrConflict }

// ContactEmailInput / ContactPhoneInput are the child rows a create carries.
type ContactEmailInput struct {
	Email     string
	EmailType string
	IsPrimary bool
	Position  int
	// VouchedNotCorresponded marks an address a provider's directory supplied
	// for a human reached on another medium, rather than one this workspace has
	// ever exchanged mail with.
	//
	// It identifies the contact, which is what it is stored for, and it proves
	// nothing about mail — so the mail ladder must not read it as a settled
	// verdict about the address. False for every writer that has correspondence
	// or a human's own assertion behind it, which is every writer but one.
	VouchedNotCorresponded bool
}

// ContactPhoneInput is one number on a contact, as a caller states it.
type ContactPhoneInput struct {
	Phone     string
	PhoneType string
	IsPrimary bool
	Position  int
}

// CreateContactInput is a new contact and everything that arrives with it.
type CreateContactInput struct {
	// Acquisition says why this contact exists — what the contact did, or what
	// was done to obtain them. A caller that does not say records
	// unknown_legacy, which is the honest answer and the one that makes the
	// gap visible instead of leaving the question unasked.
	Acquisition Acquisition
	FullName    string
	FirstName   *string
	LastName    *string
	Title       *string
	OwnerID     *ids.UserID
	Social      map[string]any
	Address     *crmcontracts.Address
	Emails      []ContactEmailInput
	Phones      []ContactPhoneInput
	Source      string
	// SourceSystem names the system an import took this contact from; nil
	// for one created here, which is what makes it unattributable.
	SourceSystem *string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

// CreateContact inserts the contact + child rows + audit + event atomically.
// The email dedupe unique index turns a duplicate into the 409 contract.
func (s *Store) CreateContact(ctx context.Context, in CreateContactInput) (crmcontracts.Contact, error) {
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return crmcontracts.Contact{}, err
	}
	by, err := s.readyContactCreate(ctx, in)
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	// The store-opened path reads the catalog through the unexported helper,
	// not ActiveContactColumns: that one takes contact:read on the caller's
	// behalf, and a seat may hold create without it.
	active, err := s.activeColumns(ctx, "contact")
	if err != nil {
		return crmcontracts.Contact{}, err
	}

	var out crmcontracts.Contact
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createContactInTx(ctx, tx, in, by, active)
		return err
	})
	return out, err
}

// CreateContactTx is CreateContact for a caller that already opened a
// transaction — one whose own write must land with this contact or not at all.
// Same gates in the same order; only the transaction is borrowed.
//
// Custom fields are refused rather than dropped: the catalog they are matched
// against is read in a transaction of its own, which is exactly the second
// connection this seam exists to avoid taking.
func (s *Store) CreateContactTx(ctx context.Context, tx pgx.Tx, in CreateContactInput) (crmcontracts.Contact, error) {
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return crmcontracts.Contact{}, err
	}
	if err := refuseCustomFields(in.CustomFields); err != nil {
		return crmcontracts.Contact{}, err
	}
	by, err := s.readyContactCreate(ctx, in)
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	return createContactInTx(ctx, tx, in, by, nil)
}

// readyContactCreate runs what a create settles BEFORE any transaction opens —
// the contact parse and the captured-by resolution — and answers the
// attribution the write shape stamps. Both entry points call it, so neither
// can drift from the other's validation.
func (s *Store) readyContactCreate(ctx context.Context, in CreateContactInput) (string, error) {
	if err := parseContactContacts(in.Emails, in.Phones); err != nil {
		return "", err
	}
	return storekit.CapturedBy(ctx)
}

// createContactInTx is CreateContact's transactional body, shared by the
// store-opened and caller-opened entry points.
func createContactInTx(ctx context.Context, tx pgx.Tx, in CreateContactInput, by string,
	active []fieldcatalog.Column,
) (crmcontracts.Contact, error) {
	if err := ensureContactEmailsUnclaimed(ctx, tx, in.Emails); err != nil {
		return crmcontracts.Contact{}, err
	}

	match, err := manualDedupeContact(ctx, tx, in)
	if err != nil {
		return crmcontracts.Contact{}, err
	}

	id, err := createContact(ctx, tx, match, ContactSpec{
		// Whatever the caller declared. An unset kind records unknown_legacy,
		// which is the honest answer for a contact somebody typed in without
		// saying why — and the answer that makes the gap visible rather than
		// leaving the question unasked.
		Acquisition: acquisitionForCreate(in),
		// A typed create publishes to the workspace, whoever typed it. An agent
		// creating a contact on a rep's behalf is doing the rep's filing, and a
		// contact only its creator can see is not in the CRM in any useful
		// sense: the colleague who goes looking finds nothing, and cannot tell an
		// invisible record from a missing one.
		//
		// The capture paths are unaffected. They carry OwnerScoped on the spec
		// and still mint an owner-scoped contact where privacy demands one — an
		// unjudged sender, and an advisor's record among them.
		Visibility:   visibilityWorkspace,
		FullName:     in.FullName,
		FirstName:    in.FirstName,
		LastName:     in.LastName,
		Title:        in.Title,
		OwnerID:      in.OwnerID,
		Address:      in.Address,
		Social:       in.Social,
		Emails:       in.Emails,
		Phones:       in.Phones,
		Source:       in.Source,
		SourceSystem: in.SourceSystem,
		CapturedBy:   by,
		CustomFields: in.CustomFields,
		Active:       active,
	})
	if err != nil {
		return crmcontracts.Contact{}, err
	}

	auditID, err := storekit.Audit(ctx, tx, "create", "contact", id.UUID, nil, map[string]any{"full_name": in.FullName})
	if err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("audit contact create: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactCreated{FullName: in.FullName}); err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("emit contact.created: %w", err)
	}
	if err := match.recordIfReview(ctx, tx, id, in.FullName, in.Source, by); err != nil {
		return crmcontracts.Contact{}, err
	}

	out, err := readContact(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Contact{}, fmt.Errorf("read created contact: %w", err)
	}
	return out, nil
}

// GetContact returns one contact with child rows; archived rows resolve
// only under IncludeArchived (they stay fetchable by id after merge). The
// object grant is asked before the transaction as well as inside
// EnsureReadable, so a caller holding none costs no connection.
func (s *Store) GetContact(ctx context.Context, id ids.ContactID, archived storekit.ArchivedFilter) (crmcontracts.Contact, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return crmcontracts.Contact{}, err
	}
	active, err := s.activeColumns(ctx, "contact")
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	var out crmcontracts.Contact
	err = s.tx(ctx, func(tx pgx.Tx) (err error) {
		if err := auth.EnsureReadable(ctx, tx, "contact", id.UUID); err != nil {
			return err
		}
		out, err = readContact(ctx, tx, id, archived, active)
		return err
	})
	return out, err
}

// GetContactTx is GetContact for a caller that already opened a transaction —
// the composite record read, which must see every one of its sections at the
// same instant and cannot afford a second connection per section. Same gates
// in the same order; only the transaction is borrowed.
//
// active is the caller's to fetch, with ActiveContactColumns, before it opens
// that transaction: the catalog read runs a transaction of its own, and a
// second connection taken from inside the caller's would commit separately and
// block undetectably against a lock the caller already holds.
func (s *Store) GetContactTx(ctx context.Context, tx pgx.Tx, id ids.ContactID,
	archived storekit.ArchivedFilter, active CustomColumns,
) (crmcontracts.Contact, error) {
	if err := auth.EnsureReadable(ctx, tx, "contact", id.UUID); err != nil {
		return crmcontracts.Contact{}, err
	}
	return readContact(ctx, tx, id, archived, active.cols)
}

// EnsureContactByEmail returns the contact holding this address, creating one
// when none does — where an address seen on a message becomes a record.
func (s *Store) EnsureContactByEmail(ctx context.Context, fullName, email, source string) (ids.UUID, error) {
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return ids.Nil, err
	}
	lookup := func() (ids.UUID, bool, error) {
		var id ids.UUID
		found := false
		err := s.tx(ctx, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `
				SELECT p.id FROM contact p
				JOIN contact_email e ON e.contact_id = p.id
				WHERE lower(e.email) = lower($1) AND p.archived_at IS NULL
				ORDER BY p.created_at LIMIT 1`, email).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			if err == nil {
				found = true
			}
			return err
		})
		return id, found, err
	}

	if id, found, err := lookup(); err != nil || found {
		return id, err
	}
	created, err := s.CreateContact(ctx, CreateContactInput{
		FullName: fullName,
		Emails:   []ContactEmailInput{{Email: email, EmailType: emailTypeWork, IsPrimary: true}},
		Source:   source,
	})
	if err == nil {
		return ids.UUID(created.Id), nil
	}
	// A concurrent capture of the same email won the race: its row IS
	// the idempotent answer.
	var dup *DuplicateEmailError
	if errors.As(err, &dup) {
		if id, found, lookupErr := lookup(); lookupErr == nil && found {
			return id, nil
		}
	}
	return ids.Nil, err
}
