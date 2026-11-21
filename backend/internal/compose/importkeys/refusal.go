// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importkeys

import (
	"errors"

	"github.com/margince/margince/backend/internal/compose/importtextbounds"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// The held reasons name the row's own value and nothing about the incumbent.
// They never say who holds it, nor whether the caller could see them.
var (
	LinkedInHeldWhy     = migration.Why(migration.ReasonLinkedinHeld)
	DomainHeldWhy       = migration.Why(migration.ReasonDomainHeld)
	ContactEmailHeldWhy = migration.Why(migration.ReasonContactEmailHeld)
	ContactHoldsLeadWhy = migration.Why(migration.ReasonContactHoldsLead)
	AddressErasedWhy    = migration.Why(migration.ReasonAddressErased)
	DuplicateEmailWhy   = migration.Why(migration.ReasonDuplicateEmail)
	NaturalKeyTakenWhy  = migration.Why(migration.ReasonNaturalKeyTaken)
)

// UpdateRefusal names the skip for an update the store refused over one bad
// row. The commit and the test run both read a refused update through it.
func UpdateRefusal(err error) (migration.Reason, bool) {
	if contacts.IsClaimedEmail(err) {
		return DuplicateEmailWhy, true
	}
	if _, ok := errors.AsType[*contacts.DuplicateDomainError](err); ok {
		return DomainHeldWhy, true
	}
	if _, ok := errors.AsType[*contacts.DuplicateLeadError](err); ok {
		return NaturalKeyTakenWhy, true
	}
	if _, ok := errors.AsType[*contacts.LeadHoldsIdentityError](err); ok {
		return ContactEmailHeldWhy, true
	}
	if _, ok := errors.AsType[*contacts.ContactHoldsIdentityError](err); ok {
		return ContactHoldsLeadWhy, true
	}
	if LinkedInClaimed(err) {
		return LinkedInHeldWhy, true
	}
	if _, ok := errors.AsType[*storekit.ErasedIdentifierError](err); ok {
		return AddressErasedWhy, true
	}
	return importtextbounds.NameTooLong(err)
}

// LinkedInClaimed reports whether the store refused a write because another
// live company or lead holds its LinkedIn URL.
func LinkedInClaimed(err error) bool {
	if _, ok := errors.AsType[*contacts.DuplicateCompanyLinkedInError](err); ok {
		return true
	}
	_, ok := errors.AsType[*contacts.DuplicateLeadLinkedInError](err)
	return ok
}
