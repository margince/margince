// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobfanout

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	runner, completed, failed := startRetentionRunner(t, e, time.Hour)
	for _, released := range []bool{false, true} {
		if released {
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				_, err := tx.Exec(e.Admin(), `UPDATE contact SET legal_hold=false WHERE id=@id`, pgx.StrictNamedArgs{"id": contact})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := runner.Enqueue(e.Admin(), compose.PrivacyRetentionArgs{}, nil); err != nil {
				t.Fatal(err)
			}
		}
		waitCtx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
		succeeded := jobtest.AwaitKindOutcome(waitCtx, t, completed, failed, compose.PrivacyRetentionArgs{}.Kind())
		cancel()
		if !succeeded {
			t.Fatal("retention failed before finishing the historical cleanup")
		}
		if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
			var personal bool
			if err := tx.QueryRow(e.Admin(), `SELECT after IS NOT NULL FROM audit_log WHERE id=@id`, pgx.StrictNamedArgs{"id": original}).Scan(&personal); err != nil {
				return err
			}
			if personal == released {
				t.Fatalf("released=%v retained audit image=%v", released, personal)
			}
			var metadata []byte
			if err := tx.QueryRow(e.Admin(), `SELECT metadata FROM river_job WHERE kind=@kind AND state='completed' ORDER BY id DESC LIMIT 1`, pgx.StrictNamedArgs{"kind": compose.PrivacyRetentionArgs{}.Kind()}).Scan(&metadata); err != nil {
				return err
			}
			var cursor struct {
				PoliciesDone bool                          `json:"privacy_policies_done"`
				Audit        privacy.HistoricalAuditCursor `json:"privacy_audit_cursor"`
			}
			if err := json.Unmarshal(metadata, &cursor); err != nil {
				return err
			}
			if !cursor.PoliciesDone || cursor.Audit.Phase < 1 {
				t.Fatalf("historical cursor did not persist across snoozes: %s", metadata)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
