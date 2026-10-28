// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The company object's own claimed-key checks, and the LinkedIn one a lead shares.
//
// A domain names ONE company across the estate, the same way an email names one
// contact, so the preview has to answer the same question the contact path does:
// will the store refuse this row before it is written?

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// companyDomainAlreadyHeld answers whether the estate already holds this row's
// domain, WITHOUT regard to which company holds it.
//
// Not visibility-filtered, and that is the opposite of the name-collision check
// beside it — for the reason the contact email version gives. A duplicate NAME is
// a judgement the commit resolves by creating a twin and filing a review pair,
// so telling a caller about an incumbent they cannot see would disclose one. A
// duplicate domain is not a judgement: the key is estate-wide, so the commit
// refuses the row whoever holds the domain, and a preview promising a create
// would simply be wrong.
//
// The reason text names the row's own value and never the incumbent.
func (w *csvWriters) companyDomainAlreadyHeld(ctx context.Context, row migration.Row) (bool, error) {
	if w.object != migration.ObjectCompany {
		return false, nil
	}
	domain := strings.TrimSpace(textFields(row.Fields)[fieldDomain])
	if domain == "" {
		return false, nil
	}
	// Compared as the store will hold it, so a file spelling a URL does not read
	// as a different domain from the host already on file.
	domain = canonicalFor(fieldDomain, domain)
	var held bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM company_domain
			   WHERE domain = ANY($1) AND archived_at IS NULL)`,
			owndomain.Spellings(domain)).Scan(&held)
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

// linkedInAlreadyHeld is linkedInClaimed asked before the write, so the
// preview skips the row the commit will. A contact's handle is not a key.
func (w *csvWriters) linkedInAlreadyHeld(ctx context.Context, typed map[string]any) (bool, error) {
	url, ok := typed[fieldLinkedIn].(string)
	if !ok || w.object == migration.ObjectContact {
		return false, nil
	}
	var held bool
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		if w.object == migration.ObjectLead {
			var err error
			_, held, err = storekit.LiveLeadByLinkedInURL(ctx, tx, url, nil)
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM company WHERE lower(linkedin_url) = lower($1) AND archived_at IS NULL)`,
			url).Scan(&held)
	})
	if err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", url, err)
	}
	return held, nil
}
