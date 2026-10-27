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
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCaptureObservationFailureLeavesTheWorklistSnapshotUsable(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	subject := "Check the recorded request"
	store := activities.NewStore(e.DB())
	if _, _, err := store.LogActivity(reader, activities.LogActivityInput{
		Kind: "task", Source: "manual", Subject: &subject,
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.booked}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.booked)); err != nil {
		t.Fatal(err)
	}
	cards.WithMailCoverage(func(ctx context.Context, _ *time.Time, _ time.Time) (crmcontracts.MailCaptureCoverage, error) {
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "SELECT 1 / 0")
			return err
		})
		return crmcontracts.MailCaptureCoverage{}, err
	})
	err := database.WithWorkspaceSnapshot(reader, e.Pool, func(ctx context.Context) error {
		moves, err := cards.CachedMoves(ctx, []ids.UUID{e.booked})
		if err != nil {
			return err
		}
		move, found := moves[e.booked]
		if !found || move.MailCoverage == nil || move.MailCoverage.State != crmcontracts.MailCaptureCoverageStateUnknown {
			t.Fatalf("failed observation removed recorded work: %+v", moves)
		}
		return database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var readable int
			if err := tx.QueryRow(ctx, "SELECT 1").Scan(&readable); err != nil {
				return err
			}
			if readable != 1 {
				t.Fatalf("snapshot continuation = %d", readable)
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}
