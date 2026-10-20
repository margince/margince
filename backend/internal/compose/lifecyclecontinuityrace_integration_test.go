// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnUncommittedEngagementChangePreventsEndingTheAccount(t *testing.T) {
	for _, kind := range []string{"contract", "project"} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			signal := seedOpenContractEnded(t, e, company)
			change := prepareContinuityChange(t, e, company, kind)
			if proposePass(t, e) != 1 {
				t.Fatal("fixture did not stage the initial termination")
			}
			release := make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			ready, written := make(chan int32, 1), make(chan error, 1)
			go func() {
				err := database.WithWorkspaceSnapshot(e.Admin(), e.Pool, func(ctx context.Context) error {
					if err := change(ctx); err != nil {
						return err
					}
					var pid int32
					if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
						return tx.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid)
					}); err != nil {
						return err
					}
					ready <- pid
					<-release
					return nil
				})
				written <- err
				close(ready)
			}()
			holder := <-ready
			if holder == 0 {
				t.Fatalf("prepare uncommitted engagement: %v", <-written)
			}
			proposal := ids.From[ids.ApprovalKind](stagedOffer(t, e, company))
			decided, done := make(chan error, 1), make(chan struct{})
			go func() {
				defer close(done)
				_, err := approvalsServiceWithEffects(e.Pool).Decide(e.As(e.Rep1, nil, integration.AdminWithSignals), proposal, true, nil)
				decided <- err
			}()
			testdb.WaitForContention(t, done, "termination ignored an uncommitted engagement", "termination never reached the engagement lock", func(ctx context.Context) (bool, error) {
				var waiting bool
				err := e.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_stat_activity WHERE $1 = ANY(pg_blocking_pids(pid)))`, holder).Scan(&waiting)
				return waiting, err
			})
			close(release)
			if err := <-written; err != nil {
				t.Fatal(err)
			}
			if err := <-decided; err != nil && !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatal(err)
			}
			if accountStage(t, e, company) != "customer" || signalStatus(t, e, signal) != "open" {
				t.Fatal("concurrent engagement was ignored during termination")
			}
		})
	}
}

func prepareContinuityChange(t *testing.T, e *integration.Env, company ids.UUID, kind string) func(context.Context) error {
	t.Helper()
	account := ids.From[ids.CompanyKind](company)
	if kind == "contract" {
		agreement, err := e.Contracts.CreateContract(e.Admin(), contracts.CreateContractInput{
			CompanyID: account, Title: "Ongoing support", ValueBasis: contracts.BasisTotal, Source: "manual",
		})
		if err != nil {
			t.Fatal(err)
		}
		return func(ctx context.Context) error {
			_, err := e.Contracts.ChangeStatus(ctx, ids.From[ids.ContractKind](ids.UUID(agreement.Id)), contracts.StatusActive, nil)
			return err
		}
	}
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{CompanyID: account, Name: "Ongoing delivery", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return func(ctx context.Context) error {
		_, err := e.Projects.AdvanceProjectPhase(ctx, ids.From[ids.ProjectKind](ids.UUID(project.Id)), projects.AdvanceProjectPhaseInput{ToPhase: projects.PhaseDelivering})
		return err
	}
}
