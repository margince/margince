// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The plan behind a busy account's Documents list. A plan that reads every
// attachment of the installation answers correctly and slowly, so only the
// plan can show it.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedBusyAccount writes one account with 5,000 mails and 10,000 of their
// files, filed through each link arm. Forty quieter accounts hold most of the
// installation's 90,000 attachments.
func seedBusyAccount(ctx context.Context, t *testing.T, tx pgx.Tx, busy ids.UUID) {
	t.Helper()
	if _, err := tx.Exec(ctx, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Busy', 'seed', 'system')`, busy); err != nil {
		t.Fatalf("seeding the busy account: %v", err)
	}
	// The last-activity and link-ceiling triggers recompute an account per
	// link row, minutes for this volume, and this plan reads none of what they
	// write. The rollback restores them. A batch takes no binds, so the rest
	// names the account by a temp table.
	if _, err := tx.Exec(ctx, `
	ALTER TABLE activity DISABLE TRIGGER USER; ALTER TABLE activity_link DISABLE TRIGGER USER;
	ALTER TABLE relationship DISABLE TRIGGER USER; ALTER TABLE deal DISABLE TRIGGER USER;
	ALTER TABLE attachment DISABLE TRIGGER USER; ALTER TABLE contact DISABLE TRIGGER USER;
	CREATE TEMP TABLE seeded_busy AS SELECT id FROM company WHERE display_name = 'Busy';
	INSERT INTO company (display_name, source, captured_by)
	  SELECT 'Quiet '||g, 'seed', 'system' FROM generate_series(1, 40) g;
	CREATE TEMP TABLE seeded_company AS
	  SELECT id, row_number() OVER (ORDER BY id) n FROM company WHERE id = (SELECT id FROM seeded_busy) OR display_name LIKE 'Quiet %';
	CREATE TEMP TABLE seeded_contact AS
	  SELECT gen_random_uuid() id, g,
	         CASE WHEN g <= 200 THEN (SELECT id FROM seeded_busy) ELSE (SELECT id FROM seeded_company WHERE id <> (SELECT id FROM seeded_busy) ORDER BY id OFFSET g % 40 LIMIT 1) END company_id
	    FROM generate_series(1, 4000) g;
	INSERT INTO contact (id, full_name, source, captured_by)
	  SELECT id, 'Employee '||g, 'seed', 'system' FROM seeded_contact;
	INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
	  SELECT 'employment', id, company_id, 'seed', 'system' FROM seeded_contact;
	INSERT INTO pipeline (id, name) VALUES ('00000000-0000-7000-8000-00000000d0c5', 'Seeded');
	INSERT INTO stage (id, pipeline_id, name, "position")
	  VALUES ('00000000-0000-7000-8000-00000000d0c6', '00000000-0000-7000-8000-00000000d0c5', 'Open', 1);
	INSERT INTO deal (name, status, pipeline_id, stage_id, company_id, source, captured_by)
	  SELECT 'Deal '||n, 'open', '00000000-0000-7000-8000-00000000d0c5', '00000000-0000-7000-8000-00000000d0c6', id, 'seed', 'system'
	    FROM seeded_company;
	CREATE TEMP TABLE seeded_mail AS
	  SELECT gen_random_uuid() id, s.i g, c.id contact_id, c.company_id
	    FROM generate_series(1, 5000) s(i) JOIN seeded_contact c ON c.g = 1 + s.i % 200
	  UNION ALL
	  SELECT gen_random_uuid(), 5000 + s.i, c.id, c.company_id
	    FROM generate_series(1, 40000) s(i) JOIN seeded_contact c ON c.g = 201 + s.i % 3800;
	INSERT INTO activity (id, kind, subject, occurred_at, direction, source, captured_by)
	  SELECT id, 'email', 'Mail '||g, now() - (g % 700) * interval '1 hour', 'inbound', 'seed', 'system'
	    FROM seeded_mail;
	INSERT INTO activity_link (activity_id, entity_type, contact_id)
	  SELECT id, 'contact', contact_id FROM seeded_mail WHERE g % 10 <> 0;
	INSERT INTO activity_link (activity_id, entity_type, company_id)
	  SELECT id, 'company', company_id FROM seeded_mail WHERE g % 10 = 0;
	INSERT INTO activity_link (activity_id, entity_type, deal_id)
	  SELECT m.id, 'deal', d.id FROM seeded_mail m JOIN deal d ON d.company_id = m.company_id WHERE m.g % 25 = 0;
	INSERT INTO attachment (entity_type, entity_id, filename, content_type, byte_size, source, captured_by, storage_key, category)
	  SELECT 'activity', m.id, 'file-'||m.g||'-'||k||'.pdf', 'application/pdf', 200000, 'imap', 'connector:imap', 'k/'||m.g||'/'||k, 'email_attachment'
	    FROM seeded_mail m, generate_series(1, 2) k;
	INSERT INTO attachment (entity_type, entity_id, company_id, filename, content_type, byte_size, source, captured_by, storage_key)
	  SELECT 'company', c.id, c.id, 'upload-'||c.n||'-'||k||'.pdf', 'application/pdf', 100000, 'upload', 'system', 'u/'||c.n||'/'||k
	    FROM seeded_company c, generate_series(1, 50) k;
	ANALYZE company; ANALYZE contact; ANALYZE relationship; ANALYZE deal;
	ANALYZE activity; ANALYZE activity_link; ANALYZE attachment`); err != nil {
		t.Fatalf("seeding the busy account: %v", err)
	}
}

func TestABusyAccountsDocumentsListStartsFromTheAccount(t *testing.T) {
	e := setupPromises(t)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rolling back the seeded account: %v", err)
		}
	}()
	busy := ids.NewV7()
	seedBusyAccount(ctx, t, tx, busy)

	statement, args, err := companyDocumentsQuery(e.as(), busy, DocumentFilters{}, 50)
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+statement, args...).Scan(&raw); err != nil {
		t.Fatalf("planning the account's documents: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"` //nolint:tagliatelle // fixed by the server's plan format
	}
	if err := json.Unmarshal([]byte(raw), &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v (%d plans)", err, len(plans))
	}
	// Every read of attachment must be keyed by a record the account reaches:
	// its own id, a mail's id or the account's id. An index read on the kind
	// alone, or a heap scan, walks every file of the installation.
	var attachmentReads, unkeyedReads int
	plans[0].Plan.walk(func(n planNode) {
		onAttachment := n.RelationName == "attachment" || strings.HasPrefix(n.IndexName, "attachment_") ||
			strings.HasPrefix(n.IndexName, "idx_attachment_")
		if !onAttachment {
			return
		}
		attachmentReads++
		keyed := strings.Contains(n.IndexCond, "entity_id") || strings.Contains(n.IndexCond, "company_id") ||
			strings.Contains(n.IndexCond, "(id = ")
		if n.NodeType == "Seq Scan" || (n.IndexName != "" && !keyed) {
			unkeyedReads++
			t.Logf("unkeyed attachment read: %s %s %s", n.NodeType, n.IndexName, n.IndexCond)
		}
	})
	if attachmentReads == 0 {
		t.Fatal("the plan reads no attachment: the statement changed shape, so this test checks nothing")
	}
	if unkeyedReads != 0 {
		t.Errorf("the plan reads attachment %d times without a key the account reaches: the list filters every file of the installation instead of starting from the account's own files and mails", unkeyedReads)
	}
}
