// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// HoldUnderStatutoryFloor seeds a held record without registering a process-wide
// jurisdiction policy, so a reader test cannot change other tests' retention rules.
func HoldUnderStatutoryFloor(t *testing.T, e *Env, activity ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO activity_retention_evidence
			       (activity_id, basis, qualified_at, decided_by_name, reason)
			VALUES ($1, 'controller_pin', now(), 'Datenschutz', 'supplier correspondence')`,
			activity); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			UPDATE activity
			   SET restricted_at = now(), archived_at = now(),
			       restricted_reason = 'commercial_correspondence',
			       restricted_until = now() + interval '10 years',
			       retention_class = 'commercial_correspondence', retention_class_at = now()
			 WHERE id = $1`, activity)
		return err
	}); err != nil {
		t.Fatalf("placing the message under a statutory hold: %v", err)
	}
}
