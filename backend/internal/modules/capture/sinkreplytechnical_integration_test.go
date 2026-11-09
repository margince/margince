// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A reply from a technical address names no contact, even where a contact
// stored before the rule still holds it. A human's reply still names theirs.
func TestAReplyFromATechnicalAddressNamesNoContact(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	seed := func(tx pgx.Tx, address string) (ids.UUID, error) {
		var id ids.UUID
		if err := tx.QueryRow(ctx, `INSERT INTO contact (full_name, source, captured_by)
			VALUES ('Seeded', 'manual', 'human:test') RETURNING id`).Scan(&id); err != nil {
			return id, err
		}
		_, err := tx.Exec(ctx, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
			VALUES ($1, $2, true, 'manual', 'human:test')`, id, address)
		return id, err
	}
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := seed(tx, "4711@bcc.hubspot.com"); err != nil {
			return err
		}
		human, err := seed(tx, "anna@hubspot.com")
		if err != nil {
			return err
		}
		if _, found, err := mailReplyContact(ctx, tx, "4711@bcc.hubspot.com"); err != nil || found {
			t.Errorf("a technical address resolved: found=%v err=%v, want no contact", found, err)
		}
		got, found, err := mailReplyContact(ctx, tx, "anna@hubspot.com")
		if err != nil || !found || got.UUID != human {
			t.Errorf("a human's reply resolved to %v found=%v err=%v, want %v", got, found, err, human)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
