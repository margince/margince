// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The retain-only posture the environment sets is the one the nightly pass
// and the policy list apply, with no row saying so. Not parallel: the binding
// is the process's own.
func TestABootSetRetainOnlyPostureSuppressesDestruction(t *testing.T) {
	apptest.BindEnv(t, []string{"MARGINCE_PRIVACY_RETAIN_ONLY=true"})
	e := Setup(t)
	SeedRetentionPolicies(t, e)
	staleLead, _, _, _ := seedOverAgeRecords(t, e)

	policies, err := privacy.NewPolicyStore(e.DB()).List(retentionAdminCtx(e.WS, principal.ObjectGrant{Read: true}))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range policies {
		if destructive := p.Action == "anonymize" || p.Action == "erase"; p.SuppressedByPosture != destructive {
			t.Errorf("%s/%s: suppressed_by_posture = %v, want %v under the boot-set posture",
				p.Scope, p.Action, p.SuppressedByPosture, destructive)
		}
	}

	svc := compose.NewRetentionServiceFor(e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	SettleIntoInstall(t, e)
	if err := svc.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}
	var leadName string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT full_name FROM lead WHERE id = $1`, staleLead).Scan(&leadName)
	}); err != nil {
		t.Fatal(err)
	}
	if leadName != "Old Cold Lead" {
		t.Errorf("the pass anonymized an over-age lead under the boot-set posture: %q", leadName)
	}
}
