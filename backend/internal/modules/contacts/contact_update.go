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
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
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
	// RelationshipMark marks the contact; clearing it goes through Clear. The
	// setter is read from the signed-in principal, never from the caller.
	RelationshipMark *relationshipmark.Mark
	Social           map[string]any
	Address          *crmcontracts.Address
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
	// CustomFields carries the request body's cf_* keys. A key no active catalog
	// column takes is refused (storekit.CheckCustomValues).
	CustomFields map[string]any
	// ConvertLeadIDs are the live leads that hold an address or profile this
	// edit gives the contact, which the user chose to convert into it.
	ConvertLeadIDs []ids.LeadID
	// converting is ConvertLeadIDs once checked against the holders under the
	// identity locks; only that check sets it.
	converting []ids.LeadID
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
	return out, s.AskHolderToShare(ctx, err)
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
	if err := s.lockConvertStops(ctx, tx, id, in.ConvertLeadIDs); err != nil {
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
	setter, err := relationshipMarkSetter(ctx, in)
	if err != nil {
		return out, err
	}
	p, err := buildContactPatch(current, in, setter)
	if err != nil {
		return out, err
	}
	// The If-Match check comes first. The replace-sets write child rows before the
	// guarded write could refuse a stale version. A stale version also answers 409
	// before a refused custom field answers 422.
	if err := storekit.EnsureIfMatch(ctx, tx, "contact", id.UUID, in.IfVersion); err != nil {
		return out, err
	}
	if err := storekit.SetCustomFieldPatch(p, active, in.CustomFields, current.AdditionalProperties); err != nil {
		return out, err
	}
	if err := admitEditLeadHolders(ctx, tx, id, current, &in); err != nil {
		return out, err
	}
	if err := lockEditedContactLanes(ctx, tx, id, current, in); err != nil {
		return out, err
	}
	setsChanged, err := applyContactReplaceSets(ctx, tx, id, current, in, active)
	if err != nil {
		return out, err
	}
	if setsChanged.any {
		// A replace-set changes no column on the row, so this bump carries the
		// version guard and the audit row.
		p.Set("updated_at", current.UpdatedAt, time.Now().UTC())
	}
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
	if err := recheckIfEdited(ctx, tx, id, current.FullName, in, setsChanged.emails || setsChanged.phones); err != nil {
		return out, err
	}
	if err := s.convertEditedLeads(ctx, tx, id, in.converting); err != nil {
		return out, err
	}
	if out, err = readContact(ctx, tx, id, storekit.LiveOnly, active); err != nil {
		return out, fmt.Errorf("read updated contact: %w", err)
	}
	return out, s.followShareOwnerTx(ctx, tx, id, (*ids.UUID)(current.OwnerId), (*ids.UUID)(out.OwnerId))
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

func buildContactPatch(current crmcontracts.Contact, in UpdateContactInput, markSetter string) (*storekit.Patch, error) {
	p := storekit.NewPatch()
	if in.FullName != nil {
		name, err := httperr.RequireNonBlank("full_name", *in.FullName)
		if err != nil {
			return nil, err
		}
		p.Set("full_name", current.FullName, name)
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
	if err := patchRelationshipMark(p, current, in, markSetter); err != nil {
		return nil, err
	}
	if err := setAddress(p, current.Address, in.Address, in.Trail); err != nil {
		return nil, err
	}
	return p, nil
}
