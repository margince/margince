// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The check of a stored country against the country its postal code and city
// resolve to. The check only flags a mismatch; it never changes the stored
// country. A trigger clears the result when the postal code, city or country
// change. So the flag never speaks for an address the row no longer holds.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CountryCheckedRecord names a table whose address country is checked. Its value
// is both the RBAC object and the table name.
type CountryCheckedRecord string

// The records whose address country is checked.
const (
	CountryCheckedCompany CountryCheckedRecord = "company"
	CountryCheckedContact CountryCheckedRecord = "contact"
)

// CountryCheckedRecords lists the records the sweep visits, in the order it visits them.
var CountryCheckedRecords = []CountryCheckedRecord{CountryCheckedCompany, CountryCheckedContact}

// AddressCountryCheck is one record whose stored country is due a check. The
// three address values are the ones read, and the write lands only while the
// row still holds them.
type AddressCountryCheck struct {
	Record     CountryCheckedRecord
	ID         ids.UUID
	PostalCode string
	City       string
	Country    string
}

// Query is what the geocoder is asked: postal code and city only, so the stored
// country cannot steer the answer.
func (c AddressCountryCheck) Query() string {
	return c.PostalCode + " " + c.City
}

// addressCountryMismatch is the one rule for when a stored country contradicts
// its postal code and city. A stored value that is no assigned code, from a row
// written before the country rule, has nothing to compare and is not flagged.
func addressCountryMismatch(stored, resolved *string) *crmcontracts.AddressCountryMismatch {
	if stored == nil || resolved == nil {
		return nil
	}
	code, err := NormalizeCountryCode(*stored)
	if err != nil || code == *resolved {
		return nil
	}
	return &crmcontracts.AddressCountryMismatch{ResolvedCountry: *resolved}
}

// ListAddressCountryChecksDue answers up to limit live records of one kind that
// are due a check. Each holds a postal code, a city and a two-letter country,
// unchecked since they last changed. A row with no stored country has nothing
// to contradict, so it costs no lookup.
func (s *Store) ListAddressCountryChecksDue(
	ctx context.Context, record CountryCheckedRecord, limit int,
) ([]AddressCountryCheck, error) {
	if err := auth.Require(ctx, string(record), principal.ActionRead); err != nil {
		return nil, err
	}
	table, err := countryCheckedTable(record)
	if err != nil {
		return nil, err
	}
	var due []AddressCountryCheck
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, address_postal_code, address_city, address_country
			  FROM `+table+`
			 WHERE archived_at IS NULL
			   AND address_country_checked_at IS NULL
			   AND btrim(coalesce(address_postal_code, '')) <> ''
			   AND btrim(coalesce(address_city, '')) <> ''
			   AND address_country ~ '^[A-Za-z]{2}$'
			 ORDER BY created_at, id
			 LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			check := AddressCountryCheck{Record: record}
			if err := rows.Scan(&check.ID, &check.PostalCode, &check.City, &check.Country); err != nil {
				return err
			}
			due = append(due, check)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("listing %s addresses due a country check: %w", record, err)
	}
	return due, nil
}

// RecordAddressCountry stores what the postal code and city resolved to, or that
// they resolved to no place when found is false. It writes nothing when an edit
// changed them or the country after the check read them. That edit's trigger
// already cleared the row, and the next sweep asks again.
func (s *Store) RecordAddressCountry(
	ctx context.Context, check AddressCountryCheck, resolvedCode string, found bool,
) (bool, error) {
	if err := auth.Require(ctx, string(check.Record), principal.ActionUpdate); err != nil {
		return false, err
	}
	table, err := countryCheckedTable(check.Record)
	if err != nil {
		return false, err
	}
	var resolved *string
	if code, err := NormalizeCountryCode(resolvedCode); found && err == nil {
		resolved = &code
	}
	var written bool
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, string(check.Record), check.ID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE `+table+`
			   SET address_country_resolved = $5, address_country_checked_at = $6
			 WHERE id = $1 AND archived_at IS NULL
			   AND address_postal_code = $2 AND address_city = $3 AND address_country = $4`,
			check.ID, check.PostalCode, check.City, check.Country, resolved, s.clock())
		written = tag.RowsAffected() == 1
		return err
	})
	if err != nil {
		return false, fmt.Errorf("recording the country of a %s address: %w", check.Record, err)
	}
	return written, nil
}

// countryCheckedTable maps the closed record set onto its table, so no caller
// can format another name into the statement.
func countryCheckedTable(record CountryCheckedRecord) (string, error) {
	switch record {
	case CountryCheckedCompany:
		return "company", nil
	case CountryCheckedContact:
		return "contact", nil
	}
	return "", fmt.Errorf("contacts: %q carries no checked address country", record)
}
