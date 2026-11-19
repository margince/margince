// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importkeys

// The stored records' half of each question. Every key here is estate-wide:
// the store refuses a second holder whoever holds it, so these probes are not
// visibility-filtered. A preview filtered by visibility would promise a create
// the store then refuses. The refusal names only the row's own value, never
// the holder.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// probe runs one read in its own workspace transaction and names what it
// asked about when it fails.
func probe(ctx context.Context, pool *pgxpool.Pool, about string, read func(pgx.Tx) error) error {
	if err := database.WithWorkspaceTx(ctx, pool, read); err != nil {
		return fmt.Errorf("import: checking whether %s is already held: %w", about, err)
	}
	return nil
}

// EmailsHeld answers which of these addresses a live contact or a live lead
// holds, in one query: the identity check a contact create asks.
func EmailsHeld(ctx context.Context, pool *pgxpool.Pool, emails []string) (map[string]bool, error) {
	held := make(map[string]bool, len(emails))
	err := probe(ctx, pool, fmt.Sprintf("%d addresses", len(emails)), func(tx pgx.Tx) error {
		holders, err := storekit.IdentityHoldersOf(ctx, tx, storekit.IdentityKeys{Emails: emails})
		for _, email := range emails {
			held[email] = holders.ContactHoldsEmail(email) || holders.LeadHoldsEmail(email)
		}
		return err
	})
	return held, err
}

// LeadEmailHeld is the refusal CreateLeadTx makes for an address a live lead holds.
func LeadEmailHeld(ctx context.Context, pool *pgxpool.Pool, email string) (bool, error) {
	var held bool
	err := probe(ctx, pool, fmt.Sprintf("%q", email), func(tx pgx.Tx) error {
		var err error
		_, held, err = storekit.LiveLeadByEmail(ctx, tx, email, nil)
		return err
	})
	return held, err
}

// ContactHoldsLeadKeys is the refusal CreateLeadTx makes for a lead row whose
// address or LinkedIn URL a contact holds.
func ContactHoldsLeadKeys(ctx context.Context, pool *pgxpool.Pool, email, linkedInURL string) (bool, error) {
	var keys storekit.IdentityKeys
	if email != "" {
		keys.Emails = []string{email}
	}
	if linkedInURL != "" {
		keys.LinkedInURLs = []string{linkedInURL}
	}
	var held bool
	err := probe(ctx, pool, "a lead row's address", func(tx pgx.Tx) error {
		holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
		held = len(holders.Contacts) > 0
		return err
	})
	return held, err
}

// LinkedInHeld is the refusal the lead or company create makes for a LinkedIn
// URL a live record of that type holds. A contact's handle is not a key.
func LinkedInHeld(ctx context.Context, pool *pgxpool.Pool, object, url string) (bool, error) {
	if url == "" || object == migration.ObjectContact {
		return false, nil
	}
	var held bool
	err := probe(ctx, pool, fmt.Sprintf("%q", url), func(tx pgx.Tx) error {
		if object == migration.ObjectLead {
			var err error
			_, held, err = storekit.LiveLeadByLinkedInURL(ctx, tx, url, nil)
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM company WHERE lower(linkedin_url) = lower($1) AND archived_at IS NULL)`,
			url).Scan(&held)
	})
	return held, err
}

// DomainHeld is the refusal the company create makes for a domain a live
// company holds, compared in every spelling the store keeps.
func DomainHeld(ctx context.Context, pool *pgxpool.Pool, domain string) (bool, error) {
	if domain == "" {
		return false, nil
	}
	var held bool
	err := probe(ctx, pool, fmt.Sprintf("%q", domain), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM company_domain
			   WHERE domain = ANY($1) AND archived_at IS NULL)`,
			owndomain.Spellings(domain)).Scan(&held)
	})
	return held, err
}

// VisibleDomainHolder finds the live company holding a domain, in any spelling
// the store keeps, inside the caller's row scope. A domain is unique among
// live rows, so one row is the whole answer.
func VisibleDomainHolder(ctx context.Context, pool *pgxpool.Pool, domain string) (ids.UUID, bool, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return ids.UUID{}, false, err
	}
	var holder ids.UUID
	found := false
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		domainPos := arg(owndomain.Spellings(domain))
		scope, err := auth.ScopeClauseFor(ctx, migration.ObjectCompany, "o", arg)
		if err != nil {
			return err
		}
		if scope == "" {
			scope = "TRUE"
		}
		err = tx.QueryRow(ctx, storekit.SQLf(`
			SELECT o.id FROM company_domain d JOIN company o ON o.id = d.company_id
			 WHERE d.domain = ANY($%d) AND d.archived_at IS NULL
			   AND o.archived_at IS NULL AND o.merged_into_id IS NULL AND %s
			 LIMIT 1`, domainPos, scope), args...).Scan(&holder)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		found = err == nil
		return err
	})
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("import: finding the company holding %q: %w", domain, err)
	}
	return holder, found, nil
}

// WritableDomainHolder is VisibleDomainHolder for a row that will be filed
// onto the holder: the caller must also be allowed to change it. A holder the
// caller may only read is no answer, so the row keeps its domain_held refusal.
func WritableDomainHolder(ctx context.Context, pool *pgxpool.Pool, domain string) (ids.UUID, bool, error) {
	if !auth.Allows(ctx, migration.ObjectCompany, principal.ActionRead) || !auth.Allows(ctx, migration.ObjectCompany, principal.ActionUpdate) {
		return ids.UUID{}, false, nil
	}
	holder, found, err := VisibleDomainHolder(ctx, pool, domain)
	if err != nil || !found {
		return ids.UUID{}, false, err
	}
	err = database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return auth.EnsureWritableLive(ctx, tx, migration.ObjectCompany, holder)
	})
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return ids.UUID{}, false, nil
	}
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("import: checking whether the company holding %q may be changed: %w", domain, err)
	}
	return holder, true, nil
}
