// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The check of a stored country against the countries its postal code and city
// resolve to. The check only flags a mismatch; it never changes the stored
// country. Results live in company_address_country and contact_address_country,
// so writing one never moves the record's version. A trigger deletes the result
// when the postal code, city or country change.

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

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

// AddressCountryRefreshAge is how old a check or a cached country answer may
// grow before it is asked again. Postal codes are reassigned rarely, so half a
// year keeps the evidence current at little cost to the shared rate.
const AddressCountryRefreshAge = 180 * 24 * time.Hour

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
// its postal code and city: it is none of the countries they can name. A stored
// value that is no assigned code has nothing to compare and is not flagged.
func addressCountryMismatch(stored *string, resolved []string) *crmcontracts.AddressCountryMismatch {
	if stored == nil || len(resolved) == 0 {
		return nil
	}
	code, err := NormalizeCountryCode(*stored)
	if err != nil || slices.Contains(resolved, code) {
		return nil
	}
	return &crmcontracts.AddressCountryMismatch{ResolvedCountry: resolved[0]}
}

// addressCountriesSQL reads a record's resolved countries beside its columns.
// table is one of the two literal record tables.
func addressCountriesSQL(table string) string {
	return `(SELECT k.countries FROM ` + table + `_address_country k WHERE k.` + table + `_id = ` +
		table + `.id) AS address_countries_resolved`
}

// assignedCountryCodes is assignedCountry as a sorted list, so the due read's
// SQL selects the rows NormalizeCountryCode accepts and no others.
var assignedCountryCodes = slices.Sorted(maps.Keys(assignedCountry))

// ListAddressCountryChecksDue answers up to limit live records of one kind that
// are due a check, never-checked ones first. Each holds a postal code, a city
// and an assigned country code. It is due when unchecked since those changed,
// or checked longer than AddressCountryRefreshAge ago.
func (s *Store) ListAddressCountryChecksDue(
	ctx context.Context, record CountryCheckedRecord, limit int,
) ([]AddressCountryCheck, error) {
	if err := requireCountryCheckRead(ctx, record); err != nil {
		return nil, err
	}
	table, err := countryCheckedTable(record)
	if err != nil {
		return nil, err
	}
	var due []AddressCountryCheck
	err = s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT r.id, r.address_postal_code, r.address_city, r.address_country
			  FROM `+table+` r
			  LEFT JOIN `+table+`_address_country k ON k.`+table+`_id = r.id
			 WHERE r.archived_at IS NULL
			   AND (k.checked_at IS NULL OR k.checked_at < $2)
			   AND btrim(coalesce(r.address_postal_code, '')) <> ''
			   AND btrim(coalesce(r.address_city, '')) <> ''
			   AND upper(btrim(r.address_country)) = ANY($3)
			 ORDER BY k.checked_at NULLS FIRST, r.created_at, r.id
			 LIMIT $1`, limit, s.clock().Add(-AddressCountryRefreshAge), assignedCountryCodes)
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

// RecordAddressCountries stores the countries the postal code and city can name;
// an empty list records that they named no place. It writes nothing when an edit
// changed them or the country after the check read them. That edit's trigger
// already cleared the row, and the next sweep asks again.
func (s *Store) RecordAddressCountries(
	ctx context.Context, check AddressCountryCheck, resolved []string,
) (bool, error) {
	if err := requireCountryCheckWrite(ctx, check.Record); err != nil {
		return false, err
	}
	table, err := countryCheckedTable(check.Record)
	if err != nil {
		return false, err
	}
	codes := assignedCodesOf(resolved)
	var written bool
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, string(check.Record), check.ID); err != nil {
			return err
		}
		// FOR SHARE holds an edit back until this commits, and the edit's trigger
		// then deletes the row. An edit that committed first leaves no row to lock.
		tag, err := tx.Exec(ctx, `
			INSERT INTO `+table+`_address_country (`+table+`_id, countries, checked_at)
			SELECT id, $5, $6 FROM `+table+`
			 WHERE id = $1 AND archived_at IS NULL
			   AND address_postal_code = $2 AND address_city = $3 AND address_country = $4
			   FOR SHARE
			ON CONFLICT (`+table+`_id) DO UPDATE
			   SET countries = EXCLUDED.countries, checked_at = EXCLUDED.checked_at`,
			check.ID, check.PostalCode, check.City, check.Country, codes, s.clock())
		written = tag.RowsAffected() == 1
		return err
	})
	if err != nil {
		return false, fmt.Errorf("recording the countries of a %s address: %w", check.Record, err)
	}
	return written, nil
}

// LookupPlaceCountries answers the remembered countries of a place query, when
// they were asked within AddressCountryRefreshAge. Keyed like the place cache.
func (s *Store) LookupPlaceCountries(ctx context.Context, query string) ([]string, bool, error) {
	key := normalizePlaceQuery(query)
	if key == "" {
		return nil, false, nil
	}
	var codes []string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT country_codes FROM geocode_country_cache WHERE query = $1 AND resolved_at >= $2`,
			key, s.clock().Add(-AddressCountryRefreshAge)).Scan(&codes)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("reading the place country cache: %w", err)
	}
	return codes, true, nil
}

// RememberPlaceCountries records one answer whole: its countries, provider and
// time replace whatever an older answer said.
func (s *Store) RememberPlaceCountries(ctx context.Context, query string, codes []string, provider string) error {
	key := normalizePlaceQuery(query)
	if key == "" {
		return nil
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO geocode_country_cache (query, country_codes, provider, resolved_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (query) DO UPDATE
			   SET country_codes = EXCLUDED.country_codes, provider = EXCLUDED.provider,
			       resolved_at = EXCLUDED.resolved_at`,
			key, assignedCodesOf(codes), provider, s.clock())
		return err
	})
}

// assignedCodesOf keeps the assigned codes of a provider answer, upper-case and
// in order. Never nil, because an empty set is an answer.
func assignedCodesOf(codes []string) []string {
	out := []string{}
	for _, raw := range codes {
		if code, err := NormalizeCountryCode(raw); err == nil && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}

// countryCheckedTable maps the closed record set onto its table, so no caller
// can format another name into the statement.
func countryCheckedTable(record CountryCheckedRecord) (string, error) {
	switch record {
	case CountryCheckedCompany, CountryCheckedContact:
		return string(record), nil
	}
	return "", fmt.Errorf("contacts: %q carries no checked address country", record)
}

// requireCountryCheckRead and requireCountryCheckWrite name each object and
// verb as a literal, so the grant scan can resolve every door a check uses.
func requireCountryCheckRead(ctx context.Context, record CountryCheckedRecord) error {
	switch record {
	case CountryCheckedCompany:
		return auth.Require(ctx, "company", principal.ActionRead)
	case CountryCheckedContact:
		return auth.Require(ctx, "contact", principal.ActionRead)
	}
	return fmt.Errorf("contacts: %q carries no checked address country", record)
}

func requireCountryCheckWrite(ctx context.Context, record CountryCheckedRecord) error {
	switch record {
	case CountryCheckedCompany:
		return auth.Require(ctx, "company", principal.ActionUpdate)
	case CountryCheckedContact:
		return auth.Require(ctx, "contact", principal.ActionUpdate)
	}
	return fmt.Errorf("contacts: %q carries no checked address country", record)
}
