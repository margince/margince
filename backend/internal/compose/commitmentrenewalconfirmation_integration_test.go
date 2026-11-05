// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReportedRenewalDoesNotChangeAnExistingTaskDeadline(t *testing.T) {
	f := setupRenewalPrivacy(t, 0.95)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	source, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: new("The pricing will arrive next Friday."), OccurredAt: &now,
		ThreadKey: "renewal-privacy", Direction: new("outbound"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := f.commitment
	latest := ids.UUID(source.Id)
	c.AsOf, c.AdmissionSourceID, c.RenewedAt = now.Add(time.Hour), &latest, &now
	c.ReadMessages = append(c.ReadMessages, latest)
	c.DueDate, c.NeedsConfirmation = "2026-10-23", true
	ctx := withTranscriptReader(f.e.Admin(), "human:"+f.e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(f.e.Pool, approvals.NewService(f.e.DB()))
	for _, confirmed := range []bool{false, true} {
		c.NeedsConfirmation = !confirmed
		var outcome CommitmentOutcome
		if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
			var err error
			outcome, _, err = dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
			return err
		}); err != nil {
			t.Fatal(err)
		}
		want, day := CommitmentRemembered, "2026-10-19"
		if confirmed {
			want, day = CommitmentRetained, "2026-10-23"
		}
		if outcome != want {
			t.Fatalf("confirmed=%v: outcome=%s, want %s", confirmed, outcome, want)
		}
		if got := f.e.WsScalar(t, `SELECT due_at::date::text FROM activity WHERE id=$1`, f.work); got != day {
			t.Fatalf("confirmed=%v: deadline=%s, want %s", confirmed, got, day)
		}
	}
}
