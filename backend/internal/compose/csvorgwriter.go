// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The organization object's own claimed-key check.
//
// A domain names ONE company across the estate, the same way an email names one
// person, so the preview has to answer the same question the person path does:
// will the store refuse this row before it is written?

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
)

// orgDomainAlreadyHeld answers whether the estate already holds this row's
// domain, WITHOUT regard to which company holds it.
//
// Not visibility-filtered, and that is the opposite of the name-collision check
// beside it — for the reason the person email version gives. A duplicate NAME is
// a judgement the commit resolves by creating a twin and filing a review pair,
// so telling a caller about an incumbent they cannot see would disclose one. A
// duplicate domain is not a judgement: the key is estate-wide, so the commit
// refuses the row whoever holds the domain, and a preview promising a create
// would simply be wrong.
//
// The reason text names the row's own value and never the incumbent.
func (w *csvWriters) orgDomainAlreadyHeld(ctx context.Context, row migration.Row) (bool, error) {
	if w.object != migration.ObjectOrganization {
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
			`SELECT EXISTS (SELECT 1 FROM organization_domain WHERE domain = lower($1) AND archived_at IS NULL)`,
			domain).Scan(&held)
	}); err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", domain, err)
	}
	return held, nil
}
