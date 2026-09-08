// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// "This is not a company" — one decision, one transaction.
//
// Archiving alone does not settle it. The record was minted from mail, so the
// next message on the same domain mints it again, and the person who archived
// it a week ago learns nothing about why it is back. The standing refusal is
// what stops that, and it belongs to the DOMAIN rather than to the record —
// which is why this verb writes both halves and neither alone.
//
// Two HTTP calls cannot do it. Whichever order they run in, a failure between
// them commits half the intent: a refused domain whose company is still on
// every list, or an archived company whose domain still mints new ones. One
// transaction has no such middle.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Rejection is both halves of the decision, because both landed.
//
// The archived record alone would leave the refusal — the half that stops the
// company coming back — invisible to the person who just made it, which is what
// made the two-call version report a partial write as total failure.
type Rejection struct {
	Organization crmcontracts.Organization
	Domain       BlockedDomain
}

// RejectOrganization archives a company and refuses its domain another, in ONE
// transaction.
//
// The domain is read HERE, under the lock the archive takes, and is not a
// parameter. A domain the caller carried is a snapshot of what the page showed:
// on a company with several domains, a primary that moved since then would
// suppress the address nobody writes from any more while archiving a record
// whose live mail still arrives — and the company returns tomorrow on the
// domain that was never refused.
//
// Both gates, before anything: the archive is `organization:delete` and the
// standing domain decision is `organization:update`. A seat holding one and not
// the other is refused with nothing written, which is what the two-call version
// could not promise — a rep holding update alone suppressed the domain and then
// took a 403 on the archive.
func (s *Store) RejectOrganization(
	ctx context.Context, id ids.OrganizationID, reason string, ifVersion *int64,
) (Rejection, error) {
	if reason == "" {
		return Rejection{}, httperr.Validation(fieldKeyReason, "required",
			"say why this is not a company: the refusal outlives the record, and only a reason makes it reviewable")
	}
	if err := auth.Require(ctx, entityOrganization, principal.ActionDelete); err != nil {
		return Rejection{}, err
	}
	if err := auth.Require(ctx, entityOrganization, principal.ActionUpdate); err != nil {
		return Rejection{}, err
	}
	active, err := s.activeColumns(ctx, entityOrganization)
	if err != nil {
		return Rejection{}, err
	}
	var out Rejection
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, entityOrganization, id.UUID); err != nil {
			return err
		}
		// The anchor first, and not only through the archive's own copy of this
		// guard below: whether the installation's own company can be rejected
		// must not depend on whether it happens to carry a domain. Read second,
		// a domainless anchor would be refused for the wrong reason and an
		// anchor WITH a domain would have its refusal decided by the read.
		if err := refuseIfAnchor(ctx, tx, id, "id",
			"it cannot be rejected. Reject a different company, or edit this one on the company page"); err != nil {
			return err
		}
		// THE COMPANY ROW FIRST, before the domain read locks anything.
		//
		// Both this verb and a plain archive touch the same two tables, and
		// only the order is negotiable: ArchiveOrganization takes the company
		// row under its guarded patch and then updates organization_domain, so
		// a rejection that locked the domain first and the company second would
		// meet it head-on. Two transactions each holding what the other wants
		// is a deadlock Postgres breaks by aborting one of them — a rejection
		// or somebody's archive failing at random, under exactly the concurrency
		// that makes it hard to reproduce.
		//
		// The archive below takes this same lock again; a second acquire of a
		// row this transaction already holds is free.
		if _, err := storekit.LockRow(ctx, tx, entityOrganization, id.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		// The domain is read BEFORE the archive, because the archive retires
		// organization_domain with the record — read after, the live-only query
		// below would find nothing and every rejection would refuse itself.
		domain, err := primaryDomainForRejectTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if out.Organization, err = archiveOrganizationTx(ctx, tx, id, ifVersion, active); err != nil {
			return err
		}
		out.Domain, err = recordHumanDomainAdmissionTx(ctx, tx, domain, DomainSuppressed, reason)
		return err
	})
	if err != nil {
		return Rejection{}, err
	}
	return out, nil
}

// primaryDomainForRejectTx reads the company's current primary domain, locked,
// and refuses the rejection when there is none.
//
// FOR UPDATE because this read decides what the write below refuses, and the
// two are one unit: unlocked, a concurrent primary change lands between them
// and the refusal is recorded against a domain the company no longer uses.
//
// A company with no domain is refused rather than archived silently: somebody
// typed it in by hand, so it was never derived from mail and there is nothing a
// refusal would stop. Archiving it is the plain archive, and saying so is
// better than doing half of what the button promises.
func primaryDomainForRejectTx(ctx context.Context, tx pgx.Tx, id ids.OrganizationID) (string, error) {
	var domain string
	err := tx.QueryRow(ctx, `
		SELECT domain FROM organization_domain
		 WHERE organization_id = $1 AND is_primary AND archived_at IS NULL
		 FOR UPDATE`, id).Scan(&domain)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", httperr.Validation("id", "no_primary_domain",
			"this company has no primary domain, so there is nothing to refuse. Archive it instead")
	}
	if err != nil {
		return "", fmt.Errorf("people: reading the company's primary domain: %w", err)
	}
	// Normalized to the registrable form the admission table is keyed on. A
	// company may carry `mail.vendor.example`; the refusal has to be the one
	// the capture path looks up, or it records a decision nothing consults.
	base, ok := freemail.Hostname(domain)
	if !ok {
		return "", httperr.Validation("id", "no_primary_domain",
			"this company's primary domain is not a domain name, so there is nothing to refuse. Archive it instead")
	}
	return base, nil
}
