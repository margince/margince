// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAuditRetryCursorCountsHeldRowsAgainstItsPageAndContinues(t *testing.T) {
	e := setupAuditBoundary(t)
	withAuditRetryFixture(t, e, func(ctx context.Context) {
		var boundary, later ids.UUID
		if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE contact SET legal_hold=true WHERE id=@id`, pgx.StrictNamedArgs{"id": e.contact}); err != nil {
				return err
			}
			for range retentionBatch {
				id, err := storekit.AuditWithEvidence(ctx, tx, "erase", "contact", e.contact.UUID, nil, nil,
					map[string]any{"cause": "contact_erasure"})
				if err != nil {
					return err
				}
				boundary = id
			}
			other := ids.NewV7()
			later = seedPersonalAudit(ctx, t, tx, "contact", other)
			_, err := storekit.AuditWithEvidence(ctx, tx, "erase", "contact", other, nil, nil,
				map[string]any{"cause": "contact_erasure"})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		service := NewRetentionService(e.db, nil, slog.Default(), nil)
		cursor, done, err := service.RetryHistoricalAuditPage(ctx, HistoricalAuditCursor{})
		if err != nil {
			t.Fatal(err)
		}
		if done || cursor.After != boundary || cursor.Phase != 0 {
			t.Fatalf("one-page cursor=%+v done=%v; held rows must count toward the cap", cursor, done)
		}
		if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
			assertLegacyAuditImage(ctx, t, tx, later, true)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, _, err := service.RetryHistoricalAuditPage(ctx, cursor); err != nil {
			t.Fatal(err)
		}
		if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
			assertLegacyAuditImage(ctx, t, tx, later, false)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func TestAuditRetrySweepReturnsToHeldLinkedCopiesAfterRelease(t *testing.T) {
	e := setupAuditBoundary(t)
	withAuditRetryFixture(t, e, func(ctx context.Context) {
		var original, subject ids.UUID
		if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
			subject = e.contact.UUID
			twin := ids.NewV7()
			if _, err := tx.Exec(ctx, `INSERT INTO contact(id,full_name,source,captured_by)
 VALUES (@id,'Another contact','manual','system:privacy_test')`, pgx.StrictNamedArgs{"id": twin}); err != nil {
				return err
			}
			pair := fileDedupePair(ctx, t, tx, "contact", subject, twin, subject.String()+"@audit.test", e.user)
			original = seedPersonalAudit(ctx, t, tx, "dedupe_candidate", pair)
			legacyContactProof(ctx, t, tx, subject)
			_, err := tx.Exec(ctx, `UPDATE contact SET legal_hold=true WHERE id=@id`, pgx.StrictNamedArgs{"id": subject})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		service := NewRetentionService(e.db, nil, slog.Default(), nil)
		runAuditRetrySweep(t, ctx, service)
		if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
			assertLegacyAuditImage(ctx, t, tx, original, true)
			_, err := tx.Exec(ctx, `UPDATE contact SET legal_hold=false WHERE id=@id`, pgx.StrictNamedArgs{"id": subject})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		runAuditRetrySweep(t, ctx, service)
		if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
			assertLegacyAuditImage(ctx, t, tx, original, false)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func runAuditRetrySweep(t *testing.T, ctx context.Context, service *RetentionService) {
	t.Helper()
	var cursor HistoricalAuditCursor
	for range 20 {
		next, done, err := service.RetryHistoricalAuditPage(ctx, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			return
		}
		if next == cursor {
			t.Fatalf("audit sweep made no cursor progress: %+v", cursor)
		}
		cursor = next
	}
	t.Fatal("small audit fixture did not finish within 20 bounded pages")
}

func withAuditRetryFixture(t *testing.T, e *auditBoundaryEnv, fn func(context.Context)) {
	t.Helper()
	rollback := errors.New("rollback audit retry fixture")
	ctx := principal.SystemActing(e.ctx, "privacy_test")
	err := database.WithWorkspaceSnapshot(ctx, e.db.Pool(), func(ctx context.Context) error {
		fn(ctx)
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("audit retry fixture: %v", err)
	}
}
