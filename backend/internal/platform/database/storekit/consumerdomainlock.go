// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/freemail"
)

// LockConsumerDomain serializes the writers that decide what a domain is: the
// consumer-mail list and a company's domain list. Keyed on the registrable
// domain, so `mail.gmx.net` and `gmx.net` queue together. A writer with several
// domains takes them in sorted order.
func LockConsumerDomain(ctx context.Context, tx pgx.Tx, domain string) error {
	key := freemail.Registrable(domain)
	if _, err := tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('margince:consumer-mail:' || $1)::bigint)`, key); err != nil {
		return fmt.Errorf("storekit: serializing the decision about %s: %w", key, err)
	}
	return nil
}
