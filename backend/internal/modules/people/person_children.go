// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

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

// replacePersonSocial makes the person_social relation mirror the given
// (platform → handle) map — the queryable form of what used to hide in
// a jsonb column. nil means "not supplied": existing rows stand.
func replacePersonSocial(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, personID ids.PersonID, social map[string]any) error {
	if social == nil {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM person_social WHERE person_id = $1`, personID); err != nil {
		return fmt.Errorf("clear person social rows: %w", err)
	}
	for platform, handle := range social {
		text := fmt.Sprintf("%v", handle)
		if strings.TrimSpace(platform) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO person_social (person_id, platform, handle) VALUES ($1, $2, $3)`,
			personID, platform, text); err != nil {
			return fmt.Errorf("insert person social row: %w", err)
		}
	}
	return nil
}

// replacePersonEmails makes the person's LIVE addresses mirror the given set.
// nil means "not supplied": existing rows stand.
//
// It ARCHIVES rather than deletes, which is the one place this differs from
// replacePersonSocial above. person_social carries no cross-record uniqueness
// and no history; person_email carries both. uq_person_email_dedupe is what
// makes an address name exactly one person, and the dedupe ladder, the merge
// trail and every audit read filter on archived_at IS NULL. A hard DELETE would
// erase the evidence that a person ever held an address — which is the thing a
// merge dispute is settled by.
//
// Order is load-bearing: archive first, then insert. The reverse would put two
// is_primary rows of one type on the record while both are live, which the
// schema answers with a bare conflict.
func replacePersonEmails(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, personID ids.PersonID, source, by string, emails []PersonEmailInput) error {
	if emails == nil {
		return nil
	}
	if err := parsePersonContacts(emails, nil); err != nil {
		return err
	}
	// The claim check runs before anything is written and skips this person's
	// own rows, so re-stating an address the person already holds is not
	// refused by that person's own row. Without it the unique index would still
	// refuse the insert, but the aborted transaction cannot re-query — so the
	// refusal would reach the caller with no incumbent id to name.
	if err := ensurePersonEmailsUnclaimedExcept(ctx, tx, personID, emails); err != nil {
		return err
	}

	held, err := livePersonEmails(ctx, tx, personID)
	if err != nil {
		return err
	}

	keep := make([]string, 0, len(emails))
	fresh := make([]PersonEmailInput, 0, len(emails))
	for _, e := range emails {
		keep = append(keep, strings.ToLower(e.Email))
		if !held[strings.ToLower(e.Email)] {
			fresh = append(fresh, e)
		}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE person_email SET archived_at = now()
		  WHERE person_id = $1 AND archived_at IS NULL AND email <> ALL($2)`,
		personID, keep); err != nil {
		return fmt.Errorf("archive person emails: %w", err)
	}
	// Retained rows are re-placed BEFORE the new addresses land, and that order is
	// the whole correctness of this function.
	//
	// uq_person_email_primary allows one primary per (person_id, email_type). The
	// ordinary correction — a person's work address changed, and the file carries
	// the new one while this person's other stored addresses are carried through —
	// produces two work rows where the stored one is still primary and the
	// incoming one wants to be. Inserting first makes both live primaries of one
	// type for the length of a statement, which the index refuses, and the whole
	// run fails on the most common row a corrected export contains.
	//
	// Demoting first empties the slot the insert is about to claim.
	for _, e := range emails {
		if !held[strings.ToLower(e.Email)] {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE person_email SET email_type = $3, is_primary = $4, position = $5
			  WHERE person_id = $1 AND email = lower($2) AND archived_at IS NULL`,
			personID, e.Email, e.EmailType, e.IsPrimary, e.Position); err != nil {
			if _, ok := storekit.UniqueViolation(err); ok {
				return apperrors.ErrConflict
			}
			return fmt.Errorf("update person email placement: %w", err)
		}
	}
	// Only the addresses this person does not already hold are inserted: a held
	// address would collide with its own live row on the unique index.
	if err := insertPersonEmails(ctx, tx, wsID, personID, source, by, fresh); err != nil {
		return err
	}
	return nil
}

// livePersonEmails answers the addresses a person currently holds, lowercased
// as the column stores them. Archived rows are excluded: they are history, and
// re-inserting one would collide with nothing while telling the caller it did.
func livePersonEmails(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (map[string]bool, error) {
	rows, err := tx.Query(ctx,
		`SELECT email FROM person_email WHERE person_id = $1 AND archived_at IS NULL`, personID)
	if err != nil {
		return nil, fmt.Errorf("read person emails: %w", err)
	}
	defer rows.Close()
	held := map[string]bool{}
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, fmt.Errorf("scan person email: %w", err)
		}
		held[email] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read person emails: %w", err)
	}
	return held, nil
}

// parsePersonContacts is the parse-don't-validate seam for a person's
// contact rows: addresses normalize to the lowercased form the dedupe
// index compares (the SQL lower() below stays as defense in depth) and
// phones normalize to E.164 — making the schema's "E.164 normalized at
// write" contract true instead of documentary. Values are written back
// in place so everything downstream handles only normalized strings.
func parsePersonContacts(emails []PersonEmailInput, phones []PersonPhoneInput) error {
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
	return nil
}

// insertPersonEmails lands the person's emails; the unique index stays
// the structural guarantee under races, mapping uq_person_email_dedupe
// to the typed 409 (which omits existing_id — the aborted transaction
// cannot re-query) and two primary emails of one type to a plain conflict.
func insertPersonEmails(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, personID ids.PersonID, source, by string, emails []PersonEmailInput) error {
	for _, e := range emails {
		if _, err := tx.Exec(ctx,
			`INSERT INTO person_email (person_id, email, email_type, is_primary, position, source, captured_by, from_correspondence)
			 VALUES ($1, lower($2), $3, $4, $5, $6, $7, $8)`,
			personID, e.Email, e.EmailType, e.IsPrimary, e.Position, source, by,
			!e.VouchedNotCorresponded); err != nil {
			if name, ok := storekit.UniqueViolation(err); ok {
				if name == "uq_person_email_dedupe" {
					return &DuplicateEmailError{Email: e.Email}
				}
				return apperrors.ErrConflict
			}
			return fmt.Errorf("insert person email: %w", err)
		}
	}
	return nil
}

// ensurePersonEmailsUnclaimed is the dedupe pre-check, so the 409 can
// carry the existing id; the unique index remains the structural
// guarantee under races. The existing id is disclosed only when the
// caller could read that row; the conflict itself is still answered
// (existence-hiding survives the 409).
func ensurePersonEmailsUnclaimed(ctx context.Context, tx pgx.Tx, emails []PersonEmailInput) error {
	return ensurePersonEmailsUnclaimedExcept(ctx, tx, ids.PersonID{}, emails)
}

// ensurePersonEmailsUnclaimedExcept is the same probe, ignoring one person's
// own rows. An update that re-states an address the person already holds must
// not be refused by that person's own row, which is the only difference between
// the create case (nothing to exclude) and the replace case.
func ensurePersonEmailsUnclaimedExcept(ctx context.Context, tx pgx.Tx, self ids.PersonID, emails []PersonEmailInput) error {
	for _, e := range emails {
		var existing ids.PersonID
		err := tx.QueryRow(ctx,
			`SELECT person_id FROM person_email
			  WHERE email = lower($1) AND archived_at IS NULL AND person_id <> $2`,
			e.Email, self.UUID).Scan(&existing)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("probe email dedupe: %w", err)
		}
		dup := &DuplicateEmailError{Email: e.Email}
		visible, err := auth.VisibleTo(ctx, tx, "person", existing.UUID)
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

const personColumns = `id, full_name, first_name, last_name, title, owner_id, visibility,
	address_line1, address_line2, address_city, address_region, address_postal_code, address_country,
	merged_into_id, converted_from_lead_id, source, captured_by,
	version, created_at, updated_at, archived_at, last_activity_at`

// readPerson resolves one person row; active names the custom-field
// columns to carry alongside the core ones — nil for internal decision
// reads whose result never reaches the wire.
func readPerson(ctx context.Context, tx pgx.Tx, id ids.PersonID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Person, error) {
	q := `SELECT ` + personColumns + storekit.SelectSuffix(active) + ` FROM person WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += ` AND archived_at IS NULL`
	}
	row := tx.QueryRow(ctx, q, id)
	p, err := scanPerson(row, active)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Person{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Person{}, err
	}

	people := []crmcontracts.Person{p}
	if err := attachPersonChildren(ctx, tx, people); err != nil {
		return crmcontracts.Person{}, err
	}
	return people[0], nil
}

// scanPerson scans core + active custom columns; extra receives any
// trailing expressions the caller's SELECT appended (the sorted list's
// cursor key).
func scanPerson(row pgx.Row, active []fieldcatalog.Column, extra ...any) (crmcontracts.Person, error) {
	var p crmcontracts.Person
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
	if v := crmcontracts.PersonVisibility(visibility); v != "" {
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

// attachPersonChildren loads emails + phones + social for a page in
// three queries, not 3N.
func attachPersonChildren(ctx context.Context, tx pgx.Tx, people []crmcontracts.Person) error {
	if len(people) == 0 {
		return nil
	}
	// Whether each row is this caller's to change, one statement for the page.
	// It is stamped HERE because this is the seam the list and the single read
	// already share: a client is otherwise left inferring write access from the
	// object grant alone, which says nothing about who owns the row.
	if _, err := auth.StampWritable(ctx, tx, "person", people,
		func(p crmcontracts.Person) ids.UUID { return ids.UUID(p.Id) },
		func(p *crmcontracts.Person, may bool) { p.Writable = &may }); err != nil {
		return err
	}
	idx := make(map[openapi_types.UUID]*crmcontracts.Person, len(people))
	personIDs := make([]ids.UUID, len(people))
	for i := range people {
		idx[people[i].Id] = &people[i]
		personIDs[i] = ids.UUID(people[i].Id)
	}
	if err := attachPersonEmails(ctx, tx, idx, personIDs); err != nil {
		return err
	}
	if err := attachPersonPhones(ctx, tx, idx, personIDs); err != nil {
		return err
	}
	if err := attachPersonSocial(ctx, tx, idx, personIDs); err != nil {
		return err
	}
	if err := attachPersonEmployers(ctx, tx, idx, personIDs); err != nil {
		return err
	}
	if err := storekit.AttachRowTags(ctx, tx, personEntity, people,
		func(p crmcontracts.Person) ids.UUID { return ids.UUID(p.Id) },
		func(p *crmcontracts.Person, tags []storekit.RowTag) { p.Tags = wireRowTags(tags) }); err != nil {
		return err
	}
	return attachPersonReachability(ctx, tx, idx, personIDs)
}

func attachPersonEmails(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Person, personIDs []ids.UUID) error {
	rows, err := tx.Query(ctx,
		`SELECT person_id, id, email, email_type, is_primary, position, source, captured_by
		 FROM person_email WHERE person_id = ANY($1) AND archived_at IS NULL
		 ORDER BY position, created_at`, personIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var personID, emailID ids.UUID
		var e crmcontracts.PersonEmail
		var email string
		if err := rows.Scan(&personID, &emailID, &email, &e.EmailType, &e.IsPrimary, &e.Position, &e.Source, &e.CapturedBy); err != nil {
			return err
		}
		e.Id = openapi_types.UUID(emailID)
		e.Email = openapi_types.Email(email)
		p := idx[openapi_types.UUID(personID)]
		if p.Emails == nil {
			p.Emails = &[]crmcontracts.PersonEmail{}
		}
		*p.Emails = append(*p.Emails, e)
	}
	return rows.Err()
}

func attachPersonPhones(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Person, personIDs []ids.UUID) error {
	phoneRows, err := tx.Query(ctx,
		`SELECT person_id, id, phone, phone_type, is_primary, position, source, captured_by
		 FROM person_phone WHERE person_id = ANY($1) AND archived_at IS NULL
		 ORDER BY position, created_at`, personIDs)
	if err != nil {
		return err
	}
	defer phoneRows.Close()
	for phoneRows.Next() {
		var personID, phoneID ids.UUID
		var ph crmcontracts.PersonPhone
		if err := phoneRows.Scan(&personID, &phoneID, &ph.Phone, &ph.PhoneType, &ph.IsPrimary, &ph.Position, &ph.Source, &ph.CapturedBy); err != nil {
			return err
		}
		ph.Id = openapi_types.UUID(phoneID)
		p := idx[openapi_types.UUID(personID)]
		if p.Phones == nil {
			p.Phones = &[]crmcontracts.PersonPhone{}
		}
		*p.Phones = append(*p.Phones, ph)
	}
	return phoneRows.Err()
}

func attachPersonSocial(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Person, personIDs []ids.UUID) error {
	// The wire keeps social as the (platform → handle) map; the relation
	// is the stored form.
	socialRows, err := tx.Query(ctx,
		`SELECT person_id, platform, handle FROM person_social WHERE person_id = ANY($1)
		 ORDER BY platform`, personIDs)
	if err != nil {
		return err
	}
	defer socialRows.Close()
	for socialRows.Next() {
		var personID ids.UUID
		var platform, handle string
		if err := socialRows.Scan(&personID, &platform, &handle); err != nil {
			return err
		}
		p := idx[openapi_types.UUID(personID)]
		if p.Social == nil {
			p.Social = &map[string]any{}
		}
		(*p.Social)[platform] = handle
	}
	return socialRows.Err()
}

// attachPersonReachability loads the reachability projection (design §6.6):
// {provider, reachable, since}, never the raw channel_user_id — an opaque
// third-party account identifier belongs in a governed read, not this broad
// one. A blocked identity is still a live row (archived_at IS NULL), so it
// still appears here, with reachable=false — the record must keep showing
// that a conversation exists even when a reply cannot currently be delivered.
func attachPersonReachability(ctx context.Context, tx pgx.Tx, idx map[openapi_types.UUID]*crmcontracts.Person, personIDs []ids.UUID) error {
	rows, err := tx.Query(ctx,
		`SELECT person_id, provider, blocked_at, created_at
		 FROM person_channel_identity WHERE person_id = ANY($1) AND archived_at IS NULL
		 ORDER BY provider, created_at`, personIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var personID ids.UUID
		var provider string
		var blockedAt, createdAt time.Time
		var blockedAtPtr *time.Time
		if err := rows.Scan(&personID, &provider, &blockedAtPtr, &createdAt); err != nil {
			return err
		}
		since := createdAt
		if blockedAtPtr != nil {
			blockedAt = *blockedAtPtr
			since = blockedAt
		}
		r := crmcontracts.PersonReachability{
			Provider:  provider,
			Reachable: blockedAtPtr == nil,
			Since:     since,
		}
		p := idx[openapi_types.UUID(personID)]
		if p.Reachability == nil {
			p.Reachability = &[]crmcontracts.PersonReachability{}
		}
		*p.Reachability = append(*p.Reachability, r)
	}
	return rows.Err()
}
