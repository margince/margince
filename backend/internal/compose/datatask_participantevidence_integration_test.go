// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestParticipantEvidenceCorrectionRollsBackReplaysAndRepairsLegacyGaps(t *testing.T) {
	e := integration.Setup(t)
	subject := "Stored domain evidence"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Source: "manual", EmailFrom: "buyer@bücher.example", EmailTo: []string{"owner@myco.example"},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	readiness := func(want bool) {
		t.Helper()
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			got, err := storekit.ParticipantEvidenceReady(ctx, tx)
			if err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("projection ready=%v want=%v", got, want)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	readiness(true)
	if got := e.WsCount(t, `SELECT count(*) FROM activity_participant WHERE address_domain_folded='xn--bcher-kva.example' AND domain_evidence_at IS NOT NULL`); got != 1 {
		t.Fatalf("real writer evidence keys=%d", got)
	}
	integration.ApplyRiverSchema(t)
	client, err := river.NewClient(riverpgxv5.New(e.Pool), &river.Config{Logger: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	worker := dataTaskReconcileWorker{pool: e.Pool}
	clearParticipantEvidenceFixture(t)
	readiness(false)
	if err := worker.Work(rivertest.WorkContext(t.Context(), client), nil); err != nil {
		t.Fatal(err)
	}
	readiness(false)
	task := participantEvidenceDataTask()
	task.Steps[0].Limit = 1
	task.Steps[0].MinLimit = 1
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		sp, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		out, err := task.Steps[0].Batch(ctx, sp, datatask.BatchIn{Limit: 1})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			return fmt.Errorf("first batch changed=%d", out.Changed)
		}
		return sp.Rollback(ctx)
	}); err != nil {
		t.Fatal(err)
	}
	readiness(false)
	runMachineAddressTask(ctx, t, e, task)
	readiness(true)
	runMachineAddressTask(ctx, t, e, task)
	readiness(true)
	clearParticipantEvidenceFixture(t)
	readiness(false)
	if err := worker.Work(rivertest.WorkContext(t.Context(), client), nil); err != nil {
		t.Fatal(err)
	}
	readiness(true)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		replay, err := activities.RepairParticipantEvidenceTx(ctx, tx, 500)
		if err != nil {
			return err
		}
		if replay != 0 {
			return fmt.Errorf("replay changed=%d", replay)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	readiness(true)
}

func clearParticipantEvidenceFixture(t *testing.T) {
	t.Helper()
	testdb.WithFixtureTriggerDisabled(t, "activity_participant", "stamp_participant_domain_evidence", func(tx pgx.Tx) error {
		_, err := tx.Exec(t.Context(), `UPDATE activity_participant SET domain_evidence_parent_version=NULL,domain_evidence_at=NULL,domain_evidence_precision=NULL,domain_evidence_visible=NULL`)
		return err
	})
}
