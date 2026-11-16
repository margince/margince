// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"testing"
)

func TestOtherCaptureLabelsRemainWritableAcrossARollback(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	var id string
	if err := conn.QueryRow(ctx, `INSERT INTO activity(kind,source,captured_by,capture_label,capture_labeled_at) VALUES('email','manual','system','other',now()) RETURNING id::text`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	applyMigrationFile(t, conn, "core/1794701219_linkedin_can_name_a_logged_message.down.sql")
	applyMigrationFile(t, conn, "core/1794701217_other_names_the_capture_remainder.down.sql")
	if _, err := conn.Exec(ctx, `UPDATE activity SET subject='Updated after rollback' WHERE id=$1`, id); err != nil {
		t.Fatalf("renamed row cannot be updated after rollback: %v", err)
	}
	applyMigrationFile(t, conn, "core/1794701217_other_names_the_capture_remainder.up.sql")
	applyMigrationFile(t, conn, "core/1794701219_linkedin_can_name_a_logged_message.up.sql")
	var label string
	if err := conn.QueryRow(ctx, `SELECT capture_label FROM activity WHERE id=$1`, id).Scan(&label); err != nil || label != "other" {
		t.Fatalf("label after re-upgrade = %q, %v", label, err)
	}
}

func TestLinkedInRegistrationPreservesAnExistingTransportOwner(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `UPDATE channel_provider SET transport='unit', label='LinkedIn', supplies_transport=true WHERE provider='linkedin'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(context.Background(), `UPDATE channel_provider SET transport='core', supplies_transport=false WHERE provider='linkedin'`); err != nil {
			t.Errorf("restoring provider owner: %v", err)
		}
	})
	applyMigrationFile(t, conn, "core/1794701219_linkedin_can_name_a_logged_message.up.sql")
	var transport string
	if err := conn.QueryRow(ctx, `SELECT transport FROM channel_provider WHERE provider='linkedin'`).Scan(&transport); err != nil || transport != "unit" {
		t.Fatalf("transport owner = %q, %v", transport, err)
	}
}
