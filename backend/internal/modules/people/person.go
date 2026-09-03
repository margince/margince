// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

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

// DuplicateEmailError carries the existing person for the 409 dedupe
// contract (data-model §3.2: "create with an existing email returns 409 +
// existing id").
type DuplicateEmailError struct {
	Email      string
	ExistingID ids.PersonID
}

func (e *DuplicateEmailError) Error() string {
	return "person with email " + e.Email + " already exists"
}
func (e *DuplicateEmailError) Is(target error) bool { return target == apperrors.ErrConflict }

// PersonEmailInput / PersonPhoneInput are the child rows a create carries.
type PersonEmailInput struct {
	Email     string
	EmailType string
	IsPrimary bool
	Position  int
	// VouchedNotCorresponded marks an address a provider's directory supplied
	// for a human reached on another medium, rather than one this workspace has
	// ever exchanged mail with.
	//
	// It identifies the person, which is what it is stored for, and it proves
	// nothing about mail — so the mail ladder must not read it as a settled
	// verdict about the address. False for every writer that has correspondence
	// or a human's own assertion behind it, which is every writer but one.
	VouchedNotCorresponded bool
}

type PersonPhoneInput struct {
	Phone     string
	PhoneType string
	IsPrimary bool
	Position  int
}

type CreatePersonInput struct {
	// Acquisition says why this contact exists — what the person did, or what
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
	Emails      []PersonEmailInput
	Phones      []PersonPhoneInput
	Source      string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

// CreatePerson inserts the person + child rows + audit + event atomically.
// The email dedupe unique index turns a duplicate into the 409 contract.
func (s *Store) CreatePerson(ctx context.Context, in CreatePersonInput) (crmcontracts.Person, error) {
	if err := auth.Require(ctx, "person", principal.ActionCreate); err != nil {
		return crmcontracts.Person{}, err
	}
	by, err := s.readyPersonCreate(ctx, in)
	if err != nil {
		return crmcontracts.Person{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	// The store-opened path reads the catalog through the unexported helper,
	// not ActivePersonColumns: that one takes person:read on the caller's
	// behalf, and a seat may hold create without it.
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return crmcontracts.Person{}, err
	}

	var out crmcontracts.Person
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = createPersonInTx(ctx, tx, in, by, active)
		return err
	})
	return out, err
}

// CreatePersonTx is CreatePerson for a caller that already opened a
// transaction — one whose own write must land with this person or not at all.
// Same gates in the same order; only the transaction is borrowed.
//
// Custom fields are refused rather than dropped: the catalog they are matched
// against is read in a transaction of its own, which is exactly the second
// connection this seam exists to avoid taking.
func (s *Store) CreatePersonTx(ctx context.Context, tx pgx.Tx, in CreatePersonInput) (crmcontracts.Person, error) {
	if err := auth.Require(ctx, "person", principal.ActionCreate); err != nil {
		return crmcontracts.Person{}, err
	}
	if err := refuseCustomFields(in.CustomFields); err != nil {
		return crmcontracts.Person{}, err
	}
	by, err := s.readyPersonCreate(ctx, in)
	if err != nil {
		return crmcontracts.Person{}, err
	}
	in.OwnerID = storekit.OwnerOrActor(ctx, in.OwnerID)
	return createPersonInTx(ctx, tx, in, by, nil)
}

// readyPersonCreate runs what a create settles BEFORE any transaction opens —
// the contact parse and the captured-by resolution — and answers the
// attribution the write shape stamps. Both entry points call it, so neither
// can drift from the other's validation.
func (s *Store) readyPersonCreate(ctx context.Context, in CreatePersonInput) (string, error) {
	if err := parsePersonContacts(in.Emails, in.Phones); err != nil {
		return "", err
	}
	return storekit.CapturedBy(ctx)
}

// createPersonInTx is CreatePerson's transactional body, shared by the
// store-opened and caller-opened entry points.
func createPersonInTx(ctx context.Context, tx pgx.Tx, in CreatePersonInput, by string,
	active []fieldcatalog.Column,
) (crmcontracts.Person, error) {
	if err := ensurePersonEmailsUnclaimed(ctx, tx, in.Emails); err != nil {
		return crmcontracts.Person{}, err
	}

	match, err := manualDedupePerson(ctx, tx, in)
	if err != nil {
		return crmcontracts.Person{}, err
	}

	id, err := createPerson(ctx, tx, match, PersonSpec{
		// Whatever the caller declared. An unset kind records unknown_legacy,
		// which is the honest answer for a contact somebody typed in without
		// saying why — and the answer that makes the gap visible rather than
		// leaving the question unasked.
		Acquisition:  in.Acquisition,
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
		return crmcontracts.Person{}, err
	}

	auditID, err := storekit.Audit(ctx, tx, "create", "person", id.UUID, nil, map[string]any{"full_name": in.FullName})
	if err != nil {
		return crmcontracts.Person{}, fmt.Errorf("audit person create: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventPersonCreated{FullName: in.FullName}); err != nil {
		return crmcontracts.Person{}, fmt.Errorf("emit person.created: %w", err)
	}
	if err := match.recordIfReview(ctx, tx, id, in.FullName, in.Source, by); err != nil {
		return crmcontracts.Person{}, err
	}

	out, err := readPerson(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return crmcontracts.Person{}, fmt.Errorf("read created person: %w", err)
	}
	return out, nil
}

// GetPerson returns one person with child rows; archived rows resolve
// only under IncludeArchived (they stay fetchable by id after merge).
func (s *Store) GetPerson(ctx context.Context, id ids.PersonID, archived storekit.ArchivedFilter) (crmcontracts.Person, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return crmcontracts.Person{}, err
	}
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return crmcontracts.Person{}, err
	}
	var out crmcontracts.Person
	err = s.tx(ctx, func(tx pgx.Tx) (err error) {
		if err := auth.EnsureVisible(ctx, tx, "person", id.UUID); err != nil {
			return err
		}
		out, err = readPerson(ctx, tx, id, archived, active)
		return err
	})
	return out, err
}

// GetPersonTx is GetPerson for a caller that already opened a transaction —
// the composite record read, which must see every one of its sections at the
// same instant and cannot afford a second connection per section. Same gates
// in the same order; only the transaction is borrowed.
//
// active is the caller's to fetch, with ActivePersonColumns, before it opens
// that transaction: the catalog read runs a transaction of its own, and a
// second connection taken from inside the caller's would commit separately and
// block undetectably against a lock the caller already holds.
func (s *Store) GetPersonTx(ctx context.Context, tx pgx.Tx, id ids.PersonID,
	archived storekit.ArchivedFilter, active CustomColumns,
) (crmcontracts.Person, error) {
	if err := auth.Require(ctx, "person", principal.ActionRead); err != nil {
		return crmcontracts.Person{}, err
	}
	if err := auth.EnsureVisible(ctx, tx, "person", id.UUID); err != nil {
		return crmcontracts.Person{}, err
	}
	return readPerson(ctx, tx, id, archived, active.cols)
}

type UpdatePersonInput struct {
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
	Social    map[string]any
	Address   *crmcontracts.Address
	// Emails replaces the person's live addresses when non-nil. nil is "not
	// supplied" and leaves the stored rows standing, exactly as Social is —
	// the distinction matters for an import whose file carried no email
	// column at all, which must not read as "this person now has none".
	Emails    []PersonEmailInput
	IfVersion *int64
	Source    string
	// CustomFields carries the request body's extra top-level keys
	// (additionalProperties); only active cf_* catalog columns land,
	// drop-on-mismatch (customfields.go).
	CustomFields map[string]any
}

func (s *Store) UpdatePerson(ctx context.Context, id ids.PersonID, in UpdatePersonInput) (crmcontracts.Person, error) {
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return crmcontracts.Person{}, err
	}
	active, err := s.activeColumns(ctx, "person")
	if err != nil {
		return crmcontracts.Person{}, err
	}
	var out crmcontracts.Person
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, "person", id.UUID); err != nil {
			return err
		}
		current, err := readPerson(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read person before update: %w", err)
		}

		p, err := buildPersonPatch(current, in)
		if err != nil {
			return err
		}
		storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)
		if in.Social != nil || in.Emails != nil {
			// The relation replacement rides the person row's version
			// bump (updated_at below), so If-Match still guards it and
			// the audit row still records the transition.
			//
			// Emails is in this condition for a second reason: without it a
			// row whose ONLY change is an address hits p.Empty() below and
			// returns having written nothing, so a corrected export would
			// report success and drop every email edit in the file.
			p.Set("updated_at", current.UpdatedAt, time.Now().UTC())
		}
		if p.Empty() {
			out = current
			return nil
		}

		if err := p.ApplyGuarded(ctx, tx, "person", id.UUID, in.IfVersion); err != nil {
			return fmt.Errorf("apply person patch: %w", err)
		}
		if in.Social != nil {
			if err := replacePersonSocial(ctx, tx, workspaceID(ctx), id, in.Social); err != nil {
				return err
			}
		}
		if in.Emails != nil {
			by, err := storekit.CapturedBy(ctx)
			if err != nil {
				return err
			}
			if err := replacePersonEmails(ctx, tx, workspaceID(ctx), id, in.Source, by, in.Emails); err != nil {
				return err
			}
		}
		before, after := p.Before(), p.After()
		if in.Social != nil {
			before["social"] = current.Social
			after["social"] = in.Social
		}
		if in.Emails != nil {
			before["emails"] = current.Emails
			after["emails"] = in.Emails
		}
		auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "person", id.UUID, before, after)
		if err != nil {
			return fmt.Errorf("audit person update: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventPersonUpdated{ChangedFields: after}); err != nil {
			return fmt.Errorf("emit person.updated: %w", err)
		}
		if out, err = readPerson(ctx, tx, id, storekit.LiveOnly, active); err != nil {
			return fmt.Errorf("read updated person: %w", err)
		}
		return nil
	})
	return out, err
}

// buildPersonPatch stages only the fields the caller supplied, each
// diffed against the current row so the audit before/after captures the
// real change and an unchanged field is left out of the UPDATE.
func buildPersonPatch(current crmcontracts.Person, in UpdatePersonInput) (*storekit.Patch, error) {
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
	if err := storekit.ApplyClears(p, in.Clear, clearablePersonColumns(current)); err != nil {
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

func (s *Store) EnsurePersonByEmail(ctx context.Context, fullName, email, source string) (ids.UUID, error) {
	if err := auth.Require(ctx, "person", principal.ActionCreate); err != nil {
		return ids.Nil, err
	}
	lookup := func() (ids.UUID, bool, error) {
		var id ids.UUID
		found := false
		err := s.tx(ctx, func(tx pgx.Tx) error {
			err := tx.QueryRow(ctx, `
				SELECT p.id FROM person p
				JOIN person_email e ON e.person_id = p.id
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
	created, err := s.CreatePerson(ctx, CreatePersonInput{
		FullName: fullName,
		Emails:   []PersonEmailInput{{Email: email, EmailType: "work", IsPrimary: true}},
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
