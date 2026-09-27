// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Applying the phone numbers one dated statement lists.
//
// A phone is a list, so a signature or a card states a SET of numbers and the
// set is applied as one. A number that a newer statement leaves out is kept:
// signatures get trimmed for length, and deleting a working number costs a
// contact the product can no longer reach, where a stale one costs a wasted
// call. A newer statement replaces a number only by stating a different number
// of the same country and type — the German number that changed replaces the
// old German number, and the Singapore number nobody mentioned stays.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// observedNumber is one number a statement lists, with the text it was read
// from. Confidence is nil for a card, which is parsed rather than inferred.
type observedNumber struct {
	Phone      string
	PhoneType  string
	Evidence   string
	Confidence *float64
}

// observedNumbers is one dated statement's numbers. A zero ObservedAt dates it
// by the transaction's clock, which is the honest date for a card without one.
type observedNumbers struct {
	Numbers    []observedNumber
	SourceRef  string
	Source     string
	CapturedBy string
	ObservedAt time.Time
}

// listedNumber is one parsed number and what applying it did.
type listedNumber struct {
	observedNumber
	phone    values.Phone
	outcome  observedOutcome
	replaced string // the E.164 number this one retired, when it retired one
}

// applyObservedNumbers writes every number one statement lists and returns the
// ones that changed the number list, in E.164.
//
// Confirmations run before additions. A number the statement repeats is moved
// to the statement's date first, so when a new number then looks for the older
// number of its country to replace, the repeated ones are no longer older and
// only a number this statement left out can be chosen. In listing order, a
// changed mobile listed first would retire the unchanged desk number after it.
//
// Every number that landed gets its own evidence row, and a replacing number's
// row takes over the replaced number's row, so the undo buffer names the one
// number it is about.
func applyObservedNumbers(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, s observedNumbers) ([]string, error) {
	listed := parseListedNumbers(s.Numbers)
	if len(listed) == 0 {
		return nil, nil
	}
	// The subject before any row this transaction writes: Art. 17 erasure
	// takes it first and then deletes what hangs off it.
	if err := auth.HoldWritableLive(ctx, tx, "contact", contactID.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	observedAt, err := statementDate(ctx, tx, s.ObservedAt)
	if err != nil {
		return nil, err
	}
	for i := range listed {
		if listed[i].outcome, err = confirmKnownNumber(ctx, tx, contactID, listed[i].phone.String(), observedAt); err != nil {
			return nil, err
		}
	}
	for i := range listed {
		if listed[i].outcome != observedSkipped {
			continue
		}
		if err := applyObservedPhone(ctx, tx, contactID, &listed[i], s, observedAt); err != nil {
			return nil, err
		}
	}
	var landed []string
	for _, n := range listed {
		if n.outcome != observedApplied && n.outcome != observedReplaced {
			continue
		}
		if _, err := writeContactProfileField(ctx, tx, contactID, contactProfileFieldRow{
			Field: fieldPhone, Value: n.phone.String(), EvidenceSnippet: n.Evidence,
			SourceRef: s.SourceRef, Source: s.Source, CapturedBy: s.CapturedBy,
			Confidence: n.Confidence, ObservedAt: &observedAt, Replaces: n.replaced,
		}, supersedeOnNewerObservation); err != nil {
			return nil, err
		}
		landed = append(landed, n.phone.String())
	}
	return landed, nil
}

// parseListedNumbers keeps the numbers this reader can parse, once each.
//
// An unparseable number is declined, not failed: abandoning the other numbers
// of the same signature over one footer's formatting would lose what could be
// read. A number listed twice is one number, and the first listing's type and
// evidence stand.
func parseListedNumbers(numbers []observedNumber) []listedNumber {
	out := make([]listedNumber, 0, len(numbers))
	seen := map[string]bool{}
	for _, n := range numbers {
		parsed, err := values.ParsePhone(n.Phone)
		if err != nil || seen[parsed.String()] {
			continue
		}
		seen[parsed.String()] = true
		if n.PhoneType == "" {
			n.PhoneType = emailTypeWork
		}
		out = append(out, listedNumber{observedNumber: n, phone: parsed})
	}
	return out
}

// statementDate is the statement's own date, or the transaction's clock when
// it states none. Read from the database rather than the process so it
// compares against stored dates on the clock that wrote them, and so every
// number of one statement carries ONE date.
func statementDate(ctx context.Context, tx pgx.Tx, stated time.Time) (time.Time, error) {
	if !stated.IsZero() {
		return stated, nil
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return time.Time{}, fmt.Errorf("contacts: dating an undated statement: %w", err)
	}
	return now, nil
}

// applyObservedPhone adds one number the record does not carry, or replaces
// the older number of its country and type.
//
// Additive across types: a mobile says nothing about the desk number. Additive
// across countries: a Singapore number says nothing about the German one.
// Within one country and type it is recency, because two work numbers of one
// country, one older than the other, is what a changed number looks like.
func applyObservedPhone(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, n *listedNumber, s observedNumbers, observedAt time.Time) error {
	country := "+" + n.phone.CountryCode()
	// A number of this country and type stated LATER already stands, so this
	// statement is stale: a re-delivered old mail would otherwise file its
	// number beside the current one with no way to tell which rings. STRICTLY
	// later, because the numbers of one statement share its date and a tie is
	// a contact who gave two numbers together.
	var newerStands bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM contact_phone
			WHERE contact_id = $1 AND phone_type = $2 AND archived_at IS NULL
			  AND starts_with(phone, $3) AND observed_at > $4)`,
		contactID, n.PhoneType, country, observedAt).Scan(&newerStands); err != nil {
		return fmt.Errorf("contacts: looking for a number stated later: %w", err)
	}
	if newerStands {
		return nil
	}

	supersededID, supersededPhone, err := numberThisReplaces(ctx, tx, contactID, n.PhoneType, country, observedAt)
	if err != nil {
		return err
	}
	if supersededID != nil {
		if _, err := tx.Exec(ctx, `
			UPDATE contact_phone SET archived_at = now() WHERE id = $1 AND archived_at IS NULL`,
			supersededID); err != nil {
			return fmt.Errorf("contacts: retiring the replaced number: %w", err)
		}
	}

	// is_primary only when this type has no live primary left: the partial
	// unique index permits exactly one, and claiming it from a number this
	// statement said nothing about would silently re-rank the record.
	tag, err := tx.Exec(ctx, `
		INSERT INTO contact_phone
		  (contact_id, phone, phone_type, is_primary, position, source, captured_by, observed_at, superseded_phone_id)
		SELECT $1, $2, $3,
		  NOT EXISTS (
			SELECT 1 FROM contact_phone
			WHERE contact_id = $1 AND phone_type = $3 AND is_primary AND archived_at IS NULL),
		  COALESCE((SELECT MAX(position) + 1 FROM contact_phone
			WHERE contact_id = $1 AND archived_at IS NULL), 0),
		  $4, $5, $6, $7
		WHERE EXISTS (SELECT 1 FROM contact WHERE id = $1 AND archived_at IS NULL)`,
		contactID, n.phone.String(), n.PhoneType, s.Source, s.CapturedBy, observedAt, supersededID)
	if err != nil {
		return fmt.Errorf("contacts: writing the observed number: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	n.outcome = observedApplied
	if supersededID != nil {
		n.outcome, n.replaced = observedReplaced, supersededPhone
	}
	return nil
}

// confirmKnownNumber handles a number the record already carries: it advances
// the date rather than filing a duplicate, and reports observedSkipped when
// this is a number the caller still has to write.
//
// Advancing on an identical value is what makes the row say "still true as of
// this date". Without it a number confirmed a dozen times keeps the date of its
// first sighting, and a late-delivered OLDER mail would then outrank it.
func confirmKnownNumber(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, normalized string, observedAt time.Time) (observedOutcome, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE contact_phone SET observed_at = $3
		WHERE contact_id = $1 AND phone = $2 AND archived_at IS NULL AND observed_at < $3`,
		contactID, normalized, observedAt)
	if err != nil {
		return observedSkipped, fmt.Errorf("contacts: confirming a known number: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return observedApplied, nil
	}
	// The number is here but was already dated at or after this statement, so
	// there is nothing to advance and nothing to add.
	var live bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM contact_phone
			WHERE contact_id = $1 AND phone = $2 AND archived_at IS NULL)`,
		contactID, normalized).Scan(&live); err != nil {
		return observedSkipped, fmt.Errorf("contacts: looking for a known number: %w", err)
	}
	if live {
		return observedConfirmed, nil
	}
	return observedSkipped, nil
}

// numberThisReplaces finds the older live number of this country and type, or
// nil where there is none.
//
// Chosen by id so the archive and the insert name the same row: several live
// numbers of one type and country are permitted, and the primary is the one
// displayed.
func numberThisReplaces(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, phoneType, country string, observedAt time.Time) (*ids.UUID, string, error) {
	var supersededID *ids.UUID
	var phone string
	if err := tx.QueryRow(ctx, `
		SELECT id, phone FROM contact_phone
		WHERE contact_id = $1 AND phone_type = $2 AND archived_at IS NULL
		  AND starts_with(phone, $3) AND observed_at < $4
		ORDER BY is_primary DESC, position, created_at
		LIMIT 1`,
		contactID, phoneType, country, observedAt).Scan(&supersededID, &phone); err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return nil, "", fmt.Errorf("contacts: looking for the number this replaces: %w", err)
	}
	return supersededID, phone, nil
}
