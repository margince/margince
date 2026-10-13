// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The update half of the held-key refusals. An update may move an address, a
// domain or a LinkedIn URL onto its record. The store refuses that when another
// live record holds the key, and reconcile skips the row. The preview asks the
// same question first, so it does not promise an update the commit skips.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// updateKeyRefusal is reconcile's skip reason for a changed unique key that
// another live record holds, or "". Not visibility-filtered, for the reason
// createPathRefusal gives: the store refuses the key whoever holds it.
func (w *csvWriters) updateKeyRefusal(
	ctx context.Context, id ids.UUID, changed map[string]string, typed map[string]any,
) (string, error) {
	email := canonicalFor(fieldEmail, changed[fieldEmail])
	domain := canonicalFor(fieldDomain, changed[fieldDomain])
	url := ""
	if parsed, ok := typed[fieldLinkedIn].(string); ok && changed[fieldLinkedIn] != "" {
		url = parsed
	}
	var reason string
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		switch w.object {
		case migration.ObjectContact:
			reason, err = heldElsewhere(ctx, tx, email, skipReasonDuplicateEmail,
				`SELECT EXISTS (SELECT 1 FROM contact_email WHERE email = lower($1) AND archived_at IS NULL AND contact_id <> $2)`, id)
		case migration.ObjectCompany:
			reason, err = heldElsewhere(ctx, tx, domain, domainClaimedReason,
				`SELECT EXISTS (SELECT 1 FROM company_domain WHERE domain = lower($1) AND archived_at IS NULL AND company_id <> $2)`, id)
			if reason == "" && err == nil {
				reason, err = heldElsewhere(ctx, tx, url, linkedInClaimedReason,
					`SELECT EXISTS (SELECT 1 FROM company WHERE lower(linkedin_url) = lower($1) AND archived_at IS NULL AND id <> $2)`, id)
			}
		case migration.ObjectLead:
			reason, err = leadKeyHeldElsewhere(ctx, tx, id, email, url)
		}
		return err
	})
	if err != nil {
		return "", fmt.Errorf("import: checking whether the row's changed keys are already held: %w", err)
	}
	return reason, nil
}

// heldElsewhere runs one existence query, binding the key and the record to
// leave out. It answers the reason when it finds a holder.
func heldElsewhere(ctx context.Context, tx pgx.Tx, key, reason, query string, id ids.UUID) (string, error) {
	if key == "" {
		return "", nil
	}
	var held bool
	if err := tx.QueryRow(ctx, query, key, id).Scan(&held); err != nil || !held {
		return "", err
	}
	return reason, nil
}

// leadKeyHeldElsewhere asks the lead store's own identity probes, leaving
// this lead out.
func leadKeyHeldElsewhere(ctx context.Context, tx pgx.Tx, id ids.UUID, email, url string) (string, error) {
	holder, held, err := storekit.LiveLeadByEmail(ctx, tx, email, nil)
	if err != nil {
		return "", err
	}
	if held && holder.UUID != id {
		return leadEmailClaimedReason, nil
	}
	holder, held, err = storekit.LiveLeadByLinkedInURL(ctx, tx, url, nil)
	if err != nil || !held || holder.UUID == id {
		return "", err
	}
	return linkedInClaimedReason, nil
}
