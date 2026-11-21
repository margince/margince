// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importkeys

// The question an update asks when it moves a key onto a record: does another
// record already hold it. An earlier row of the run holds it through the
// ledger. A stored record holds it through the store's own lookups, the ones
// its uniqueness checks use. The record's own values do not count.

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Holder is the record type holding a key a row moves.
type Holder struct {
	Kind   Kind
	Object string
}

// holderRefusers are the record types whose hold on a key the store refuses a
// lead or contact update over, in the order the store asks. A contact may share
// a profile with another contact, so a LinkedIn URL is no key between two.
var holderRefusers = map[string]map[Kind][]string{
	migration.ObjectContact: {Email: {migration.ObjectContact, migration.ObjectLead}, LinkedIn: {migration.ObjectLead}},
	migration.ObjectLead:    {Email: {migration.ObjectLead, migration.ObjectContact}, LinkedIn: {migration.ObjectContact}},
}

// MovedKinds are the key kinds an update moves, in the order the store asks
// them. fields maps a column to its key kind.
func MovedKinds(changed map[string]string, fields map[string]Kind) []Kind {
	var moved []Kind
	for field, kind := range fields {
		if _, ok := changed[field]; ok {
			moved = append(moved, kind)
		}
	}
	slices.Sort(moved)
	return moved
}

// ClaimUnmoved claims a landed row's keys except the kinds its update has not
// yet moved onto the record.
func (l Ledger) ClaimUnmoved(object string, keys map[Kind]string, moved []Kind) {
	for _, kind := range moved {
		delete(keys, kind)
	}
	l.Claim(object, keys)
}

// MovedKeyWhy is the reason an update that moves these key kinds is refused,
// or the zero reason when no other record holds them. The reasons are the ones
// the store's refusals map to.
func MovedKeyWhy(ctx context.Context, pool *pgxpool.Pool, ledger Ledger, object string, self ids.UUID, keys map[Kind]string, moved []Kind) (migration.Reason, error) {
	held, found, err := MovedKeyHeld(ctx, pool, ledger, object, self, keys, moved)
	switch {
	case err != nil || !found:
		return migration.Reason{}, err
	case held.Kind == LinkedIn && object == migration.ObjectCompany:
		return LinkedInHeldWhy, nil
	case held.Kind == Domain:
		return DomainHeldWhy, nil
	case object == migration.ObjectLead && held.Object == migration.ObjectContact:
		return ContactHoldsLeadWhy, nil
	case object == migration.ObjectLead:
		return NaturalKeyTakenWhy, nil
	case held.Object == migration.ObjectContact:
		return DuplicateEmailWhy, nil
	}
	return ContactEmailHeldWhy, nil
}

// MovedKeyHeld names the first holder of a key a row moves, asking the kinds in
// the order given.
func MovedKeyHeld(ctx context.Context, pool *pgxpool.Pool, ledger Ledger, object string, self ids.UUID, keys map[Kind]string, moved []Kind) (Holder, bool, error) {
	for _, kind := range moved {
		value := keys[kind]
		if value == "" {
			continue
		}
		var holder string
		var err error
		if object == migration.ObjectCompany {
			holder, err = companyKeyHolder(ctx, pool, ledger, kind, value, self)
		} else {
			holder, err = leadOrContactKeyHolder(ctx, pool, ledger, object, kind, value, self)
		}
		if err != nil || holder != "" {
			return Holder{kind, holder}, holder != "", err
		}
	}
	return Holder{}, false, nil
}

// leadOrContactKeyHolder is MovedKeyHeld for the address or profile a lead or contact moves.
func leadOrContactKeyHolder(ctx context.Context, pool *pgxpool.Pool, ledger Ledger, object string, kind Kind, value string, self ids.UUID) (string, error) {
	refusers := holderRefusers[object][kind]
	if len(refusers) == 0 {
		return "", nil
	}
	keys := storekit.IdentityKeys{Emails: []string{value}}
	if kind == LinkedIn {
		keys = storekit.IdentityKeys{LinkedInURLs: []string{value}}
	}
	var holders storekit.IdentityHolders
	if err := probe(ctx, pool, fmt.Sprintf("%q", value), func(tx pgx.Tx) (err error) {
		holders, err = storekit.IdentityHoldersOf(ctx, tx, keys)
		return err
	}); err != nil {
		return "", err
	}
	for _, refuser := range refusers {
		if ledger.Held(kind, value, refuser) || holds(holders, refuser, object, self) {
			return refuser, nil
		}
	}
	return "", nil
}

// holds reports whether a live record of this type other than self is among the holders.
func holds(holders storekit.IdentityHolders, holder, object string, self ids.UUID) bool {
	if holder == migration.ObjectContact {
		for _, contact := range holders.Contacts {
			if contact.UUID != self || object != migration.ObjectContact {
				return true
			}
		}
		return false
	}
	for _, lead := range holders.Leads {
		if lead.UUID != self || object != migration.ObjectLead {
			return true
		}
	}
	return false
}

// companyKeyHolder is MovedKeyHeld for the LinkedIn URL or domain a company moves.
func companyKeyHolder(ctx context.Context, pool *pgxpool.Pool, ledger Ledger, kind Kind, value string, self ids.UUID) (string, error) {
	if ledger.Held(kind, value, migration.ObjectCompany) {
		return migration.ObjectCompany, nil
	}
	var held bool
	err := probe(ctx, pool, fmt.Sprintf("%q", value), func(tx pgx.Tx) (err error) {
		switch kind {
		case LinkedIn:
			held, err = contacts.CompanyLinkedInRival(ctx, tx, ids.From[ids.CompanyKind](self), value)
		case Domain:
			held, err = contacts.CompanyDomainHeld(ctx, tx, ids.From[ids.CompanyKind](self), value)
		}
		return err
	})
	if held {
		return migration.ObjectCompany, err
	}
	return "", err
}
