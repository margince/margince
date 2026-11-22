// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importkeys

// The refusals the create door of each record type makes over a unique key, and
// the words a row refused over one is skipped with. The test run, the import and
// the moved-key check say each refusal in the same words.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

// The reasons name the row's own value and nothing about the holder. They never
// say who holds it, nor whether the caller could see them.
var (
	LinkedInHeldWhy     = migration.Why(migration.ReasonLinkedinHeld)
	DomainHeldWhy       = migration.Why(migration.ReasonDomainHeld)
	ContactEmailHeldWhy = migration.Why(migration.ReasonContactEmailHeld)
	ContactHoldsLeadWhy = migration.Why(migration.ReasonContactHoldsLead)
	DuplicateEmailWhy   = migration.Why(migration.ReasonDuplicateEmail)
	NaturalKeyTakenWhy  = migration.Why(migration.ReasonNaturalKeyTaken)
)

// KeyCheck is one key the create door refuses: held by an earlier row of the
// run, or by a stored record.
type KeyCheck struct {
	Why    migration.Reason
	InFile bool
	Stored func() (bool, error)
}

// CreateKeyChecks are the refusals the create door of the row's type makes, in
// its order. contactEmailHeld is the contact create's own stored check, which
// a dry run answers from one query per page.
func CreateKeyChecks(ctx context.Context, pool *pgxpool.Pool, ledger Ledger, object string, keys map[Kind]string, email string, contactEmailHeld func() (bool, error)) []KeyCheck {
	url := keys[LinkedIn]
	linkedIn := KeyCheck{
		LinkedInHeldWhy, ledger.Held(LinkedIn, url, object),
		func() (bool, error) { return LinkedInHeld(ctx, pool, object, url) },
	}
	switch object {
	case migration.ObjectContact:
		return []KeyCheck{{
			ContactEmailHeldWhy,
			ledger.Held(Email, keys[Email], migration.ObjectContact, migration.ObjectLead),
			contactEmailHeld,
		}}
	case migration.ObjectLead:
		return []KeyCheck{
			{
				NaturalKeyTakenWhy, ledger.Held(Email, keys[Email], migration.ObjectLead),
				func() (bool, error) { return LeadEmailHeld(ctx, pool, email) },
			},
			linkedIn,
			{
				ContactHoldsLeadWhy, ledger.Held(Email, keys[Email], migration.ObjectContact) ||
					ledger.Held(LinkedIn, url, migration.ObjectContact),
				func() (bool, error) { return ContactHoldsLeadKeys(ctx, pool, email, url) },
			},
		}
	case migration.ObjectCompany:
		domain := keys[Domain]
		return []KeyCheck{{
			DomainHeldWhy, ledger.Held(Domain, domain, migration.ObjectCompany),
			func() (bool, error) { return DomainHeld(ctx, pool, domain) },
		}, linkedIn}
	}
	return nil
}

// StoreKeyWhy is the reason for a store write refused over a unique key, so a
// row skipped at commit says what the test run said. One bad row is a skip, not
// a failed run, and the rest of the file lands.
func StoreKeyWhy(err error) (migration.Reason, bool) {
	if _, ok := errors.AsType[*contacts.DuplicateDomainError](err); ok {
		return DomainHeldWhy, true
	}
	if _, ok := errors.AsType[*contacts.DuplicateCompanyLinkedInError](err); ok {
		return LinkedInHeldWhy, true
	}
	if _, ok := errors.AsType[*contacts.DuplicateLeadLinkedInError](err); ok {
		return LinkedInHeldWhy, true
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
	return migration.Reason{}, false
}
