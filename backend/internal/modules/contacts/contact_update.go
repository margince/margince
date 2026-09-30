// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Editing a contact: the partial write, the patch it builds, and the images its
// audit row records.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

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
	// direction, for anybody the write gate admits. Narrowing stays open because
	// the sender classifier publishes contacts with no human approving it, and
	// its owner must be able to take that back.
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
		var err error
		out, err = s.updateContactInTx(ctx, tx, id, in, active)
		return err
	})
	return out, err
}

// updateContactInTx is the edit itself, on the caller's transaction: the write
// check, the patch, the child rows it replaces, and the record of what it did.
//
//nolint:cyclop // one edit applies each optional field in turn; splitting it would scatter the single write it records.
func (s *Store) updateContactInTx(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, in UpdateContactInput, active []fieldcatalog.Column,
) (crmcontracts.Contact, error) {
	var out crmcontracts.Contact
	if err := auth.EnsureChangeable(ctx, tx, "contact", id.UUID); err != nil {
		return out, err
	}
	current, err := readContact(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return out, fmt.Errorf("read contact before update: %w", err)
	}

	if err := refuseUnreadableResult(current, in); err != nil {
		return out, err
	}
	if err := auth.EnsureOwnerHandOn(ctx, tx, (*ids.UUID)(current.OwnerId), in.OwnerID); err != nil {
		return out, err
	}
	in.Clear = storekit.CoreFieldClears(in.Clear, active, in.CustomFields)
	p, err := buildContactPatch(current, in)
	if err != nil {
		return out, err
	}
	storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties)
	if p.Empty() {
		return current, nil
	}

	if err := guardVisibilityWrite(ctx, tx, p, id, current, in.Visibility); err != nil {
		return out, err
	}
	if err := p.ApplyGuarded(ctx, tx, "contact", id.UUID, in.IfVersion); err != nil {
		if constraint, ok := storekit.CheckViolation(err); ok && constraint == "contact_owner_private_names_its_owner" {
			return out, &RequiredFieldError{Field: filterOwnerID}
		}
		return out, fmt.Errorf("apply contact patch: %w", err)
	}
	if in.Social != nil {
		if err := replaceContactSocial(ctx, tx, workspaceID(ctx), id, in.Social); err != nil {
			return out, err
		}
	}

	if in.Emails != nil || in.Phones != nil {
		by, err := storekit.CapturedBy(ctx)
		if err != nil {
			return out, err
		}
		if err := replaceContactEmails(ctx, tx, workspaceID(ctx), id, in.Source, by, in.Emails); err != nil {
			return out, err
		}
		if err := replaceContactPhones(ctx, tx, id, in.Source, by, in.Phones); err != nil {
			return out, err
		}
	}
	// AFTER the addresses are replaced, never before: the cohort pass
	// selects the correspondence to attach by reading this contact's live
	// contact_email rows, so running it first would file mail from an
	// address the same patch is removing — and the replacement archives
	// the address without retracting the links.
	if err := s.carryHistoryIfPublished(ctx, tx, id, current, in); err != nil {
		return out, err
	}
	before, after := contactChangeImages(p, current, in)
	auditID, err := storekit.AuditWithTrail(ctx, tx, in.Trail, "contact", id.UUID, before, after)
	if err != nil {
		return out, fmt.Errorf("audit contact update: %w", err)
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{ChangedFields: after}); err != nil {
		return out, fmt.Errorf("emit contact.updated: %w", err)
	}
	if err := recheckIfRenamed(ctx, tx, id, current.FullName, in); err != nil {
		return out, err
	}
	if out, err = readContact(ctx, tx, id, storekit.LiveOnly, active); err != nil {
		return out, fmt.Errorf("read updated contact: %w", err)
	}
	return out, nil
}

// Child sets are separate rows, so their audit images supplement the column patch.
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
	return p, nil
}
