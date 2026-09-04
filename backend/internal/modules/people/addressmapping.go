// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The contract Address and the six columns behind it, in both directions.
// Person and organization store the same six and share this seam: two
// spellings of "is this address empty" would disagree the first time one
// of them gained a field.

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// addressColumns destructures the contract's Address into the six
// person/organization columns; a nil address is six NULLs.
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
