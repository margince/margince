// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company object's create, its claimed-key checks, and the LinkedIn one a
// lead shares.
//
// A domain names ONE company across the estate, the same way an email names one
// contact, so the preview has to answer the same question the contact path does:
// will the store refuse this row before it is written?

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// createCompany lands one row as a company, its identity-map row in the same
// transaction.
func (w *csvWriters) createCompany(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	in := companyCreateFrom(textFields(row.Fields), typed, w.sourceSystem, w.provenanceOf(row.ExternalID))
	err := w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		if err := w.authors.resolve(ctx, tx, &in.Author); err != nil {
			return ids.UUID{}, err
		}
		company, err := w.contacts.CreateCompanyTx(ctx, tx, in, w.cols.store)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating company %s: %w", row.ExternalID, err)
		}
		return ids.UUID(company.Id), nil
	})
	if _, ok := errors.AsType[*contacts.DuplicateDomainError](err); ok {
		// A domain names one company across the estate, as an address names one
		// contact. A row claiming a held one is a skip with a reason, not a
		// failed run, so the rest of the file still lands.
		return skippedFor(domainClaimedWhy), nil
	}
	if linkedInClaimed(err) {
		return skippedFor(linkedInClaimedWhy), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// companyDomainHeld answers whether a live company other than except holds
// this domain, without regard to which company holds it.
//
// Not visibility-filtered, and that is the opposite of the name-collision check
// beside it — for the reason the contact email version gives. A duplicate NAME is
// a judgement the commit resolves by creating a twin and filing a review pair,
// so telling a caller about an incumbent they cannot see would disclose one. A
// duplicate domain is not a judgement: the key is estate-wide, so the commit
// refuses the row whoever holds the domain, and a preview promising a create
// would simply be wrong.
func (w *csvWriters) companyDomainHeld(ctx context.Context, domain string, except *ids.UUID) (bool, error) {
	var held bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM company_domain
			                WHERE domain = lower($1) AND archived_at IS NULL
			                  AND ($2::uuid IS NULL OR company_id <> $2))`,
			domain, except).Scan(&held)
	}); err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", domain, err)
	}
	return held, nil
}

// linkedInClaimed reports whether the store refused a write because another
// live company or lead holds its LinkedIn URL.
func linkedInClaimed(err error) bool {
	if _, ok := errors.AsType[*contacts.DuplicateCompanyLinkedInError](err); ok {
		return true
	}
	_, ok := errors.AsType[*contacts.DuplicateLeadLinkedInError](err)
	return ok
}

// companyLinkedInHeld is linkedInClaimed asked before the write, so the
// preview skips the row the commit will.
func (w *csvWriters) companyLinkedInHeld(ctx context.Context, url string, except *ids.UUID) (bool, error) {
	var held bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM company
			                WHERE lower(linkedin_url) = lower($1) AND archived_at IS NULL
			                  AND ($2::uuid IS NULL OR id <> $2))`,
			url, except).Scan(&held)
	}); err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", url, err)
	}
	return held, nil
}

// leadLinkedInHeld is the lead store's own probe, asked before a create.
func (w *csvWriters) leadLinkedInHeld(ctx context.Context, url string) (bool, error) {
	var held bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		_, held, err = storekit.LiveLeadByLinkedInURL(ctx, tx, url, nil)
		return err
	}); err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", url, err)
	}
	return held, nil
}
