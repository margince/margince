// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The contract Address and the six columns behind it, in both directions.
// Contact and company store the same six and share this seam: two
// spellings of "is this address empty" would disagree the first time one
// of them gained a field.

import (
	"strings"

	"golang.org/x/text/language"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// InvalidCountryError maps to 422. The contract declares an address country as
// an ISO 3166-1 alpha-2 code. A name, an alpha-3 code or an unassigned pair
// would never group or filter with the same country spelled properly.
type InvalidCountryError struct{}

func (e *InvalidCountryError) Error() string {
	return "address.country must be a two-letter ISO 3166-1 country code such as DE"
}

// FieldFault names the nested field the caller corrects.
func (e *InvalidCountryError) FieldFault() (field, code, message string) {
	return "address.country", "invalid_country_code", e.Error()
}

// checkedAddress is the one rule for an address a caller typed. Create and edit
// hold it on both contacts and companies. A stated country is an assigned
// alpha-2 code, stored upper-case so "de" and "DE" are one country. The input
// is not changed, and a nil address or one with no country passes.
func checkedAddress(a *crmcontracts.Address) (*crmcontracts.Address, error) {
	if a == nil || a.Country == nil {
		return a, nil
	}
	code, err := NormalizeCountryCode(*a.Country)
	if err != nil {
		return nil, err
	}
	out := *a
	out.Country = &code
	return &out, nil
}

// NormalizeCountryCode is the country rule on its own, for a caller that checks
// a cell before it builds an address: an assigned alpha-2 code, upper-case.
func NormalizeCountryCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if len(code) != 2 {
		return "", &InvalidCountryError{}
	}
	region, err := language.ParseRegion(code)
	if err != nil || !region.IsCountry() {
		return "", &InvalidCountryError{}
	}
	return code, nil
}

// setAddress stages a caller's address edit onto a patch, after the country
// rule. Contact and company edits share it so neither can stage a country the
// other would refuse.
func setAddress(p *storekit.Patch, current, edit *crmcontracts.Address) error {
	if edit == nil {
		return nil
	}
	checked, err := checkedAddress(edit)
	if err != nil {
		return err
	}
	cur := addressColumns(current)
	p.Set("address_line1", cur.Line1, checked.Line1)
	p.Set("address_line2", cur.Line2, checked.Line2)
	p.Set("address_city", cur.City, checked.City)
	p.Set("address_region", cur.Region, checked.Region)
	p.Set("address_postal_code", cur.PostalCode, checked.PostalCode)
	p.Set("address_country", cur.Country, checked.Country)
	return nil
}

// addressColumns destructures the contract's Address into the six
// contact/company columns; a nil address is six NULLs.
func addressColumns(a *crmcontracts.Address) crmcontracts.Address {
	if a == nil {
		return crmcontracts.Address{}
	}
	return *a
}

// addressOrNil collapses six all-NULL columns back to "no address" so
// the wire shape stays a null, not an empty object.
func addressOrNil(a crmcontracts.Address) *crmcontracts.Address {
	if a.Line1 == nil && a.Line2 == nil && a.City == nil && a.Region == nil &&
		a.PostalCode == nil && a.Country == nil {
		return nil
	}
	return &a
}
