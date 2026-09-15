// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAllowanceRecoveryLeavesIneligibleRequestersParkedWithoutFailing(t *testing.T) {
	for _, reason := range []string{"grant_revoked", "deactivated", "archived_company", "outside_scope"} {
		t.Run(reason, func(t *testing.T) {
			e := integration.Setup(t)
			grantBudgetRequester(t, e, e.Rep1)
			service := identity.NewService(e.Pool)
			actor := recoveryAdminIdentity(e)
			user := ids.From[ids.UserKind](e.Rep1)
			owner := e.Rep1
			if reason == "outside_scope" {
				owner = e.Rep2
				if err := service.ChangeUserRole(e.Admin(), actor, user, "admin"); err != nil {
					t.Fatal(err)
				}
			}
			company := insertCompany(t, e, owner, "acme.example", "")
			read, args := startDeepRead(t, e, company)
			next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
			worker, _ := newDeepReadTestWorker(e, acmeDeepSite(), budgetDeferringBrain{next: next})
			if err := worker.run(context.Background(), args); err == nil {
				t.Fatal("expected deferral")
			}
			runner, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
			if err != nil {
				t.Fatal(err)
			}
			if err := runner.Enqueue(e.Admin(), args, &river.InsertOpts{ScheduledAt: next, MaxAttempts: 3}); err != nil {
				t.Fatal(err)
			}
			switch reason {
			case "grant_revoked":
				err = service.ChangeUserRole(e.Admin(), actor, user, "read_only")
			case "deactivated":
				err = service.DeactivateUser(e.Admin(), actor, identity.DeactivateUserInput{UserID: user})
			case "archived_company":
				_, err = e.Contacts.ArchiveCompany(e.Admin(), companyIDOf(company), nil)
			case "outside_scope":
				err = service.ChangeUserRole(e.Admin(), actor, user, "rep")
			}
			if err != nil {
				t.Fatal(err)
			}
			setRecoveryAllowance(t, e, 1000000)
			recovery := newAIBudgetResumeWorker(e.Pool, slog.New(slog.DiscardHandler))
			for range 2 {
				if err := recovery.resumeWorkspace(e.Admin(), e.WS); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
				var deadline, scheduled time.Time
				if err := tx.QueryRow(e.Admin(), `SELECT next_attempt_at FROM site_read WHERE id=$1`, read.ID).Scan(&deadline); err != nil {
					return err
				}
				if err := tx.QueryRow(e.Admin(), `SELECT scheduled_at FROM river_job WHERE kind='site_deep_read' AND args->>'site_read_id'=$1`, read.ID.String()).Scan(&scheduled); err != nil {
					return err
				}
				if !deadline.Equal(next) || !scheduled.Equal(next) {
					t.Fatal("ineligible request or queue clock advanced")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if reason == "grant_revoked" {
				if err := service.ChangeUserRole(e.Admin(), actor, user, "rep"); err != nil {
					t.Fatal(err)
				}
				if err := recovery.resumeWorkspace(e.Admin(), e.WS); err != nil {
					t.Fatal(err)
				}
				current, err := e.Contacts.GetSiteRead(e.Admin(), companyIDOf(company), read.ID)
				if err != nil || current.NextAttemptAt == nil || !current.NextAttemptAt.Before(next) {
					t.Fatalf("restored grant did not resume: %+v %v", current, err)
				}
			}
		})
	}
}
