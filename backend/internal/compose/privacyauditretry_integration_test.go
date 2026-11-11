// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestRetentionJobPersistsItsAuditCursorAndRevisitsAReleasedHold(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	contact := e.SeedContact(t, "Subject with an earlier erasure", nil)
	var original ids.UUID
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(e.Admin(), `UPDATE contact SET legal_hold=true WHERE id=@id`, pgx.StrictNamedArgs{"id": contact}); err != nil {
			return err
		}
		var err error
		original, err = storekit.AuditEvent(e.Admin(), tx, "create", "contact", contact, map[string]any{"full_name": "Old subject value"})
		if err != nil {
			return err
		}
		_, err = storekit.AuditWithEvidence(e.Admin(), tx, "erase", "contact", contact, nil, nil, map[string]any{"cause": "contact_erasure"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, released := range []bool{false, true} {
		if released {
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				_, err := tx.Exec(e.Admin(), `UPDATE contact SET legal_hold=false WHERE id=@id`, pgx.StrictNamedArgs{"id": contact})
				return err
			}); err != nil {
				t.Fatal(err)
			}
		}
		var row *rivertype.JobRow
		completed := false
		for range 12 {
			result := runPrivacyRetentionAttempt(t, e, row)
			row = result.Job
			if result.EventKind == river.EventKindJobCompleted {
				completed = true
				break
			}
			if result.EventKind != river.EventKindJobSnoozed {
				t.Fatalf("retention attempt ended as %s", result.EventKind)
			}
			var cursor privacyRetentionCursor
			if err := json.Unmarshal(row.Metadata, &cursor); err != nil || !cursor.PoliciesDone {
				t.Fatalf("retention cursor did not persist: %s, %v", row.Metadata, err)
			}
		}
		if !completed {
			t.Fatal("retention did not finish its bounded historical passes")
		}
		got := countRows(t, e, `SELECT count(*) FROM audit_log WHERE id=@id AND after IS NOT NULL`, pgx.NamedArgs{"id": original})
		if (!released && got != 1) || (released && got != 0) {
			t.Fatalf("released=%v retained audit images=%d", released, got)
		}
	}
}

func runPrivacyRetentionAttempt(t *testing.T, e *integration.Env, row *rivertype.JobRow) *rivertest.WorkResult {
	t.Helper()
	spec, ok := jobs.SpecFor(PrivacyRetentionArgs{}.Kind())
	if !ok {
		t.Fatal("retention job is not declared")
	}
	production := &privacyRetentionWorker{pool: e.Pool, identity: identity.NewService(e.Pool), retention: func(db *database.DB) *privacy.RetentionService {
		return NewRetentionServiceFor(db, nil, slog.Default())
	}}
	worker := rivertest.NewWorker(t, riverpgxv5.New(e.Pool), &river.Config{}, jobs.Govern(production, spec))
	tx, err := e.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(context.Background()); err != nil && err != pgx.ErrTxClosed {
			t.Error(err)
		}
	}()
	var result *rivertest.WorkResult
	if row == nil {
		result, err = worker.Work(t.Context(), t, tx, PrivacyRetentionArgs{}, nil)
	} else {
		result, err = worker.WorkJob(t.Context(), t, tx, row)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return result
}
