// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLegacyCapturedMailUsesItsOriginalDateInsteadOfArrival(t *testing.T) {
	e := integration.Setup(t)
	captureThreadMail(t, e, e.Rep1, threadMail(seatAddress(t, e, e.Rep1), "legacy-date@buyer.test", "Fri, 18 Oct 2024 10:00:00 +0000", ""))
	source := ids.MustParse(e.WsScalar(t, `SELECT activity_id::text FROM activity_identity WHERE identity_kind='mail' AND identity_key=$1`, "legacy-date@buyer.test"))
	asOf := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	e.WsExec(t, `UPDATE capture_import SET source_occurred_at=NULL WHERE activity_id=$1`, source)
	e.WsExec(t, `UPDATE activity SET occurred_at=$2 WHERE id=$1`, source, asOf)
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	c := Commitment{
		Meaning: openCommitmentMeaning("", ""),
		AsOf:    asOf, SourceActivityID: source, ReadMessages: []ids.UUID{source}, Extractor: signalScanActor,
		Locator: commitmentLocator(source, claimKindOurs, e.Rep1.String(), "Send pricing"), Seat: &e.Rep1, Summary: "Send pricing", Confidence: 0.95,
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
		if err == nil && outcome != CommitmentOutcome(activities.CommitmentHistorical) {
			t.Errorf("legacy source outcome=%s, want historical", outcome)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatalf("historical mail created %d tasks", got)
	}
}
