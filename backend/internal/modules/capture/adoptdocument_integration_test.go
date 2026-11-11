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

func TestAnAdoptedMessageCarryingADocumentDoesNotInheritAnOpeningVerdict(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	for _, c := range []struct {
		name   string
		status string
		file   string
		kind   string
		want   string
	}{
		{"a cleared thread, no file", VerdictCleared, "", "", VerdictCleared},
		{"a cleared thread, a spreadsheet", VerdictCleared, "Cost+.xlsx", "application/zip", ""},
		{"an owner-shared thread, a spreadsheet", VerdictSharedByOwner, "Cost+.xlsx", "application/zip", ""},
		{"a cleared thread, a logo", VerdictCleared, "logo.png", "image/png", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
				seat, mail, key := ids.NewV7(), ids.NewV7(), "adopt-"+ids.NewV7().String()
				if _, err := tx.Exec(ctx, `INSERT INTO app_user (id, email, display_name, status)
					VALUES ($1, $2, 'Seat', 'active')`, seat, seat.String()+"@seat.example"); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO activity (id, kind, subject, body, direction, source_system, source_id, source,
					                      captured_by, counterparty_email, thread_key)
					VALUES ($1, 'email', 'Re: invoice', 'see attached', 'inbound', 'gmail', $2, 'gmail:'||$2,
					        'connector:gmail', 'billing@vendor.example', $3)`, mail, "adopt-"+mail.String(), key); err != nil {
					return err
				}
				if c.file != "" {
					if _, err := tx.Exec(ctx, `
						INSERT INTO attachment (entity_type, entity_id, activity_id, filename, content_type,
						                        storage_key, source, captured_by)
						VALUES ('activity', $1, $1, $2, $3, $4, 'gmail', 'connector:gmail')`,
						mail, c.file, c.kind, "k/"+mail.String()); err != nil {
						return err
					}
				}
				if _, err := tx.Exec(ctx, `
					INSERT INTO capture_thread_verdict (thread_key, user_id, status, seen_addresses, resolved_at)
					VALUES ($1, $2, $3, '{billing@vendor.example}', now())`, key, seat, c.status); err != nil {
					return err
				}
				got, err := settledVerdictForTx(ctx, tx, seat, adoptableMessage{
					id: ids.From[ids.ActivityKind](mail), threadKey: key, counterparty: "billing@vendor.example",
				})
				if err != nil {
					return err
				}
				if got != c.want {
					t.Errorf("inherited %q, want %q", got, c.want)
				}
				return nil
			}); err != nil {
				t.Fatalf("seeding and reading: %v", err)
			}
		})
	}
}
