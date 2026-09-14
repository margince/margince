// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

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
		Acquisition: in.Acquisition,
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
// only under IncludeArchived (they stay fetchable by id after merge).
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
		if err := auth.EnsureVisible(ctx, tx, "contact", id.UUID); err != nil {
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
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return crmcontracts.Contact{}, err
	}
	if err := auth.EnsureVisible(ctx, tx, "contact", id.UUID); err != nil {
		return crmcontracts.Contact{}, err
	}
	return readContact(ctx, tx, id, archived, active.cols)
}

// UpdateContactInput is a partial write; Clear below says "set this to NULL".
type UpdateContactInput struct {
	// Clear names the wire fields to set to NULL. A JSON null cannot say so —
	// it decodes to a nil pointer and reads as "not supplied" — so the
	// reversal path names them here instead.
	Clear []string
	// Trail names what the audit trail calls this write; zero is an update.
	Trail     storekit.AuditTrail
	FullName  *string
	FirstName *string
	LastName  *string
	Title     *string
	OwnerID   *ids.UserID
	// Visibility moves a contact between 'workspace' and 'owner', in either
	// direction, for anybody the write gate admits.
	//
	// It was one-way until now — POST /contacts/{id}/publish only widened — on
	// the reasoning that a colleague may already have acted on seeing the
	// contact. That reasoning assumed a human made the disclosure, and the
	// common case is not a human: the sender classifier publishes a contact it
	// judges a real counterparty with nobody approving it, so a machine made a
	// decision no human could undo, the row's own owner included.
	Visibility *string
	Social     map[string]any
	Address    *crmcontracts.Address
	// Emails replaces the contact's live addresses when non-nil. nil is "not
	// supplied" and leaves the stored rows standing, exactly as Social is —
	// the distinction matters for an import whose file carried no email
	// column at all, which must not read as "this contact now has none".
	Emails []ContactEmailInput
	// Phones replaces the contact's live numbers when non-nil, with the same
	// nil-vs-empty distinction Emails carries.
	Phones    []ContactPhoneInput
	IfVersion *int64
	Source    string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

// UpdateContact applies a partial write under the caller's If-Match version,
// replacing the child rows the input names and leaving the ones it does not.
//
//nolint:gocognit,cyclop // the rename added no branch: this body is what it was under the old noun.
func (s *Store) UpdateContact(ctx context.Context, id ids.ContactID, in UpdateContactInput) (crmcontracts.Contact, error) {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return crmcontracts.Contact{}, err
	}
	active, err := s.activeColumns(ctx, "contact")
	if err != nil {
		return crmcontracts.Contact{}, err
	}
	var out crmcontracts.Contact
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, "contact", id.UUID); err != nil {
			return err
		}
		current, err := readContact(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read contact before update: %w", err)
		}

		if err := refuseUnreadableResult(current, in); err != nil {
			return err
		}
		p, err := buildContactPatch(current, in)
		if err != nil {
			return err
		}
		storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)
		if in.Social != nil || in.Emails != nil || in.Phones != nil {
			// The relation replacement rides the contact row's version
			// bump (updated_at below), so If-Match still guards it and
			// the audit row still records the transition.
			//
			// Emails and Phones are in this condition for a second reason:
			// without it a row whose ONLY change is an address or a number
			// hits p.Empty() below and returns having written nothing, so a
			// corrected export would report success and drop every such edit
			// in the file.
			p.Set("updated_at", current.UpdatedAt, time.Now().UTC())
		}
		if p.Empty() {
			out = current
			return nil
		}

		if in.Visibility != nil {
			if err := refuseStaleVisibility(ctx, tx, id, current); err != nil {
				return err
			}
		}
		if err := p.ApplyGuarded(ctx, tx, "contact", id.UUID, in.IfVersion); err != nil {
			return fmt.Errorf("apply contact patch: %w", err)
		}
		if in.Social != nil {
			if err := replaceContactSocial(ctx, tx, workspaceID(ctx), id, in.Social); err != nil {
				return err
			}
		}

		if in.Emails != nil || in.Phones != nil {
			by, err := storekit.CapturedBy(ctx)
			if err != nil {
				return err
			}
			if err := replaceContactEmails(ctx, tx, workspaceID(ctx), id, in.Source, by, in.Emails); err != nil {
				return err
			}
			if err := replaceContactPhones(ctx, tx, id, in.Source, by, in.Phones); err != nil {
				return err
			}
		}
		// AFTER the addresses are replaced, never before: the cohort pass
		// selects the correspondence to attach by reading this contact's live
		// contact_email rows, so running it first would file mail from an
		// address the same patch is removing — and the replacement archives
		// the address without retracting the links.
		if err := s.carryHistoryIfPublished(ctx, tx, id, current, in); err != nil {
			return err
		}
		before, after := contactChangeImages(p, current, in)
		auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "contact", id.UUID, before, after)
		if err != nil {
			return fmt.Errorf("audit contact update: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{ChangedFields: after}); err != nil {
			return fmt.Errorf("emit contact.updated: %w", err)
		}
		if out, err = readContact(ctx, tx, id, storekit.LiveOnly, active); err != nil {
			return fmt.Errorf("read updated contact: %w", err)
		}
		return nil
	})
	return out, err
}

// buildContactPatch stages only the fields the caller supplied, each
// diffed against the current row so the audit before/after captures the
// real change and an unchanged field is left out of the UPDATE.
// contactChangeImages is the before/after pair the audit row records.
//
// The patch knows the columns it staged; the relations it does not, because
// they are written as their own rows rather than as columns on the contact. So
// the three replaced sets are folded in here, and a relation the caller did not
// supply stays out of both images rather than appearing as an unchanged one.
func contactChangeImages(
	p *storekit.Patch, current crmcontracts.Contact, in UpdateContactInput,
) (before, after map[string]any) {
	before, after = p.Before(), p.After()
	if in.Social != nil {
		before["social"] = current.Social
		after["social"] = in.Social
	}
	if in.Emails != nil {
		before["emails"] = current.Emails
		after["emails"] = in.Emails
	}
	if in.Phones != nil {
		before["phones"] = current.Phones
		after["phones"] = in.Phones
	}
	return before, after
}

func buildContactPatch(current crmcontracts.Contact, in UpdateContactInput) (*storekit.Patch, error) {
	p := storekit.NewPatch()
	if in.FullName != nil {
		p.Set("full_name", current.FullName, *in.FullName)
	}
	if in.FirstName != nil {
		p.Set("first_name", current.FirstName, *in.FirstName)
	}
	if in.LastName != nil {
		p.Set("last_name", current.LastName, *in.LastName)
	}
	if in.Title != nil {
		p.Set("title", current.Title, *in.Title)
	}
	if in.OwnerID != nil {
		p.Set(ownerIDColumn, current.OwnerId, *in.OwnerID)
	}
	if in.Visibility != nil {
		p.Set("visibility", current.Visibility, *in.Visibility)
	}
	if err := storekit.ApplyClears(p, in.Clear, clearableContactColumns(current)); err != nil {
		return nil, err
	}
	if in.Address != nil {
		cur := addressColumns(current.Address)
		p.Set("address_line1", cur.Line1, in.Address.Line1)
		p.Set("address_line2", cur.Line2, in.Address.Line2)
		p.Set("address_city", cur.City, in.Address.City)
		p.Set("address_region", cur.Region, in.Address.Region)
		p.Set("address_postal_code", cur.PostalCode, in.Address.PostalCode)
		p.Set("address_country", cur.Country, in.Address.Country)
	}
	return p, nil
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
