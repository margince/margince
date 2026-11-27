// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestAnonymizingALegacySourceKeepsThePrivacyCertificateEmpty(t *testing.T) {
	e := setupSARIdentifiers(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, sql := range []string{
		`ALTER TABLE contact DROP CONSTRAINT contact_source_check`,
		`ALTER TABLE contact DISABLE TRIGGER contact_source_channel`,
		`UPDATE contact SET source='gmail:private-41' WHERE id=$1`,
		`ALTER TABLE contact ENABLE TRIGGER contact_source_channel`,
		`ALTER TABLE contact ADD CONSTRAINT contact_source_check CHECK (` + provenance.ListedRecordSourceSQL("source") + `) NOT VALID`,
	} {
		args := []any{}
		if sql == `UPDATE contact SET source='gmail:private-41' WHERE id=$1` {
			args = append(args, e.contact)
		}
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	svc := &RetentionService{db: e.db, eraser: NewEraser(e.db)}
	done, err := svc.anonymiseOneContact(e.ctx, e.contact.UUID, PurgeOwnerRule, func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return true, nil })
	if err != nil || !done {
		t.Fatalf("anonymized=%v err=%v", done, err)
	}
	var certificates, sourceEvents int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND action='anonymize' AND before IS NULL AND after IS NULL`, e.contact).Scan(&certificates); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=$1 AND envelope->>'type'='contact.updated'`, e.contact.String()).Scan(&sourceEvents); err != nil {
		t.Fatal(err)
	}
	if certificates != 1 || sourceEvents != 0 {
		t.Fatalf("certificates=%d source events=%d", certificates, sourceEvents)
	}
}
