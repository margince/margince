// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Rolling legal review back leaves data the binary before it can read: no
// legal_review evidence, no legal_reviewed criterion, and validated checks.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func TestRollingLegalReviewBackLeavesDataTheOlderBinaryReads(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		WITH p AS (INSERT INTO pipeline (name) VALUES ('Rollback') RETURNING id),
		     s AS (INSERT INTO stage (pipeline_id, name, position) SELECT id, 'Contract', 0 FROM p RETURNING id, pipeline_id),
		     c AS (INSERT INTO stage_exit_criterion (stage_id, key, label, kind, position)
		           SELECT id, 'legal', 'Legal approved', 'legal_reviewed', 0 FROM s RETURNING id),
		     d AS (INSERT INTO deal (name, stage_id, pipeline_id, source, captured_by)
		           SELECT 'Rollback deal', id, pipeline_id, 'manual', 'human:fixture' FROM s RETURNING id)
		INSERT INTO deal_stage_evidence (deal_id, criterion_id, source_type, source_id,
		       author_side, commitment, met, observed_at, extracted_by)
		SELECT d.id, c.id, 'legal_review', gen_random_uuid(), 'unknown', 'agreed', true, now(), 'deterministic'
		  FROM d, c`); err != nil {
		t.Fatalf("seeding an approved legal review criterion: %v", err)
	}

	if _, err := dbmigrate.Down(ctx, conn, namespaceThroughVersion(t, core, "1791956206"), 2); err != nil {
		t.Fatalf("down: %v", err)
	}

	var evidence, legalKinds int
	if err := conn.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM deal_stage_evidence WHERE source_type = 'legal_review'),
		       (SELECT count(*) FROM stage_exit_criterion WHERE kind = 'legal_reviewed')`).
		Scan(&evidence, &legalKinds); err != nil {
		t.Fatal(err)
	}
	if evidence != 0 || legalKinds != 0 {
		t.Fatalf("after the rollback %d legal_review evidence rows and %d legal_reviewed criteria remain", evidence, legalKinds)
	}
	var validated int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_constraint
		 WHERE conname IN ('deal_stage_evidence_source_type_check', 'stage_exit_criterion_kind_check')
		   AND convalidated`).Scan(&validated); err != nil {
		t.Fatal(err)
	}
	if validated != 2 {
		t.Fatalf("%d of the two narrowed checks are validated after the rollback", validated)
	}
	var table *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('legal_review')::text`).Scan(&table); err != nil {
		t.Fatal(err)
	}
	if table != nil {
		t.Fatal("the legal_review table survived the rollback")
	}
}
