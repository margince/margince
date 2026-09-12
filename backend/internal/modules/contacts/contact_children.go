// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

//nolint:unparam // all four child writers here take (ctx, tx, wsID, contactID, …); dropping wsID from the two that do not spend it would make a reader check which is which at every call site.
func replaceContactSocial(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, contactID ids.ContactID, social map[string]any) error {
	if social == nil {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM contact_social WHERE contact_id = $1`, contactID); err != nil {
		return fmt.Errorf("clear contact social rows: %w", err)
	}
	for platform, handle := range social {
		text := fmt.Sprintf("%v", handle)
		if strings.TrimSpace(platform) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_social (contact_id, platform, handle) VALUES ($1, $2, $3)`,
			contactID, platform, text); err != nil {
			return fmt.Errorf("insert contact social row: %w", err)
		}
	}
	return nil
}

// replaceContactEmails makes the contact's LIVE addresses mirror the given set.
// nil means "not supplied": existing rows stand.
//
// It ARCHIVES rather than deletes, which is the one place this differs from
// replaceContactSocial above. contact_social carries no cross-record uniqueness
// and no history; contact_email carries both. uq_contact_email_dedupe is what
// makes an address name exactly one contact, and the dedupe ladder, the merge
// trail and every audit read filter on archived_at IS NULL. A hard DELETE would
// erase the evidence that a contact ever held an address — which is the thing a
// merge dispute is settled by.
//
// Order is load-bearing: archive first, then insert. The reverse would put two
// is_primary rows of one type on the record while both are live, which the
// schema answers with a bare conflict.
//
//nolint:cyclop // the rename added no branch: this body is what it was under the old noun.
func replaceContactEmails(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, contactID ids.ContactID, source, by string, emails []ContactEmailInput) error {
	if emails == nil {
		return nil
	}
	if err := parseContactContacts(emails, nil); err != nil {
		return err
	}
	// The claim check runs before anything is written and skips this contact's
	// own rows, so re-stating an address the contact already holds is not
	// refused by that contact's own row. Without it the unique index would still
	// refuse the insert, but the aborted transaction cannot re-query — so the
	// refusal would reach the caller with no incumbent id to name.
	if err := ensureContactEmailsUnclaimedExcept(ctx, tx, contactID, emails); err != nil {
		return err
	}

	held, err := liveContactEmails(ctx, tx, contactID)
	if err != nil {
		return err
	}

	keep := make([]string, 0, len(emails))
	fresh := make([]ContactEmailInput, 0, len(emails))
	for _, e := range emails {
		keep = append(keep, strings.ToLower(e.Email))
		if !held[strings.ToLower(e.Email)] {
			fresh = append(fresh, e)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE contact_email SET archived_at = now()
		  WHERE contact_id = $1 AND archived_at IS NULL AND email <> ALL($2)`,
		contactID, keep); err != nil {
		return fmt.Errorf("archive contact emails: %w", err)
	}
	// Retained rows are re-placed BEFORE the new addresses land, and demotions
	// before promotions within that. uq_contact_email_primary allows one primary
	// per (contact_id, email_type), and both movements meet it:
	//
	// A swap of which of two same-type rows is primary travels this loop with both
	// rows retained; promoting the incoming one while the stored one is still
	// primary is two live primaries of one type for the length of a statement,
	// which the index refuses. So does a corrected export, where the stored
	// primary is carried through demoted and the file's new one promoted.
	//
	// Demoting first empties the slot every later write — this loop's promotions
	// and the archive-then-insert below alike — is about to claim.
	place := func(e ContactEmailInput) error {
		if _, err := tx.Exec(ctx,
			`UPDATE contact_email SET email_type = $3, is_primary = $4, position = $5
			  WHERE contact_id = $1 AND email = lower($2) AND archived_at IS NULL`,
			contactID, e.Email, e.EmailType, e.IsPrimary, e.Position); err != nil {
			if _, ok := storekit.UniqueViolation(err); ok {
				return apperrors.ErrConflict
			}
			return fmt.Errorf("update contact email placement: %w", err)
		}
		return nil
	}
	for _, e := range emails {
		if held[strings.ToLower(e.Email)] && !e.IsPrimary {
			if err := place(e); err != nil {
				return err
			}
		}
	}
	for _, e := range emails {
		if held[strings.ToLower(e.Email)] && e.IsPrimary {
			if err := place(e); err != nil {
				return err
			}
		}
	}
	// Only the addresses this contact does not already hold are inserted: a held
	// address would collide with its own live row on the unique index.
	if err := insertContactEmails(ctx, tx, wsID, contactID, source, by, fresh); err != nil {
		return err
	}
	return nil
}

// liveContactEmails answers the addresses a contact currently holds, lowercased
// as the column stores them. Archived rows are excluded: they are history, and
// re-inserting one would collide with nothing while telling the caller it did.
func liveContactEmails(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (map[string]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT email FROM contact_email WHERE contact_id = $1 AND archived_at IS NULL`, contactID)
	if err != nil {
		return nil, fmt.Errorf("read contact emails: %w", err)
	}
	defer rows.Close()
	held := map[string]bool{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan contact email: %w", err)
		}
		held[email] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read contact emails: %w", err)
	}
	return held, nil
}

// parseContactContacts is the parse-don't-validate seam for a contact's
// contact rows: addresses normalize to the lowercased form the dedupe
// index compares (the SQL lower() below stays as defense in depth) and
// phones normalize to E.164 — making the schema's "E.164 normalized at
// write" contract true instead of documentary. Values are written back
// in place so everything downstream handles only normalized strings.
//
// It also refuses a set that marks two rows of one type primary. This is the
// one seam create, update and vCard import all pass through, so the intent
// behind uq_contact_{email,phone}_primary is enforced here once rather than at
// each writer, and the contradiction is answered with a typed refusal before
// any write instead of the bare conflict the index would raise.
func parseContactContacts(emails []ContactEmailInput, phones []ContactPhoneInput) error {
	for i, e := range emails {
		parsed, err := values.ParseEmail(e.Email)
		if err != nil {
			return err
		}
		emails[i].Email = parsed.String()
	}
	for i, p := range phones {
		parsed, err := values.ParsePhone(p.Phone)
		if err != nil {
			return err
		}
		phones[i].Phone = parsed.String()
	}
	if err := ensureOnePrimaryPerType("address", emails, func(e ContactEmailInput) (string, bool) {
		return e.EmailType, e.IsPrimary
	}); err != nil {
		return err
	}
	return ensureOnePrimaryPerType("phone number", phones, func(p ContactPhoneInput) (string, bool) {
		return p.PhoneType, p.IsPrimary
	})
}

//nolint:unparam // the same shape as its siblings; see replaceContactSocial.
func insertContactEmails(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, contactID ids.ContactID, source, by string, emails []ContactEmailInput) error {
	for _, e := range emails {
		if _, err := tx.Exec(ctx,
			`INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by, from_correspondence)
			 VALUES ($1, lower($2), $3, $4, $5, $6, $7, $8)`,
			contactID, e.Email, e.EmailType, e.IsPrimary, e.Position, source, by,
			!e.VouchedNotCorresponded); err != nil {
			if name, ok := storekit.UniqueViolation(err); ok {
				if name == "uq_contact_email_dedupe" {
					return &DuplicateEmailError{Email: e.Email}
				}
				return apperrors.ErrConflict
			}
			return fmt.Errorf("insert contact email: %w", err)
		}
	}
	return nil
}

// ensureContactEmailsUnclaimed is the dedupe pre-check, so the 409 can
// carry the existing id; the unique index remains the structural
// guarantee under races. The existing id is disclosed only when the
// caller could read that row; the conflict itself is still answered
// (existence-hiding survives the 409).
func ensureContactEmailsUnclaimed(ctx context.Context, tx pgx.Tx, emails []ContactEmailInput) error {
	return ensureContactEmailsUnclaimedExcept(ctx, tx, ids.ContactID{}, emails)
}

// ensureContactEmailsUnclaimedExcept is the same probe, ignoring one contact's
// own rows. An update that re-states an address the contact already holds must
// not be refused by that contact's own row, which is the only difference between
// the create case (nothing to exclude) and the replace case.
func ensureContactEmailsUnclaimedExcept(ctx context.Context, tx pgx.Tx, self ids.ContactID, emails []ContactEmailInput) error {
	for _, e := range emails {
		var existing ids.ContactID
		err := tx.QueryRow(ctx,
			`SELECT contact_id FROM contact_email
			  WHERE email = lower($1) AND archived_at IS NULL AND contact_id <> $2`,
			e.Email, self.UUID).Scan(&existing)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("probe email dedupe: %w", err)
		}
		dup := &DuplicateEmailError{Email: e.Email}
		visible, err := auth.VisibleTo(ctx, tx, "contact", existing.UUID)
		if err != nil {
			return err
		}
		if visible {
			dup.ExistingID = existing
		}
		return dup
	}
	return nil
}

const contactColumns = `id, full_name, first_name, last_name, title, owner_id, visibility,
	address_line1, address_line2, address_city, address_region, address_postal_code, address_country,
	merged_into_id, converted_from_lead_id, source, captured_by,
	version, created_at, updated_at, archived_at, last_activity_at`

// readContact resolves one contact row; active names the custom-field
// columns to carry alongside the core ones — nil for internal decision
// reads whose result never reaches the wire.
func readContact(ctx context.Context, tx pgx.Tx, id ids.ContactID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Contact, error) {
	q := `SELECT ` + contactColumns + storekit.SelectSuffix(active) + ` FROM contact WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += ` AND archived_at IS NULL`
	}
	row := tx.QueryRow(ctx, q, id)
	p, err := scanContact(row, active)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Contact{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Contact{}, err
	}

	contacts := []crmcontracts.Contact{p}
	if err := attachContactChildren(ctx, tx, contacts); err != nil {
		return crmcontracts.Contact{}, err
	}
	return contacts[0], nil
}

// scanContact scans core + active custom columns; extra receives any
// trailing expressions the caller's SELECT appended (the sorted list's
// cursor key).
func scanContact(row pgx.Row, active []fieldcatalog.Column, extra ...any) (crmcontracts.Contact, error) {
	var p crmcontracts.Contact
	var id ids.UUID
	var ownerID, mergedInto, fromLead *ids.UUID
	var addr crmcontracts.Address
	var version int64
	var visibility string

	dests := []any{
		&id, &p.FullName, &p.FirstName, &p.LastName, &p.Title, &ownerID, &visibility,
		&addr.Line1, &addr.Line2, &addr.City, &addr.Region, &addr.PostalCode, &addr.Country,
		&mergedInto, &fromLead, &p.Source, &p.CapturedBy,
		&version, &p.CreatedAt, &p.UpdatedAt, &p.ArchivedAt, &p.LastActivityAt,
	}
	cf := storekit.ScanDests(active)
	if err := row.Scan(append(append(dests, cf...), extra...)...); err != nil {
		return p, err
	}
	if values := storekit.ExtractValues(active, cf); len(values) > 0 {
		p.AdditionalProperties = values
	}

	p.Id = openapi_types.UUID(id)
	p.OwnerId = uuidPtr(ownerID)
	if v := crmcontracts.ContactVisibility(visibility); v != "" {
		p.Visibility = &v
	}
	p.MergedIntoId = uuidPtr(mergedInto)
	p.ConvertedFromLeadId = uuidPtr(fromLead)
	if a := addressOrNil(addr); a != nil {
		p.Address = a
	}
	p.Version = &version
	return p, nil
}

// attachContactChildren loads emails + phones + social for a page in
// three queries, not 3N.
func attachContactChildren(ctx context.Context, tx pgx.Tx, contacts []crmcontracts.Contact) error {
	if len(contacts) == 0 {
		return nil
	}
	// Whether each row is this caller's to change, one statement for the page.
	// It is stamped HERE because this is the seam the list and the single read
	// already share: a client is otherwise left inferring write access from the
	// object grant alone, which says nothing about who owns the row.
	if _, err := auth.StampWritable(ctx, tx, "contact", contacts,
		func(p crmcontracts.Contact) ids.UUID { return ids.UUID(p.Id) },
		func(p *crmcontracts.Contact, may bool) { p.Writable = &may }); err != nil {
		return err
	}
	idx := make(map[openapi_types.UUID]*crmcontracts.Contact, len(contacts))
	contactIDs := make([]ids.UUID, len(contacts))
	for i := range contacts {
		idx[contacts[i].Id] = &contacts[i]
		contactIDs[i] = ids.UUID(contacts[i].Id)
	}
	if err := attachContactEmails(ctx, tx, idx, contactIDs); err != nil {
		return err
	}
	if err := attachContactPhones(ctx, tx, idx, contactIDs); err != nil {
		return err
	}
	if err := attachContactSocial(ctx, tx, idx, contactIDs); err != nil {
		return err
	}
	if err := attachContactEmployers(ctx, tx, idx, contactIDs); err != nil {
		return err
	}
	if err := storekit.AttachRowTags(ctx, tx, contactEntity, contacts,
		func(p crmcontracts.Contact) ids.UUID { return ids.UUID(p.Id) },
		func(p *crmcontracts.Contact, tags []storekit.RowTag) { p.Tags = wireRowTags(tags) }); err != nil {
		return err
	}
	return attachContactReachability(ctx, tx, idx, contactIDs)
}

func attachContactPhones(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Contact, contactIDs []ids.UUID) error {
	phoneRows, err := tx.Query(ctx,
		`SELECT contact_id, id, phone, phone_type, is_primary, position, source, captured_by
		 FROM contact_phone WHERE contact_id = ANY($1) AND archived_at IS NULL
		 ORDER BY position, created_at`, contactIDs)
	if err != nil {
		return err
	}
	defer phoneRows.Close()
	for phoneRows.Next() {
		var contactID, phoneID ids.UUID
		var ph crmcontracts.ContactPhone
		if err := phoneRows.Scan(&contactID, &phoneID, &ph.Phone, &ph.PhoneType, &ph.IsPrimary, &ph.Position, &ph.Source, &ph.CapturedBy); err != nil {
			return err
		}
		ph.Id = openapi_types.UUID(phoneID)
		p := idx[openapi_types.UUID(contactID)]
		if p.Phones == nil {
			p.Phones = &[]crmcontracts.ContactPhone{}
		}
		*p.Phones = append(*p.Phones, ph)
	}
	return phoneRows.Err()
}

func attachContactSocial(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Contact, contactIDs []ids.UUID) error {
	// The wire keeps social as the (platform → handle) map; the relation
	// is the stored form.
	socialRows, err := tx.Query(ctx,
		`SELECT contact_id, platform, handle FROM contact_social WHERE contact_id = ANY($1)
		 ORDER BY platform`, contactIDs)
	if err != nil {
		return err
	}
	defer socialRows.Close()
	for socialRows.Next() {
		var contactID ids.UUID
		var platform, handle string
		if err := socialRows.Scan(&contactID, &platform, &handle); err != nil {
			return err
		}
		p := idx[openapi_types.UUID(contactID)]
		if p.Social == nil {
			p.Social = &map[string]any{}
		}
		(*p.Social)[platform] = handle
	}
	return socialRows.Err()
}

// attachContactReachability loads the reachability projection (design §6.6):
// {provider, reachable, since}, never the raw channel_user_id — an opaque
// third-party account identifier belongs in a governed read, not this broad
// one. A blocked identity is still a live row (archived_at IS NULL), so it
// still appears here, with reachable=false — the record must keep showing
// that a conversation exists even when a reply cannot currently be delivered.
func attachContactReachability(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Contact, contactIDs []ids.UUID) error {
	rows, err := tx.Query(ctx,
		`SELECT contact_id, provider, blocked_at, created_at
		 FROM contact_channel_identity WHERE contact_id = ANY($1) AND archived_at IS NULL
		 ORDER BY provider, created_at`, contactIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var contactID ids.UUID
		var provider string
		var blockedAt, createdAt time.Time
		var blockedAtPtr *time.Time
		if err := rows.Scan(&contactID, &provider, &blockedAtPtr, &createdAt); err != nil {
			return err
		}
		since := createdAt
		if blockedAtPtr != nil {
			blockedAt = *blockedAtPtr
			since = blockedAt
		}
		r := crmcontracts.ContactReachability{
			Provider:  provider,
			Reachable: blockedAtPtr == nil,
			Since:     since,
		}
		p := idx[openapi_types.UUID(contactID)]
		if p.Reachability == nil {
			p.Reachability = &[]crmcontracts.ContactReachability{}
		}
		*p.Reachability = append(*p.Reachability, r)
	}
	return rows.Err()
}
