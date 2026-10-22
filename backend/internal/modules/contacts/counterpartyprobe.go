// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// ProbeCounterpartyMatch refuses a sender whose address or domain already
// belongs to a record the caller may not file onto. Ensuring them files mail
// onto that contact, publishes it, and employs it at the domain's company.
// Those are writes, so a read-only share is not enough.
func (s *Store) ProbeCounterpartyMatch(ctx context.Context, email, domain string) error {
	return s.tx(ctx, func(tx pgx.Tx) error {
		return s.ProbeCounterpartyMatchTx(ctx, tx, email, domain)
	})
}

// ProbeCounterpartyMatchTx is ProbeCounterpartyMatch on the caller's
// transaction. It matches as the ensure does, and holds each matched record
// FOR SHARE until the ensure that follows has written.
func (s *Store) ProbeCounterpartyMatchTx(ctx context.Context, tx pgx.Tx, email, domain string) error {
	// An erased address creates nothing, so there is no record to ask about;
	// the ensure resolves it as suppressed.
	suppressed, err := storekit.EmailSuppressed(ctx, tx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil || suppressed {
		return err
	}
	matched, err := exactContactByEmail(ctx, tx, []string{email})
	if err != nil {
		return err
	}
	for _, id := range matched {
		if err := auth.EnsureAttachTarget(ctx, tx, entityContact, id.UUID); err != nil {
			return err
		}
	}
	if domain == "" {
		return nil
	}
	employer, _, ok, err := employerMatchTx(ctx, tx, domain)
	if err != nil || !ok || employer.Decision != DecisionExactCollision {
		return err
	}
	return auth.EnsureAttachTarget(ctx, tx, entityCompany, employer.CompanyID.UUID)
}
