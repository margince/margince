// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

// A deployment's retain-only seed writes no row under a boot value, which
// would act only once that value is gone. Not parallel: the binding is the
// process's own.
func TestABootSetRetainOnlyPostureIsNotSeeded(t *testing.T) {
	e := integration.Setup(t)
	bindForTest(t, "MARGINCE_PRIVACY_RETAIN_ONLY=false")
	seeds := deployconfig.Seeds{Retention: &deployconfig.RetentionSeed{DefaultPolicy: deployconfig.RetentionRetainOnlyPosture}}
	if !seeds.RetainOnly() {
		t.Fatal("the fixture does not ask for the retain-only posture")
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return seedRetentionPosture(e.Admin(), tx, seeds)
	}); err != nil {
		t.Fatalf("seeding the posture under a boot value: %v", err)
	}
	var rows int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, privacy.RetainOnly.Key()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the seed wrote %d posture rows under a boot value, want none", rows)
	}
}
