// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// One fill per record field a purchase can reach.
//
// Each answers whether it landed, and none of them fails the hand-off over a
// value the record cannot take. A bought address that already belongs to
// another contact, a profile URL the normalizer cannot read, a title on a
// record that already has one — all of these are "nothing to do", because an
// error here rolls back the claim write beside it and five retries later the
// purchase is gone: paid for, and nowhere.
//
// Only a fault the caller must know about — a broken statement, a refused
// subject — comes back as an error.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The record fields and tables a purchase can fill. The revert reads
// provider_applied_field rows back and matches on these exact names, so both
// sides take them from here rather than repeating the literals.
const (
	fieldTitle      = "title"
	fieldEmployment = "employment"

	tableRelationship = "relationship"
	tablePersonSocial = "person_social"
	tablePersonEmail  = "person_email"
	tablePersonPhone  = "person_phone"
)

// filled reports one fill's outcome. The bool is what the caller acts on:
// false means the record already held something, which is the fill-only rule
// working rather than anything to report.
type filled struct {
	field appliedField
	ok    bool
}

// nothingFilled is the "the record already had one" answer.
var nothingFilled = filled{}

// fillTitle sets the person's title when they have none.
//
// archived_at is in the predicate for the reason every fill-only write in this
// tree carries it: an erasure committing between the subject hold and this
// statement would otherwise write a bought title back onto a record it had
// just cleared.
func fillTitle(ctx context.Context, tx pgx.Tx, subject ids.UUID, _ string, v applicableClaims) (filled, error) {
	if v.title == "" {
		return nothingFilled, nil
	}
	if err := auth.EnsureWritableLive(ctx, tx, entityPerson, subject); err != nil {
		return nothingFilled, err
	}
	// RowsAffected is the compare-and-set: the predicate decides, and zero
	// rows means a title was already there.
	tag, err := tx.Exec(ctx, `
		UPDATE person SET title = $2
		 WHERE id = $1 AND coalesce(title, '') = '' AND archived_at IS NULL`,
		subject, v.title)
	if err != nil {
		return nothingFilled, fmt.Errorf("people: filling a bought job title: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nothingFilled, nil
	}
	// The value was trimmed before it got here, so what the column holds is
	// what this says — which is what lets the revert compare like with like.
	stored := v.title
	return filled{appliedField{table: entityPerson, field: fieldTitle, value: &stored}, true}, nil
}

// fillLinkedIn puts the profile URL on the record when no handle is there.
//
// Normalized first: the same profile arrives from a provider, a paste and an
// import in three spellings, and a record holding one of them while a revert
// compares another would keep a value it believes it removed.
func fillLinkedIn(ctx context.Context, tx pgx.Tx, subject ids.UUID, _ string, v applicableClaims) (filled, error) {
	if v.linkedInURL == "" {
		return nothingFilled, nil
	}
	// The HOLD, not just the probe: person_social is a table Art. 17 erasure
	// DELETEs, so a snapshot probe leaves a window in which the erasure commits
	// and this write puts the handle back.
	if err := auth.HoldWritableLive(ctx, tx, entityPerson, subject); err != nil {
		return nothingFilled, err
	}
	handle, err := NormalizeLinkedInURL(v.linkedInURL)
	if err != nil {
		// A provider that answered an unreadable URL has told us nothing about
		// this contact's profile: an empty answer, not a fault to fail on.
		return nothingFilled, nil //nolint:nilerr // an unreadable answer is an empty answer
	}
	landed, err := insertSocialHandle(ctx, tx, subject, socialLinkedIn, handle)
	if err != nil || !landed {
		return nothingFilled, err
	}
	if err := touchPerson(ctx, tx, subject); err != nil {
		return nothingFilled, err
	}
	return filled{appliedField{table: tablePersonSocial, field: socialLinkedIn, value: &handle}, true}, nil
}

// fillEmployment links the contact to the company the provider names, when
// that company is already in the CRM and the contact holds no current job.
//
// It never CREATES a company. A data provider naming an employer is not a
// reason to mint a record nobody asked for, and a roster full of companies the
// installation has no relationship with is worse than an unlinked contact.
func fillEmployment(ctx context.Context, tx pgx.Tx, subject ids.UUID, providerName string, v applicableClaims) (filled, error) {
	if v.companyDomain == "" {
		return nothingFilled, nil
	}
	org, found, err := organizationByDomain(ctx, tx, v.companyDomain)
	if err != nil || !found {
		return nothingFilled, err
	}
	edge, planted, err := plantProviderEmploymentEdge(ctx, tx, subject, org, providerName)
	if err != nil || !planted {
		return nothingFilled, err
	}
	return filled{appliedField{table: tableRelationship, field: fieldEmployment, rowID: &edge}, true}, nil
}

// fillEmail puts a bought address on a contact who has none.
//
// A duplicate is a SKIP. person_email is unique on the address across every
// contact, so an address the provider found that already belongs to somebody
// else cannot land — and must not take the rest of the purchase down with it.
func fillEmail(ctx context.Context, tx pgx.Tx, subject ids.UUID, providerName string, v applicableClaims) (filled, error) {
	if v.email == "" {
		return nothingFilled, nil
	}
	address, err := values.ParseEmail(v.email)
	if err != nil {
		return nothingFilled, nil //nolint:nilerr // an address we cannot parse is one we cannot store
	}
	rowID, landed, err := insertChildRow(ctx, tx, `
		INSERT INTO person_email (person_id, email, email_type, is_primary, position, source, captured_by)
		SELECT $1, $2, 'work', true, 0, $3, $4
		 WHERE NOT EXISTS (
		       SELECT 1 FROM person_email WHERE person_id = $1 AND archived_at IS NULL)
		ON CONFLICT DO NOTHING
		RETURNING id`, subject, address.String(), providerName)
	if err != nil {
		return nothingFilled, fmt.Errorf("people: filling a bought address: %w", err)
	}
	if !landed {
		return nothingFilled, nil
	}
	return filled{appliedField{table: tablePersonEmail, field: fieldEmail, rowID: &rowID}, true}, nil
}

// fillPhone puts a bought number on a contact who has none. Same duplicate
// posture as the address beside it.
func fillPhone(ctx context.Context, tx pgx.Tx, subject ids.UUID, providerName string, v applicableClaims) (filled, error) {
	if v.phone == "" {
		return nothingFilled, nil
	}
	number, err := values.ParsePhone(v.phone)
	if err != nil {
		return nothingFilled, nil //nolint:nilerr // a number we cannot parse is one we cannot store
	}
	rowID, landed, err := insertChildRow(ctx, tx, `
		INSERT INTO person_phone (person_id, phone, phone_type, is_primary, position, source, captured_by)
		SELECT $1, $2, 'mobile', true, 0, $3, $4
		 WHERE NOT EXISTS (
		       SELECT 1 FROM person_phone WHERE person_id = $1 AND archived_at IS NULL)
		ON CONFLICT DO NOTHING
		RETURNING id`, subject, number.String(), providerName)
	if err != nil {
		return nothingFilled, fmt.Errorf("people: filling a bought phone number: %w", err)
	}
	if !landed {
		return nothingFilled, nil
	}
	return filled{appliedField{table: tablePersonPhone, field: fieldPhone, rowID: &rowID}, true}, nil
}

// insertChildRow runs one guarded child insert and says whether it landed.
// Shared by the address and the number, whose statements differ only in the
// table they write and the type they stamp.
func insertChildRow(ctx context.Context, tx pgx.Tx, statement string, subject ids.UUID, value, providerName string) (ids.UUID, bool, error) {
	var rowID ids.UUID
	err := tx.QueryRow(ctx, statement, subject, value, providerName, connectorCapturedBy(providerName)).Scan(&rowID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, false, nil
	}
	if err != nil {
		return ids.UUID{}, false, err
	}
	return rowID, true, nil
}
