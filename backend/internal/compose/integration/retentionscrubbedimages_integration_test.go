// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A retention erase of a risk day or a stored original scrubs that record's
// earlier audit images. gates/scrubbedentitytypes_test.go lists both types as
// scrubbed, and this is what makes the list true. Each record gets an imaged
// audit row through storekit, then the sweep the worker runs.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARetentionEraseScrubsTheImagesOfARiskDay(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	day := ids.NewV7()
	e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action)
		VALUES ('deal_risk_day', NULL, 90, 'erase')`)
	e.WsExec(t, `INSERT INTO deal_risk_day (id, local_day, day_start, day_end, judged_at, captured_by)
		VALUES ($1, (now() - interval '100 days')::date, now() - interval '100 days',
		        now() - interval '99 days', now() - interval '100 days', 'system:test')`, day)
	auditWithImages(t, e, "deal_risk_day", day)

	runRetentionSweep(t, e)

	assertImagesScrubbed(t, e, "deal_risk_day", day)
}

func TestARetentionEraseScrubsTheImagesOfAStoredOriginal(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	activity := ids.NewV7()
	e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action)
		VALUES ('raw_capture', NULL, 100, 'erase')`)
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by, source_system, source_id)
		VALUES ($1, 'email', 'Quarterly figures', 'the numbers themselves',
		        now() - interval '400 days', 'capture', 'connector:t', 'gmail', 'msg-scrubbed')`, activity)
	original := ids.MustParse(e.WsScalar(t, `INSERT INTO raw_capture (source_system, source_id, payload, received_at)
		VALUES ('gmail', 'msg-scrubbed', '{"body":"the numbers themselves"}', now() - interval '400 days')
		RETURNING id::text`))
	auditWithImages(t, e, "raw_capture", original)

	runRetentionSweep(t, e)

	assertImagesScrubbed(t, e, "raw_capture", original)
}

// auditWithImages files an update with both images against one record, the
// shape a scrub must redact.
func auditWithImages(t *testing.T, e *Env, entityType string, id ids.UUID) {
	t.Helper()
	ctx := e.Admin()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.Audit(ctx, tx, "update", entityType, id,
			map[string]any{"note": "what it held"}, map[string]any{"note": "what it holds"})
		return err
	}); err != nil {
		t.Fatalf("auditing the %s: %v", entityType, err)
	}
}

func runRetentionSweep(t *testing.T, e *Env) {
	t.Helper()
	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(context.Background(), e.WS)); err != nil {
		t.Fatalf("running the retention sweep: %v", err)
	}
}

func assertImagesScrubbed(t *testing.T, e *Env, entityType string, id ids.UUID) {
	t.Helper()
	certificates := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = $1 AND entity_id = $2 AND action = 'erase' AND before IS NULL AND after IS NULL`,
		entityType, id)
	if certificates != 1 {
		t.Fatalf("%d erase certificates for the %s, want the sweep's one", certificates, entityType)
	}
	imaged := e.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = $1 AND entity_id = $2 AND (before IS NOT NULL OR after IS NOT NULL)`, entityType, id)
	if imaged != 0 {
		t.Errorf("%d audit rows of the erased %s still hold an image", imaged, entityType)
	}
	ctx := e.Admin()
	var evidence string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT coalesce(evidence::text, '') FROM audit_log
			WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'`, entityType, id).Scan(&evidence)
	}); err != nil {
		t.Fatalf("reading the scrubbed update of the %s: %v", entityType, err)
	}
	if evidence != `{"redacted": true}` {
		t.Errorf("the scrubbed update of the %s carries evidence %s, want only the redaction mark", entityType, evidence)
	}
}
